package dd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cybergodev/dd/internal"
)

// standardStreams caches the original standard stream pointers at init time.
// This avoids reading the global os.Stdout/os.Stderr/os.Stdin variables in
// background goroutines, which would race with tests that modify those globals.
var standardStreams = struct {
	stdin  *os.File
	stdout *os.File
	stderr *os.File
}{
	stdin:  os.Stdin,
	stdout: os.Stdout,
	stderr: os.Stderr,
}

// closeWriter safely closes a writer if it implements io.Closer.
// Standard streams (os.Stdout, os.Stderr, os.Stdin) are never closed.
// Uses cached stream pointers to avoid data races with tests that modify os.Stdout.
// Returns the error from Close() if one occurs, nil otherwise.
// A panic raised by the writer's Close method is recovered and returned as an error.
func closeWriter(w io.Writer) error {
	if w == nil {
		return nil
	}
	// Never close standard streams (use cached pointers to avoid reading globals)
	if w == standardStreams.stdin || w == standardStreams.stdout || w == standardStreams.stderr {
		return nil
	}
	if closer, ok := w.(io.Closer); ok {
		return safeClose(closer)
	}
	return nil
}

// safeClose calls a writer's Close with panic recovery.
// SEC-003: closeWriter is reached from Logger.Close/MultiWriter.Close, public
// entry points with no surrounding recover, so a panicking writer must not
// crash the caller — it becomes a regular close error instead (mirrors
// Logger.safeWrite).
func safeClose(c io.Closer) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("close panic: %v", r)
		}
	}()
	return c.Close()
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
	closed      atomic.Bool

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// onRotate is called after successful file rotation.
	// Used to trigger HookOnRotate in the Logger.
	onRotate func(path string)
}

// FileWriterConfig configures file writer behavior including rotation settings.
// Use DefaultFileWriterConfig() to obtain a Config with sensible defaults.
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
// Use DefaultFileWriterConfig() to obtain a Config with sensible defaults.
//
// Returns errors:
//   - ErrEmptyFilePath: when path is empty
//   - ErrNullByte: when path contains null bytes
//   - ErrPathTooLong: when path exceeds 4096 bytes
//   - ErrPathTraversal: when path contains directory traversal sequences
//   - ErrInvalidPath: when path is otherwise invalid
//   - ErrSymlinkNotAllowed: when path points to a symlink
//   - ErrOverlongEncoding: when path contains overlong UTF-8 encoding
//   - ErrMaxSizeExceeded: when MaxSizeMB exceeds 10240
//   - ErrMaxBackupsExceeded: when MaxBackups exceeds 1000
//
// Example:
//
//	cfg := dd.DefaultFileWriterConfig()
//	cfg.MaxSizeMB = 50
//	fw, err := dd.NewFileWriter("logs/app.log", cfg)
func NewFileWriter(path string, cfg FileWriterConfig) (*FileWriter, error) {
	return newFileWriterWithConfig(path, cfg)
}

