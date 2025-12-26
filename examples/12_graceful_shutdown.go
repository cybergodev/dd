package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/cybergodev/dd"
)

func main() {
	fmt.Println("=== DD Graceful Shutdown ===\n ")

	basicShutdown()
	signalHandling()
	contextShutdown()
	applicationLifecycle()

	fmt.Println("\n✅ Examples completed")
}

// 1. Basic Shutdown - Always close logger before exit
func basicShutdown() {
	fmt.Println("1. Basic Shutdown")

	config, _ := dd.DefaultConfig().WithFile("logs/shutdown.log", dd.FileWriterConfig{})
	logger, _ := dd.New(config)

	logger.Info("Application started")

	// Simulate work
	for i := 0; i < 3; i++ {
		logger.InfoWith("Processing", dd.Int("iteration", i))
		time.Sleep(100 * time.Millisecond)
	}

	// Always close logger to flush buffers
	logger.Info("Shutting down")
	if err := logger.Close(); err != nil {
		fmt.Printf("Shutdown error: %v\n", err)
	}

	// Logs after close are safely ignored
	logger.Info("This is ignored (logger closed)")
}

// 2. Signal Handling - Respond to SIGINT/SIGTERM
func signalHandling() {
	fmt.Println("\n2. Signal Handling")

	config, _ := dd.JSONConfig().WithFile("logs/signals.log", dd.FileWriterConfig{})
	logger, _ := dd.New(config)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	done := make(chan bool)

	// Background worker
	go func() {
		for i := 0; ; i++ {
			select {
			case <-done:
				logger.Info("Worker stopped")
				return
			default:
				logger.InfoWith("Working", dd.Int("iteration", i))
				time.Sleep(200 * time.Millisecond)
			}
		}
	}()

	logger.Info("Running (will auto-stop in 1s)")

	// Auto-trigger shutdown for demo
	go func() {
		time.Sleep(1 * time.Second)
		sigChan <- syscall.SIGTERM
	}()

	// Wait for signal
	sig := <-sigChan
	logger.InfoWith("Received signal", dd.String("signal", sig.String()))

	done <- true
	time.Sleep(100 * time.Millisecond) // Let worker finish

	logger.Info("Shutting down")
	logger.Close()
}

// 3. Context Shutdown - Coordinate multiple workers
func contextShutdown() {
	fmt.Println("\n3. Context Shutdown")

	config, _ := dd.JSONConfig().WithFile("logs/context.log", dd.FileWriterConfig{})
	logger, _ := dd.New(config)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup

	// Start 3 workers
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			ticker := time.NewTicker(200 * time.Millisecond)
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					logger.InfoWith("Worker stopped", dd.Int("worker_id", id))
					return
				case <-ticker.C:
					logger.InfoWith("Working", dd.Int("worker_id", id))
				}
			}
		}(i)
	}

	logger.InfoWith("Started workers", dd.Int("count", 3))

	// Run then cancel
	time.Sleep(800 * time.Millisecond)
	logger.Info("Cancelling context")
	cancel()

	wg.Wait()
	logger.Info("All workers finished")
	logger.Close()
}

// 4. Application Lifecycle - Complete startup/shutdown pattern
func applicationLifecycle() {
	fmt.Println("\n4. Application Lifecycle")

	app := &Application{}

	if err := app.Start(); err != nil {
		fmt.Printf("Start failed: %v\n", err)
		return
	}

	app.Run()

	if err := app.Stop(); err != nil {
		fmt.Printf("Stop failed: %v\n", err)
	}
}

// Application demonstrates complete lifecycle management
type Application struct {
	logger *dd.Logger
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func (app *Application) Start() error {
	config, err := dd.JSONConfig().WithFile("logs/lifecycle.log", dd.FileWriterConfig{})
	if err != nil {
		return err
	}
	logger, err := dd.New(config)
	if err != nil {
		return err
	}
	app.logger = logger
	app.ctx, app.cancel = context.WithCancel(context.Background())

	app.logger.InfoWith("Application started", dd.Int("pid", os.Getpid()))

	// Start 2 background workers
	for i := 0; i < 2; i++ {
		app.wg.Add(1)
		go app.worker(i)
	}

	return nil
}

func (app *Application) Run() {
	app.logger.Info("Running")

	for i := 0; i < 3; i++ {
		select {
		case <-app.ctx.Done():
			return
		default:
			app.logger.InfoWith("Main work", dd.Int("iteration", i))
			time.Sleep(200 * time.Millisecond)
		}
	}
}

func (app *Application) Stop() error {
	app.logger.Info("Stopping")
	app.cancel()

	// Wait for workers with timeout
	done := make(chan struct{})
	go func() {
		app.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		app.logger.Info("Workers stopped")
	case <-time.After(2 * time.Second):
		app.logger.Warn("Timeout waiting for workers")
	}

	return app.logger.Close()
}

func (app *Application) worker(id int) {
	defer app.wg.Done()
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-app.ctx.Done():
			app.logger.InfoWith("Worker stopped", dd.Int("id", id))
			return
		case <-ticker.C:
			app.logger.InfoWith("Worker tick", dd.Int("id", id))
		}
	}
}
