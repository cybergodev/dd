package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/cybergodev/dd"
)

// Writers - Advanced Output Management
//
// Topics covered:
// 1. FileWriter with rotation settings
// 2. Rotation in action (backup files created live)
// 3. BufferedWriter for high throughput
// 4. MultiWriter for multiple outputs
// 5. Dynamic writer management
// 6. Error handling
//
// NOTE: constructor errors are ignored (logger, _) for brevity in these
// examples; see 07_convenience for error-handling patterns.
func main() {
	fmt.Println("=== DD Writers Management ===")

	section1FileWriter()
	section2Rotation()
	section3BufferedWriter()
	section4MultiWriter()
	section5DynamicManagement()
	section6WriterErrors()

	fmt.Println("\n✅ Writers examples completed!")
}

// Section 1: FileWriter creation and configuration
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

// Section 2: Rotation in action - write past MaxSizeMB and watch the
// backup files appear (rotate.log -> rotate_log_1.log, rotate_log_2.log, ...)
func section2Rotation() {
	fmt.Println("2. Rotation in Action")
	fmt.Println("----------------------")

	// 1MB is the smallest rotation unit; keep only 2 backups for the demo.
	// Remove leftovers from previous runs so the listing below is stable.
	for _, old := range []string{"logs/rotate.log", "logs/rotate_log_1.log", "logs/rotate_log_2.log"} {
		_ = os.Remove(old) // best-effort: missing files are fine
	}
	fw, err := dd.NewFileWriter("logs/rotate.log", dd.FileWriterConfig{
		MaxSizeMB:  1,
		MaxBackups: 2,
	})
	if err != nil {
		fmt.Printf("Failed: %v\n", err)
		return
	}

	cfg := dd.DefaultConfig()
	cfg.Targets = []dd.OutputTarget{dd.CustomOutput(fw)}

	logger, _ := dd.New(cfg)
	defer fw.Close() // no-op if the logger below already closed it

	// ~600-byte lines x 4000 = ~2.4MB -> rotations at ~1MB and ~2MB
	payload := strings.Repeat("d", 512)
	for i := 0; i < 4000; i++ {
		logger.Infof("rotation demo %04d %s", i, payload)
	}

	// Close before listing so sizes are final: on Windows the directory
	// entry of a still-open file can report a stale size.
	_ = logger.Close()

	entries, err := os.ReadDir("logs")
	if err != nil {
		fmt.Printf("Failed to list logs/: %v\n", err)
		return
	}
	fmt.Println("  Files after ~2.4MB of logs (MaxSizeMB=1, MaxBackups=2):")
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "rotate") {
			info, _ := e.Info()
			fmt.Printf("    %-22s %9d bytes\n", e.Name(), info.Size())
		}
	}
	fmt.Println()
}

// Section 3: BufferedWriter for high throughput
func section3BufferedWriter() {
	fmt.Println("3. BufferedWriter (High Throughput)")
	fmt.Println("-------------------------------------")

	// Create underlying file writer
	fileWriter, err := dd.NewFileWriter("logs/buffered.log", dd.DefaultFileWriterConfig())
	if err != nil {
		fmt.Printf("Failed: %v\n", err)
		return
	}
	defer fileWriter.Close()

	// Wrap with buffer (default 1KB buffer, 100ms flush interval)
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

// Section 4: MultiWriter for multiple outputs
func section4MultiWriter() {
	fmt.Println("4. MultiWriter (Multiple Outputs)")
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

	// MultiWriter also supports dynamic membership (deduplicated: adding an
	// already-registered writer is a no-op)
	extra, err := os.Create("logs/multi-extra.log")
	if err != nil {
		fmt.Printf("Failed to create extra file: %v\n", err)
		return
	}
	if err := multiWriter.AddWriter(extra); err != nil {
		fmt.Printf("Failed to add writer: %v\n", err)
		_ = extra.Close() // best-effort cleanup on the error path
		return
	}
	logger.Info("Now on THREE outputs (console + file + extra)")

	// Remove it again so this demo alone owns the extra file's lifecycle
	// (the logger's Close would otherwise close it via MultiWriter.Close)
	multiWriter.RemoveWriter(extra)
	_ = extra.Close() // best-effort cleanup: demo owns the file now

	fmt.Println()
}

// Section 5: Dynamic writer management
func section5DynamicManagement() {
	fmt.Println("5. Dynamic Writer Management")
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

// Section 6: Writer error handling
func section6WriterErrors() {
	fmt.Println("6. Writer Error Handling")
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