func newFileWriterWithConfig(path string, config FileWriterConfig) (*FileWriter, error) {
	securePath, err := internal.ValidateAndSecurePath(path, maxPathLength, ErrEmptyFilePath, ErrNullByte, ErrPathTooLong, ErrPathTraversal, ErrInvalidPath, ErrOverlongEncoding)
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

	file, size, err := internal.OpenFile(securePath, ErrSymlinkNotAllowed, ErrHardlinkNotAllowed)
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

	// Normalize negative cleanup parameters to zero BEFORE the default matrix
	// below: a negative value that fell through untouched would disable both
	// retention policies (cleanup needs MaxAge > 0 && MaxBackups > 0) and let
	// rotated backups accumulate without bound — the disk exhaustion the
	// maxBackupCount limit exists to prevent. Zeroed values pick up the
	// documented defaults instead.
	if config.MaxAge < 0 {
		config.MaxAge = 0
	}
	if config.MaxBackups < 0 {
		config.MaxBackups = 0
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
// Returns os.ErrClosed if the writer has been closed.
//
// If a rotation happens, the onRotate callback is invoked AFTER fw.mu is
// released: the callback runs user hooks (HookOnRotate), and a hook that logs
// to the same logger or manages its writers would otherwise re-enter Write
// (fw.mu is not reentrant) or contend Logger.writersMu while a concurrent
// Logger.Close holds it and waits on fw.mu — either way a deadlock.
func (fw *FileWriter) Write(p []byte) (int, error) {
	if fw.closed.Load() {
		return 0, os.ErrClosed
	}

	pLen := len(p)
	if pLen == 0 {
		return 0, nil
	}

	fw.mu.Lock()

	if fw.file == nil {
		fw.mu.Unlock()
		return 0, os.ErrClosed
	}

	rotated := false
	if internal.NeedsRotation(fw.currentSize.Load(), int64(pLen), fw.maxSize) {
		if err := fw.rotate(); err != nil {
			fw.mu.Unlock()
			return 0, fmt.Errorf("rotation failed: %w", err)
		}
		rotated = true
	}

	n, err := fw.file.Write(p)
	if err == nil {
		fw.currentSize.Add(int64(n))
	}

	// Snapshot the callback and path under the lock, fire after unlocking.
	onRotate, rotatedPath := fw.onRotate, fw.path
	fw.mu.Unlock()

	if rotated && onRotate != nil {
		onRotate(rotatedPath)
	}

	if err != nil {
		return n, fmt.Errorf("write failed: %w", err)
	}
	return n, nil
}

// Close stops cleanup goroutines and closes the underlying file.
// Returns os.ErrClosed if already closed. Safe to call multiple times.
func (fw *FileWriter) Close() error {
	if !fw.closed.CompareAndSwap(false, true) {
		return nil
	}

	fw.cancel()

	// Hold mu across wg.Wait() so that no concurrent Write can enter rotate()
	// (which calls wg.Add to launch a compress goroutine) after Wait returns.
	// Without this, a Write that raced past the unlocked closed.Load() check
	// could wg.Add after Wait — an Add-after-Wait, which the sync docs define
	// as incorrect — and leave a compress goroutine running past Close.
	// This is deadlock-free: every goroutine that calls wg.Done() (compressBackup,
	// cleanupRoutine) does so without acquiring fw.mu.
	fw.mu.Lock()
	defer fw.mu.Unlock()

	fw.wg.Wait()

	if fw.file != nil {
		err := fw.file.Close()
		fw.file = nil
		return err
	}
	return nil
}

// SetOnRotateCallback sets a callback that is called after successful file rotation.
// This is used internally by Logger to trigger HookOnRotate events.
// The callback is stored under the writer mutex because rotate() reads it while
// holding the same mutex; setting it without the lock would race active rotations.
func (fw *FileWriter) SetOnRotateCallback(fn func(path string)) {
	fw.mu.Lock()
	defer fw.mu.Unlock()
	fw.onRotate = fn
}

func (fw *FileWriter) rotate() error {
	if fw.file != nil {
		if err := fw.file.Close(); err != nil {
			return fmt.Errorf("close file during rotation: %w", err)
		}
		fw.file = nil
	}

	nextIndex := internal.FindNextBackupIndex(fw.path)
	backupPath := internal.GetBackupPath(fw.path, nextIndex, false)

	if err := os.Rename(fw.path, backupPath); err != nil {
		// Rename failed, try to reopen the original file
		file, size, reopenErr := internal.OpenFile(fw.path, ErrSymlinkNotAllowed, ErrHardlinkNotAllowed)
		if reopenErr != nil {
			return fmt.Errorf("rename to backup failed and cannot reopen file: rename=%w, reopen=%w", err, reopenErr)
		}
		fw.file = file
		fw.currentSize.Store(size)
		return fmt.Errorf("rename to backup: %w", err)
	}

	// Rename succeeded, now open new file using O_EXCL to prevent symlink TOCTOU attacks
	// If this fails, we need to handle it carefully to avoid data loss
	file, size, err := internal.OpenFileExclusive(fw.path, ErrSymlinkNotAllowed, ErrHardlinkNotAllowed)
	if err != nil {
		// Try to recover by renaming backup back to original
		if renameBackErr := os.Rename(backupPath, fw.path); renameBackErr != nil {
			// Recovery failed - this is a critical error
			// Log to stderr as we cannot return this error without losing the rotation error
			fmt.Fprintf(os.Stderr, "dd: CRITICAL - failed to open new log file and failed to recover backup: open=%v, recover=%v\n", err, renameBackErr)
			return fmt.Errorf("open new file failed and recovery failed: open=%w, recovery=%w", err, renameBackErr)
		}
		// Recovery succeeded, try to reopen the original file
		file, size, reopenErr := internal.OpenFile(fw.path, ErrSymlinkNotAllowed, ErrHardlinkNotAllowed)
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

	// The onRotate callback is NOT invoked here: rotate() runs under fw.mu,
	// and the callback must fire outside it (see Write).
	return nil
}

func (fw *FileWriter) compressBackup(path string) {
	defer fw.wg.Done()
	// SEC-003: background goroutine with no caller that could recover a
	// panic; a compression failure here must not crash the host process.
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "dd: compress panic %s: %v\n", path, r)
		}
	}()
	if err := internal.CompressFile(path); err != nil {
		fmt.Fprintf(os.Stderr, "dd: compress backup %s: %v\n", path, err)
	}
}

