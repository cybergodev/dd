//go:build examples

package main

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/cybergodev/dd"
)

// Writers - Advanced Output Management
//
// Topics covered:
// 1. FileWriter with rotation
// 2. BufferedWriter for high throughput
// 3. MultiWriter for multiple outputs
// 4. Dynamic writer management
// 5. Error handling
func main() {
	fmt.Println("=== DD Writers Management ===")

	section1FileWriter()
	section2BufferedWriter()
	section3MultiWriter()
	section4DynamicManagement()
	section5WriterErrors()

	fmt.Println("\n✅ Writers examples completed!")
}

// Section 1: FileWriter with rotation
func section1FileWriter() {
	fmt.Println("1. FileWriter")
	fmt.Println("--------------")

	// Direct FileWriter creation
	fileWriter, err := dd.NewFileWriter("logs/direct.log", dd.FileWriterConfig{
		MaxSizeMB:  100,
		MaxBackups: 10,
		MaxAge:     7 * 24 * time.Hour,
		Compress:   true,
	})
	if err != nil {
		fmt.Printf("Failed: %v\n", err)
		return
	}
	defer fileWriter.Close()

	// Use with logger
	cfg := dd.DefaultConfig()
	cfg.Targets = []dd.OutputTarget{dd.CustomOutput(fileWriter)}

	logger, _ := dd.New(cfg)
	defer logger.Close()

	logger.Info("Direct file writer output")

	fmt.Println("✓ File: logs/direct.log")
	fmt.Println()
}

// Section 2: BufferedWriter for high throughput
func section2BufferedWriter() {
	fmt.Println("2. BufferedWriter (High Throughput)")
	fmt.Println("-------------------------------------")

	// Create underlying file writer
	fileWriter, err := dd.NewFileWriter("logs/buffered.log", dd.DefaultFileWriterConfig())
	if err != nil {
		fmt.Printf("Failed: %v\n", err)
		return
	}
	defer fileWriter.Close()

	// Wrap with buffer (default 4KB buffer)
	bufferedWriter, err := dd.NewBufferedWriter(fileWriter, dd.DefaultBufferedWriterConfig())
	if err != nil {
		fmt.Printf("Failed: %v\n", err)
		return
	}
	defer bufferedWriter.Close() // IMPORTANT: Always call Close to flush!

	cfg := dd.DefaultConfig()
	cfg.Targets = []dd.OutputTarget{dd.CustomOutput(bufferedWriter)}

	logger, _ := dd.New(cfg)
	defer logger.Close()

	// High-throughput logging
	start := time.Now()
	for i := 0; i < 1000; i++ {
		logger.InfoWith("Buffered entry",
			dd.Int("seq", i),
		)
	}
	duration := time.Since(start)

	fmt.Printf("✓ 1000 messages in %v\n", duration)
	fmt.Println("  Note: Close() flushes the buffer")
	fmt.Println()
}

// Section 3: MultiWriter for multiple outputs
func section3MultiWriter() {
	fmt.Println("3. MultiWriter (Multiple Outputs)")
	fmt.Println("-----------------------------------")

	// Create MultiWriter combining outputs
	fileWriter, err := dd.NewFileWriter("logs/multi.log", dd.DefaultFileWriterConfig())
	if err != nil {
		fmt.Printf("Failed to create file writer: %v\n", err)
		return
	}
	defer fileWriter.Close()
	multiWriter := dd.NewMultiWriter(os.Stdout, fileWriter)

	cfg := dd.DefaultConfig()
	cfg.Targets = []dd.OutputTarget{dd.CustomOutput(multiWriter)}

	logger, _ := dd.New(cfg)
	defer logger.Close()

	logger.Info("This appears in BOTH console and file")
	logger.InfoWith("Structured data",
		dd.String("source", "multiwriter"),
	)

	fmt.Println()
}

// Section 4: Dynamic writer management
func section4DynamicManagement() {
	fmt.Println("4. Dynamic Writer Management")
	fmt.Println("-----------------------------")

	logger, _ := dd.New()
	defer logger.Close()

	fmt.Printf("Initial writers: %d\n", logger.WriterCount())

	// Add writers dynamically
	fileWriter, err := dd.NewFileWriter("logs/dynamic.log", dd.DefaultFileWriterConfig())
	if err != nil {
		fmt.Printf("Failed to create file writer: %v\n", err)
		return
	}
	// The writer outlives AddWriter/RemoveWriter — close it ourselves
	defer fileWriter.Close()

	if err := logger.AddWriter(fileWriter); err != nil {
		fmt.Printf("Failed to add writer: %v\n", err)
		return
	}
	fmt.Printf("After adding file: %d writers\n", logger.WriterCount())

	logger.Info("Goes to console + file")

	// Remove writer
	logger.RemoveWriter(fileWriter)
	fmt.Printf("After removing file: %d writers\n", logger.WriterCount())

	// Logger state inspection
	fmt.Printf("Is closed: %v\n", logger.IsClosed())
	fmt.Printf("Level: %s\n", logger.GetLevel().String())

	fmt.Println()
}

// Section 5: Writer error handling
func section5WriterErrors() {
	fmt.Println("5. Writer Error Handling")
	fmt.Println("-------------------------")

	// A failing writer guarantees the handler is invoked on every write.
	cfg := dd.DefaultConfig()
	cfg.Targets = []dd.OutputTarget{dd.CustomOutput(failingWriter{})}
	cfg.WriteErrorHandler = func(writer io.Writer, err error) {
		fmt.Printf("  [Config Handler] %T: %v\n", writer, err)
	}

	logger, _ := dd.New(cfg)
	defer logger.Close()

	// Override the handler at runtime
	logger.SetWriteErrorHandler(func(w io.Writer, err error) {
		fmt.Printf("  [Runtime Handler] Error: %v\n", err)
	})

	// This write fails -> the runtime handler is invoked
	logger.Info("This write fails and triggers the handler")

	// Flush to ensure all data is written
	_ = logger.Flush() // best-effort flush in demo

	fmt.Println("  Errors captured by handler")
}

// failingWriter is an io.Writer that always returns an error, used to
// demonstrate WriteErrorHandler invocation.
type failingWriter struct{}

func (failingWriter) Write(p []byte) (int, error) {
	return 0, fmt.Errorf("simulated write failure (disk full)")
}
