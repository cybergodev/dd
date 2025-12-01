package dd

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cybergodev/dd/internal/filewriter"
)

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

func NewFileWriter(path string, config FileWriterConfig) (*FileWriter, error) {
	securePath, err := validateAndSecurePath(path)
	if err != nil {
		return nil, err
	}

	if err := validateFileWriterConfig(&config); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())

	fw := &FileWriter{
		path:       securePath,
		maxSize:    int64(config.MaxSizeMB) * 1024 * 1024,
		maxAge:     config.MaxAge,
		maxBackups: config.MaxBackups,
		compress:   config.Compress,
		ctx:        ctx,
		cancel:     cancel,
	}

	dir := filepath.Dir(securePath)
	if err := os.MkdirAll(dir, dirPermissions); err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	file, size, err := filewriter.OpenFile(securePath)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to open file %s: %w", securePath, err)
	}
	fw.file = file
	fw.currentSize.Store(size)

	if fw.maxAge > 0 {
		fw.wg.Add(1)
		go fw.cleanupRoutine()
	}

	return fw, nil
}

const (
	maxPathLength      = 4096
	maxFileSizeMB      = 10240 // 10GB
	maxBackupCount     = 1000
	defaultMaxSizeMB   = 100
	defaultMaxAge      = 30 * 24 * time.Hour
	defaultMaxBackups  = 10
	defaultBufferSize  = 1024
	maxBufferSize      = 10 * 1024 * 1024 // 10MB
	autoFlushThreshold = 2                // buffer_size / 2
	autoFlushInterval  = 100 * time.Millisecond
	dirPermissions     = 0700
)

func validateAndSecurePath(path string) (string, error) {
	if path == "" {
		return "", ErrEmptyFilePath
	}

	if strings.Contains(path, "\x00") {
		return "", ErrNullByte
	}

	if len(path) > maxPathLength {
		return "", fmt.Errorf("%w (max %d characters)", ErrPathTooLong, maxPathLength)
	}

	cleanPath := filepath.Clean(path)
	if strings.Contains(cleanPath, "..") {
		return "", ErrPathTraversal
	}

	absPath, err := filepath.Abs(cleanPath)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrInvalidPath, err)
	}

	return absPath, nil
}

func validateFileWriterConfig(config *FileWriterConfig) error {
	if config.MaxSizeMB <= 0 {
		config.MaxSizeMB = defaultMaxSizeMB
	}
	if config.MaxAge <= 0 {
		config.MaxAge = defaultMaxAge
	}
	if config.MaxBackups < 0 {
		config.MaxBackups = defaultMaxBackups
	}

	if config.MaxSizeMB > maxFileSizeMB {
		return fmt.Errorf("%w: maximum %dMB", ErrMaxSizeExceeded, maxFileSizeMB)
	}
	if config.MaxBackups > maxBackupCount {
		return fmt.Errorf("%w: maximum %d", ErrMaxBackupsExceeded, maxBackupCount)
	}

	return nil
}

