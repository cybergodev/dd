package dd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cybergodev/dd/internal"
)

// closeWriter safely closes a writer if it implements io.Closer.
// Standard streams (os.Stdout, os.Stderr, os.Stdin) are never closed.
// Returns the error from Close() if one occurs, nil otherwise.
func closeWriter(w io.Writer) error {
	if w == nil {
		return nil
	}
	// Never close standard streams
	if w == os.Stdout || w == os.Stderr || w == os.Stdin {
		return nil
	}
	if closer, ok := w.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

// FileWriter provides thread-safe file writing with log rotation support.
// It supports size-based rotation, compression, and age-based cleanup of old log files.
type FileWriter struct {
	path       string
	maxSize    int64
	maxAge     time.Duration
	maxBackups int
	compress   bool

	mu          sync.Mutex
	file        *os.File
	currentSize atomic.Int64

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

type FileWriterConfig struct {
	MaxSizeMB  int
	MaxAge     time.Duration
	MaxBackups int
	Compress   bool
}

// DefaultFileWriterConfig returns FileWriterConfig with sensible defaults.
// Default values: MaxSizeMB=100, MaxAge=30 days, MaxBackups=10, Compress=false.
func DefaultFileWriterConfig() FileWriterConfig {
	return FileWriterConfig{
		MaxSizeMB:  DefaultMaxSizeMB,
		MaxAge:     DefaultMaxAge,
		MaxBackups: DefaultMaxBackups,
		Compress:   false,
	}
}

// Validate checks the FileWriterConfig for invalid values.
// Returns an error if MaxSizeMB or MaxBackups exceed their limits.
func (c FileWriterConfig) Validate() error {
	return validateFileWriterConfig(&c)
}

// BufferedWriterConfig configures a BufferedWriter.
// Use DefaultBufferedWriterConfig() for sensible defaults.
type BufferedWriterConfig struct {
	// BufferSize in bytes. Default: 1024 (1KB). Maximum: 10MB.
	// Values below the default are clamped to the default.
	BufferSize int
	// FlushTime is the auto-flush interval. Default: 100ms.
	// The writer flushes when the buffer is half full or after this interval.
	FlushTime time.Duration
}

// DefaultBufferedWriterConfig returns BufferedWriterConfig with sensible defaults.
// Default values: BufferSize=1KB, FlushTime=100ms.
func DefaultBufferedWriterConfig() BufferedWriterConfig {
	return BufferedWriterConfig{
		BufferSize: defaultBufferSizeKB * 1024,
		FlushTime:  autoFlushInterval,
	}
}

// Validate checks the BufferedWriterConfig for invalid values.
// Returns an error if BufferSize exceeds the maximum (10MB)
// or if FlushTime is negative.
func (c BufferedWriterConfig) Validate() error {
	if c.BufferSize < 0 {
		return fmt.Errorf("buffer size must be non-negative, got %d", c.BufferSize)
	}
	if c.BufferSize > maxBufferSizeKB*1024 {
		return fmt.Errorf("%w: maximum %dMB", ErrBufferSizeTooLarge, maxBufferSizeKB/1024)
	}
	if c.FlushTime < 0 {
		return fmt.Errorf("flush time must be non-negative, got %v", c.FlushTime)
	}
	return nil
}

// NewFileWriter creates a thread-safe file writer with rotation support.
// The file is validated for security (path traversal, null bytes, symlinks).
// If cfg fields are zero, DefaultFileWriterConfig values are applied.
//
// Example:
//
//	cfg := dd.DefaultFileWriterConfig()
//	cfg.MaxSizeMB = 50
//	fw, err := dd.NewFileWriter("logs/app.log", cfg)
func NewFileWriter(path string, cfg ...FileWriterConfig) (*FileWriter, error) {
	var config FileWriterConfig
	if len(cfg) > 0 {
		config = cfg[0]
	}
	return newFileWriterWithConfig(path, config)
}

func newFileWriterWithConfig(path string, config FileWriterConfig) (*FileWriter, error) {
	securePath, err := internal.ValidateAndSecurePath(path, maxPathLength, ErrEmptyFilePath, ErrNullByte, ErrPathTooLong, ErrPathTraversal, ErrInvalidPath)
	if err != nil {
		return nil, err
	}

	// Validate configuration (does not modify input)
	if err := validateFileWriterConfig(&config); err != nil {
		return nil, err
	}

	// Apply defaults to a local copy (preserves original config)
	effectiveConfig := applyFileWriterDefaults(config)

	ctx, cancel := context.WithCancel(context.Background())

	fw := &FileWriter{
		path:       securePath,
		maxSize:    int64(effectiveConfig.MaxSizeMB) * 1024 * 1024,
		maxAge:     effectiveConfig.MaxAge,
		maxBackups: effectiveConfig.MaxBackups,
		compress:   effectiveConfig.Compress,
		ctx:        ctx,
		cancel:     cancel,
	}

	dir := filepath.Dir(securePath)
	if err := os.MkdirAll(dir, dirPermissions); err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	file, size, err := internal.OpenFile(securePath)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to open file %s: %w", securePath, err)
	}
	fw.file = file
	fw.currentSize.Store(size)

	if fw.maxAge > 0 && fw.maxBackups > 0 {
		fw.wg.Add(1)
		go fw.cleanupRoutine()
	}

	return fw, nil
}

// validateFileWriterConfig validates the configuration without modifying it.
// Returns an error if the configuration contains invalid values.
func validateFileWriterConfig(config *FileWriterConfig) error {
	// Validate limits (negative values are allowed and will use defaults)
	if config.MaxSizeMB > maxFileSizeMB {
		return fmt.Errorf("%w: maximum %dMB", ErrMaxSizeExceeded, maxFileSizeMB)
	}
	if config.MaxBackups > maxBackupCount {
		return fmt.Errorf("%w: maximum %d", ErrMaxBackupsExceeded, maxBackupCount)
	}

	return nil
}

// applyFileWriterDefaults applies default values to a copy of the configuration.
// This ensures the original config is not modified.
func applyFileWriterDefaults(config FileWriterConfig) FileWriterConfig {
	// Apply defaults for zero/negative MaxSizeMB
	if config.MaxSizeMB <= 0 {
		config.MaxSizeMB = DefaultMaxSizeMB
	}

	// Cleanup is enabled only when at least one cleanup parameter is configured.
	// Apply defaults based on what the user has specified:
	// - Both zero: use full defaults (MaxAge + MaxBackups)
	// - Only MaxBackups set: use count-based cleanup only (MaxAge = 0)
	// - Only MaxAge set: use time-based cleanup with default MaxBackups
	if config.MaxAge == 0 && config.MaxBackups == 0 {
		config.MaxAge = DefaultMaxAge
		config.MaxBackups = DefaultMaxBackups
	} else if config.MaxAge > 0 && config.MaxBackups == 0 {
		// User set MaxAge but not MaxBackups - use default MaxBackups
		config.MaxBackups = DefaultMaxBackups
	}
	// When MaxBackups > 0 and MaxAge == 0: count-based cleanup only (no change needed)

	return config
}

// Write writes data to the log file. Triggers rotation if the file exceeds MaxSizeMB.
func (fw *FileWriter) Write(p []byte) (int, error) {
	pLen := len(p)
	if pLen == 0 {
		return 0, nil
	}

	fw.mu.Lock()
	defer fw.mu.Unlock()

	if internal.NeedsRotation(fw.currentSize.Load(), int64(pLen), fw.maxSize) {
		if err := fw.rotate(); err != nil {
			return 0, fmt.Errorf("rotation failed: %w", err)
		}
	}

	n, err := fw.file.Write(p)
	if err != nil {
		return n, fmt.Errorf("write failed: %w", err)
	}

	fw.currentSize.Add(int64(n))
	return n, nil
}

// Close stops cleanup goroutines and closes the underlying file.
func (fw *FileWriter) Close() error {
	fw.cancel()
	fw.wg.Wait()

	fw.mu.Lock()
	defer fw.mu.Unlock()

	if fw.file != nil {
		err := fw.file.Close()
		fw.file = nil
		return err
	}
	return nil
}

func (fw *FileWriter) rotate() error {
	if fw.file != nil {
		if err := fw.file.Close(); err != nil {
			return fmt.Errorf("close file during rotation: %w", err)
		}
		fw.file = nil
	}

	nextIndex := internal.FindNextBackupIndex(fw.path, fw.compress)
	backupPath := internal.GetBackupPath(fw.path, nextIndex, false)

	if err := os.Rename(fw.path, backupPath); err != nil {
		// Rename failed, try to reopen the original file
		file, size, reopenErr := internal.OpenFile(fw.path)
		if reopenErr != nil {
			return fmt.Errorf("rename to backup failed and cannot reopen file: rename=%w, reopen=%w", err, reopenErr)
		}
		fw.file = file
		fw.currentSize.Store(size)
		return fmt.Errorf("rename to backup: %w", err)
	}

	// Rename succeeded, now open new file
	// If this fails, we need to handle it carefully to avoid data loss
	file, size, err := internal.OpenFile(fw.path)
	if err != nil {
		// Try to recover by renaming backup back to original
		if renameBackErr := os.Rename(backupPath, fw.path); renameBackErr != nil {
			// Recovery failed - this is a critical error
			// Log to stderr as we cannot return this error without losing the rotation error
			fmt.Fprintf(os.Stderr, "dd: CRITICAL - failed to open new log file and failed to recover backup: open=%v, recover=%v\n", err, renameBackErr)
			return fmt.Errorf("open new file failed and recovery failed: open=%w, recovery=%w", err, renameBackErr)
		}
		// Recovery succeeded, try to reopen the original file
		file, size, reopenErr := internal.OpenFile(fw.path)
		if reopenErr != nil {
			return fmt.Errorf("open new file failed, recovery succeeded but reopen failed: open=%w, reopen=%w", err, reopenErr)
		}
		fw.file = file
		fw.currentSize.Store(size)
		return fmt.Errorf("open new file failed (recovered): %w", err)
	}
	fw.file = file
	fw.currentSize.Store(size)

	// Only perform cleanup and compression after successful file open
	internal.RotateBackups(fw.path, fw.maxBackups, fw.compress)

	if fw.compress {
		fw.wg.Add(1)
		go fw.compressBackup(backupPath)
	}

	return nil
}

func (fw *FileWriter) compressBackup(path string) {
	defer fw.wg.Done()
	if err := internal.CompressFile(path); err != nil {
		fmt.Fprintf(os.Stderr, "dd: compress backup %s: %v\n", path, err)
	}
}

func (fw *FileWriter) cleanupRoutine() {
	defer fw.wg.Done()

	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-fw.ctx.Done():
			return
		case <-ticker.C:
			if err := internal.CleanupOldFiles(fw.path, fw.maxAge); err != nil {
				// Log to stderr as fallback - cleanup errors should not be silent
				fmt.Fprintf(os.Stderr, "dd: cleanup old files %s: %v\n", fw.path, err)
			}
		}
	}
}