func (fw *FileWriter) cleanupRoutine() {
	defer fw.wg.Done()
	// SEC-003: background goroutine with no caller that could recover a
	// panic; an internal failure here must not crash the host process.
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "dd: cleanup panic %s: %v\n", fw.path, r)
		}
	}()
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

// NewBufferedWriter creates a new BufferedWriter with the specified configuration.
// The writer automatically flushes when the buffer is half full or at the configured interval.
// Remember to call Close() to ensure all buffered data is written to the underlying writer.
// Use DefaultBufferedWriterConfig() to obtain a Config with sensible defaults.
//
// Returns errors:
//   - ErrNilWriter: when the underlying writer is nil
//   - ErrBufferSizeTooLarge: when BufferSize exceeds 10MB
//
// Example:
//
//	cfg := dd.DefaultBufferedWriterConfig()
//	cfg.BufferSize = 4096
//	bw, err := dd.NewBufferedWriter(fileWriter, cfg)
func NewBufferedWriter(w io.Writer, cfg BufferedWriterConfig) (*BufferedWriter, error) {
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
	if bw.closed.Load() {
		return 0, os.ErrClosed
	}
	pLen := len(p)
	if pLen == 0 {
		return 0, nil
	}

	bw.mu.Lock()
	defer bw.mu.Unlock()

	// Re-check under the lock: Close() flushes and closes the underlying
	// writer between lock acquisitions, so a Write that raced past the unlocked
	// check above would otherwise buffer data that no one will ever flush
	// (silent data loss). Failing cleanly here mirrors FileWriter.Write.
	if bw.closed.Load() {
		return 0, os.ErrClosed
	}

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
			if bw.autoFlushTick() {
				return
			}
		}
	}
}

// autoFlushTick flushes the buffer when the flush interval has elapsed,
// reporting whether the routine should stop (writer closed). It runs on the
// autoFlush goroutine with no caller that could recover a panic: the
// underlying writer is caller-supplied, and a panicking Write there must
// neither crash the host process (SEC-003) nor leave mu locked; a lock
// abandoned mid-panic would deadlock every later Write on this writer. The
// deferred unlock pairs with the recover below for that reason; a panic is
// logged and flushing continues on the next tick.
func (bw *BufferedWriter) autoFlushTick() (stop bool) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "dd: autoflush panic: %v\n", r)
		}
	}()
	bw.mu.Lock()
	defer bw.mu.Unlock()
	if bw.closed.Load() {
		return true
	}
	if bw.buffer.Buffered() > 0 && time.Since(bw.lastFlush) >= bw.flushTime {
		if err := bw.buffer.Flush(); err != nil {
			fmt.Fprintf(os.Stderr, "dd: autoflush error: %v\n", err)
		}
		bw.lastFlush = time.Now()
	}
	return false
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

// errWriterListClosed is returned by the writer-list helpers when the list's
// atomic pointer is nil — the state Logger's close path leaves behind after
// swapping the writers out for closing. Owners translate it into their own
// closed-state sentinel so public error contracts stay unchanged.
var errWriterListClosed = errors.New("writer list closed")

// addToWriterList appends w to the copy-on-write writer list at ptr with an
// atomic pointer swap. The caller MUST hold the mutex that serializes
// mutations of the list (Logger.writersMu / MultiWriter.mu): loads and stores
// are atomic individually, but the read-modify-write is only race-free under
// that lock. dedupe makes an already-present writer a no-op (MultiWriter
// semantics; Logger allows duplicates). Returns ErrMaxWritersExceeded at
// maxWriterCount, or errWriterListClosed when the list has been taken.
//
// Shared by Logger.AddWriter and MultiWriter.AddWriter so the slice surgery
// exists in one place.
func addToWriterList(ptr *atomic.Pointer[[]io.Writer], w io.Writer, dedupe bool) error {
	current := ptr.Load()
	if current == nil {
		return errWriterListClosed
	}
	writers := *current
	if dedupe && slices.Contains(writers, w) {
		return nil // already present, not an error
	}
	if len(writers) >= maxWriterCount {
		return ErrMaxWritersExceeded
	}
	next := make([]io.Writer, len(writers)+1)
	copy(next, writers)
	next[len(writers)] = w
	ptr.Store(&next)
	return nil
}