func (fw *FileWriter) Write(p []byte) (int, error) {
	pLen := len(p)
	if pLen == 0 {
		return 0, nil
	}

	fw.mu.Lock()
	defer fw.mu.Unlock()

	if filewriter.NeedsRotation(fw.currentSize.Load(), int64(pLen), fw.maxSize) {
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

func (fw *FileWriter) Close() error {
	if fw.cancel != nil {
		fw.cancel()
	}
	fw.wg.Wait()

	fw.mu.Lock()
	defer fw.mu.Unlock()

	if fw.file != nil {
		return fw.file.Close()
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

	if err := filewriter.RotateBackups(fw.path, fw.maxBackups, fw.compress); err != nil {
		if file, size, reopenErr := filewriter.OpenFile(fw.path); reopenErr == nil {
			fw.file = file
			fw.currentSize.Store(size)
		}
		return fmt.Errorf("rotate backups: %w", err)
	}

	nextIndex := filewriter.FindNextBackupIndex(fw.path, fw.compress)
	backupPath := filewriter.GetBackupPath(fw.path, nextIndex, false)

	if err := os.Rename(fw.path, backupPath); err != nil {
		if file, size, reopenErr := filewriter.OpenFile(fw.path); reopenErr == nil {
			fw.file = file
			fw.currentSize.Store(size)
		}
		return fmt.Errorf("rename to backup: %w", err)
	}

	if fw.compress {
		fw.wg.Add(1)
		go func() {
			defer fw.wg.Done()
			if err := filewriter.CompressFile(backupPath); err != nil {
				fmt.Fprintf(os.Stderr, "dd: compress backup %s: %v\n", backupPath, err)
			}
		}()
	}

	file, size, err := filewriter.OpenFile(fw.path)
	if err != nil {
		return fmt.Errorf("open new file: %w", err)
	}
	fw.file = file
	fw.currentSize.Store(size)

	return nil
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
			filewriter.CleanupOldFiles(fw.path, fw.maxAge)
		}
	}
}

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

func NewBufferedWriter(w io.Writer, bufferSize int) (*BufferedWriter, error) {
	if w == nil {
		return nil, ErrNilWriter
	}

	if bufferSize < defaultBufferSize {
		bufferSize = defaultBufferSize
	}
	if bufferSize > maxBufferSize {
		return nil, fmt.Errorf("%w: maximum %dMB", ErrBufferSizeTooLarge, maxBufferSize/(1024*1024))
	}

	ctx, cancel := context.WithCancel(context.Background())

	bw := &BufferedWriter{
		writer:    w,
		buffer:    bufio.NewWriterSize(w, bufferSize),
		flushSize: bufferSize / autoFlushThreshold,
		flushTime: autoFlushInterval,
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
	if !bw.closed.CompareAndSwap(false, true) {
		return nil
	}

	if bw.cancel != nil {
		bw.cancel()
	}

	bw.wg.Wait()

	bw.mu.Lock()
	defer bw.mu.Unlock()

	if bw.buffer != nil {
		if err := bw.buffer.Flush(); err != nil {
			return err
		}
	}

	if closer, ok := bw.writer.(io.Closer); ok {
		return closer.Close()
	}

	return nil
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
			bw.mu.Lock()
			if bw.buffer.Buffered() > 0 && time.Since(bw.lastFlush) >= bw.flushTime {
				_ = bw.buffer.Flush()
				bw.lastFlush = time.Now()
			}
			bw.mu.Unlock()
		}
	}
}

type MultiWriter struct {
	writers []io.Writer
	mu      sync.RWMutex
}

func NewMultiWriter(writers ...io.Writer) *MultiWriter {
	var validWriters []io.Writer
	for _, w := range writers {
		if w != nil {
			validWriters = append(validWriters, w)
		}
	}

	return &MultiWriter{
		writers: validWriters,
	}
}

func (mw *MultiWriter) Write(p []byte) (int, error) {
	pLen := len(p)
	if pLen == 0 {
		return 0, nil
	}

	mw.mu.RLock()
	writerCount := len(mw.writers)
	if writerCount == 0 {
		mw.mu.RUnlock()
		return pLen, nil
	}

	writers := make([]io.Writer, writerCount)
	copy(writers, mw.writers)
	mw.mu.RUnlock()

	var firstErr error
	successCount := 0

	for i := range writerCount {
		n, err := writers[i].Write(p)
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("writer[%d]: %w", i, err)
			}
			continue
		}
		if n != pLen {
			if firstErr == nil {
				firstErr = fmt.Errorf("writer[%d]: short write (%d/%d bytes)", i, n, pLen)
			}
			continue
		}
		successCount++
	}

	if successCount == 0 {
		return 0, firstErr
	}

	if firstErr != nil {
		return pLen, fmt.Errorf("partial write failure (%d/%d succeeded): %w", successCount, writerCount, firstErr)
	}

	return pLen, nil
}

func (mw *MultiWriter) AddWriter(w io.Writer) {
	if w == nil {
		return
	}

	mw.mu.Lock()
	defer mw.mu.Unlock()

	mw.writers = append(mw.writers, w)
}

func (mw *MultiWriter) RemoveWriter(w io.Writer) {
	mw.mu.Lock()
	defer mw.mu.Unlock()

	for i := range mw.writers {
		if mw.writers[i] == w {
			mw.writers = append(mw.writers[:i], mw.writers[i+1:]...)
			break
		}
	}
}

func (mw *MultiWriter) Close() error {
	mw.mu.RLock()
	writers := make([]io.Writer, len(mw.writers))
	copy(writers, mw.writers)
	mw.mu.RUnlock()

	var lastErr error
	for _, w := range writers {
		if closer, ok := w.(io.Closer); ok {
			if err := closer.Close(); err != nil {
				lastErr = err
			}
		}
	}

	return lastErr
}