// BufferedWriter wraps an io.Writer with buffering capabilities.
// It automatically flushes when the buffer reaches a certain size or after a timeout.
//
// IMPORTANT: Always call Close() when done to ensure all buffered data is flushed.
// Failure to call Close() may result in data loss.
type BufferedWriter struct {
	writer    io.Writer
	buffer    *bufio.Writer
	flushSize int
	flushTime time.Duration

	mu        sync.Mutex
	ctx       context.Context
	cancel    context.CancelFunc
	lastFlush time.Time
	wg        sync.WaitGroup
	closed    atomic.Bool
}

// NewBufferedWriter creates a new BufferedWriter with the specified buffer size.
// The writer automatically flushes when the buffer is half full or every 100ms.
// Remember to call Close() to ensure all buffered data is written to the underlying writer.
// If bufferSize is not specified or is 0, 1KB is used.
func NewBufferedWriter(w io.Writer, bufferSizes ...int) (*BufferedWriter, error) {
	cfg := DefaultBufferedWriterConfig()
	if len(bufferSizes) > 0 {
		cfg.BufferSize = bufferSizes[0]
	}
	return newBufferedWriterWithConfig(w, cfg)
}

func newBufferedWriterWithConfig(w io.Writer, cfg BufferedWriterConfig) (*BufferedWriter, error) {
	if w == nil {
		return nil, ErrNilWriter
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	bufferSize := cfg.BufferSize
	if bufferSize < defaultBufferSizeKB*1024 {
		bufferSize = defaultBufferSizeKB * 1024
	}

	flushTime := cfg.FlushTime
	if flushTime <= 0 {
		flushTime = autoFlushInterval
	}

	ctx, cancel := context.WithCancel(context.Background())

	bw := &BufferedWriter{
		writer:    w,
		buffer:    bufio.NewWriterSize(w, bufferSize),
		flushSize: bufferSize / autoFlushThreshold,
		flushTime: flushTime,
		ctx:       ctx,
		cancel:    cancel,
		lastFlush: time.Now(),
	}

	bw.wg.Add(1)
	go bw.autoFlushRoutine()

	return bw, nil
}

func (bw *BufferedWriter) Write(p []byte) (int, error) {
	pLen := len(p)
	if pLen == 0 {
		return 0, nil
	}

	bw.mu.Lock()
	defer bw.mu.Unlock()

	n, err := bw.buffer.Write(p)
	if err != nil {
		return n, err
	}

	if bw.buffer.Buffered() >= bw.flushSize {
		if flushErr := bw.buffer.Flush(); flushErr != nil {
			return n, fmt.Errorf("auto-flush failed: %w", flushErr)
		}
		bw.lastFlush = time.Now()
	}

	return n, nil
}

func (bw *BufferedWriter) Flush() error {
	bw.mu.Lock()
	defer bw.mu.Unlock()

	err := bw.buffer.Flush()
	bw.lastFlush = time.Now()
	return err
}

func (bw *BufferedWriter) Close() error {
	if bw == nil {
		return nil
	}
	if !bw.closed.CompareAndSwap(false, true) {
		return nil
	}

	// Pre-allocate error slice: max 2 errors (flush + close writer)
	errs := make([]error, 0, 2)

	// Flush buffer BEFORE canceling context and stopping goroutine
	// This ensures no data is lost if the goroutine was about to flush
	bw.mu.Lock()
	if bw.buffer != nil {
		if err := bw.buffer.Flush(); err != nil {
			errs = append(errs, fmt.Errorf("flush: %w", err))
		}
	}
	bw.mu.Unlock()

	// Now stop the background goroutine
	if bw.cancel != nil {
		bw.cancel()
	}
	bw.wg.Wait()

	// Close the underlying writer
	bw.mu.Lock()
	defer bw.mu.Unlock()

	if bw.writer != nil {
		if closer, ok := bw.writer.(io.Closer); ok {
			if err := closer.Close(); err != nil {
				errs = append(errs, fmt.Errorf("close writer: %w", err))
			}
		}
	}

	return errors.Join(errs...)
}

func (bw *BufferedWriter) autoFlushRoutine() {
	defer bw.wg.Done()

	ticker := time.NewTicker(bw.flushTime)
	defer ticker.Stop()

	for {
		select {
		case <-bw.ctx.Done():
			return
		case <-ticker.C:
			// Check closed flag under lock to prevent race with Close()
			bw.mu.Lock()
			if bw.closed.Load() {
				bw.mu.Unlock()
				return
			}
			if bw.buffer.Buffered() > 0 && time.Since(bw.lastFlush) >= bw.flushTime {
				if err := bw.buffer.Flush(); err != nil {
					fmt.Fprintf(os.Stderr, "dd: autoflush error: %v\n", err)
				}
				bw.lastFlush = time.Now()
			}
			bw.mu.Unlock()
		}
	}
}

// MultiWriter distributes writes across multiple writers.
// It uses atomic pointer for lock-free reads on the write hot path.
type MultiWriter struct {
	// writersPtr stores an immutable slice of writers using atomic pointer.
	// This eliminates slice copying during write operations (hot path).
	// The slice is replaced atomically when writers are added/removed.
	writersPtr atomic.Pointer[[]io.Writer]
	mu         sync.Mutex // protects AddWriter/RemoveWriter operations
	closed     atomic.Bool
}

// NewMultiWriter creates a new MultiWriter. Nil writers are silently ignored.
func NewMultiWriter(writers ...io.Writer) *MultiWriter {
	validWriters := make([]io.Writer, 0, len(writers))
	for _, w := range writers {
		if w != nil {
			validWriters = append(validWriters, w)
		}
	}

	mw := &MultiWriter{}
	mw.writersPtr.Store(&validWriters)
	return mw
}

// Write writes data to all registered writers. Collects errors from failed writers.
func (mw *MultiWriter) Write(p []byte) (int, error) {
	pLen := len(p)
	if pLen == 0 {
		return 0, nil
	}

	// Fast path: atomic load of writers pointer (lock-free read)
	writersPtr := mw.writersPtr.Load()
	if writersPtr == nil || len(*writersPtr) == 0 {
		return pLen, nil
	}

	writers := *writersPtr
	writerCount := len(writers)

	// Fast path: single writer optimization
	if writerCount == 1 {
		return writers[0].Write(p)
	}

	// Iterate directly over the immutable slice - no copy needed
	var allErrors MultiWriterError
	successCount := 0

	for i := 0; i < writerCount; i++ {
		n, err := writers[i].Write(p)
		if err != nil {
			allErrors.addError(i, writers[i], err)
			continue
		}
		if n != pLen {
			allErrors.addError(i, writers[i], fmt.Errorf("short write (%d/%d bytes)", n, pLen))
			continue
		}
		successCount++
	}

	// If all writers failed, return error
	if successCount == 0 {
		return 0, &allErrors
	}

	// If partial success, return bytes written but include error info
	if allErrors.HasErrors() {
		return pLen, &allErrors
	}

	return pLen, nil
}

// AddWriter adds a writer to the MultiWriter. Duplicate writers are silently accepted.
func (mw *MultiWriter) AddWriter(w io.Writer) error {
	if mw == nil {
		return ErrNilMultiWriter
	}
	if w == nil {
		return ErrNilWriter
	}

	mw.mu.Lock()
	defer mw.mu.Unlock()

	// Load current writers slice
	currentWriters := mw.writersPtr.Load()
	if currentWriters == nil {
		return ErrNilWriter
	}

	// Check for duplicates
	for _, existing := range *currentWriters {
		if existing == w {
			return nil // Already exists, not an error
		}
	}

	if len(*currentWriters) >= maxWriterCount {
		return ErrMaxWritersExceeded
	}

	// Create new slice with the new writer added
	newWriters := make([]io.Writer, len(*currentWriters)+1)
	copy(newWriters, *currentWriters)
	newWriters[len(*currentWriters)] = w

	// Atomically swap the pointer
	mw.writersPtr.Store(&newWriters)
	return nil
}

// RemoveWriter removes a writer from the MultiWriter.
func (mw *MultiWriter) RemoveWriter(w io.Writer) error {
	if mw == nil {
		return ErrNilMultiWriter
	}

	mw.mu.Lock()
	defer mw.mu.Unlock()

	// Load current writers slice
	currentWriters := mw.writersPtr.Load()
	if currentWriters == nil {
		return ErrWriterNotFound
	}

	writerCount := len(*currentWriters)
	for i := 0; i < writerCount; i++ {
		if (*currentWriters)[i] == w {
			// Create new slice without the removed writer
			newWriters := make([]io.Writer, writerCount-1)
			copy(newWriters, (*currentWriters)[:i])
			copy(newWriters[i:], (*currentWriters)[i+1:])

			// Atomically swap the pointer
			mw.writersPtr.Store(&newWriters)
			return nil
		}
	}

	return ErrWriterNotFound
}

// Close closes all registered writers that implement io.Closer.
func (mw *MultiWriter) Close() error {
	if !mw.closed.CompareAndSwap(false, true) {
		return nil // Already closed
	}

	// Load writers atomically
	writersPtr := mw.writersPtr.Load()
	if writersPtr == nil {
		return nil
	}
	writers := *writersPtr

	errs := make([]error, 0, len(writers))
	for _, w := range writers {
		if err := closeWriter(w); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}