// removeFromWriterList deletes the first occurrence of w from the list at ptr
// with an atomic pointer swap. The caller MUST hold the mutation lock (see
// addToWriterList). Returns ErrWriterNotFound when w is absent, or
// errWriterListClosed when the list has been taken.
//
// Shared by Logger.RemoveWriter and MultiWriter.RemoveWriter.
func removeFromWriterList(ptr *atomic.Pointer[[]io.Writer], w io.Writer) error {
	current := ptr.Load()
	if current == nil {
		return errWriterListClosed
	}
	writers := *current
	for i := range len(writers) {
		if writers[i] == w {
			next := make([]io.Writer, len(writers)-1)
			copy(next, writers[:i])
			copy(next[i:], writers[i+1:])
			ptr.Store(&next)
			return nil
		}
	}
	return ErrWriterNotFound
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
// Returns an error wrapping os.ErrClosed after Close, mirroring FileWriter and
// BufferedWriter: a closed writer must fail fast rather than silently accept
// data into writers that were never Closer (e.g. bytes.Buffer).
func (mw *MultiWriter) Write(p []byte) (int, error) {
	if mw.closed.Load() {
		return 0, fmt.Errorf("cannot write to closed MultiWriter: %w", os.ErrClosed)
	}

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
		n, err := writers[0].Write(p)
		if err == nil && n != pLen {
			// Mirror the multi-writer path (and the io.Writer contract): a
			// short write without an error is reported, not silently passed on.
			return n, io.ErrShortWrite
		}
		return n, err
	}

	// Iterate directly over the immutable slice - no copy needed
	var allErrors MultiWriterError
	successCount := 0

	for i := range writerCount {
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

// AddWriter adds a writer to the MultiWriter. Adding a writer that is already
// registered is a no-op (MultiWriter deduplicates; Logger.AddWriter, by
// contrast, accepts duplicates) — pinned by TestWriterListAddDedupSemantics.
// Returns an error wrapping os.ErrClosed if the MultiWriter has been closed:
// a writer accepted after Close would never be closed itself (resource leak).
func (mw *MultiWriter) AddWriter(w io.Writer) error {
	if mw == nil {
		return ErrNilMultiWriter
	}
	if w == nil {
		return ErrNilWriter
	}

	mw.mu.Lock()
	defer mw.mu.Unlock()

	if mw.closed.Load() {
		return fmt.Errorf("cannot add writer to closed MultiWriter: %w", os.ErrClosed)
	}

	if err := addToWriterList(&mw.writersPtr, w, true); err != nil {
		if errors.Is(err, errWriterListClosed) {
			// Legacy nil-list mapping (unreachable in practice: Close
			// snapshots the list without nil-ing it).
			return ErrNilWriter
		}
		return err
	}
	return nil
}

// RemoveWriter removes a writer from the MultiWriter.
func (mw *MultiWriter) RemoveWriter(w io.Writer) error {
	if mw == nil {
		return ErrNilMultiWriter
	}

	mw.mu.Lock()
	defer mw.mu.Unlock()

	if err := removeFromWriterList(&mw.writersPtr, w); err != nil {
		if errors.Is(err, errWriterListClosed) {
			// Legacy nil-list mapping (unreachable in practice: Close
			// snapshots the list without nil-ing it).
			return ErrWriterNotFound
		}
		return err
	}
	return nil
}

// Close closes all registered writers that implement io.Closer.
func (mw *MultiWriter) Close() error {
	if !mw.closed.CompareAndSwap(false, true) {
		return nil // Already closed
	}

	// Load the writers snapshot under mu: this serializes with AddWriter, whose
	// closed check runs under the same lock. Without it, a writer accepted by a
	// concurrent AddWriter (past its closed check, before our snapshot) would
	// never be closed here — a resource leak.
	mw.mu.Lock()
	writersPtr := mw.writersPtr.Load()
	mw.mu.Unlock()
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
