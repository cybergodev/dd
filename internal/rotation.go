package internal

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// MaxDecompressSize limits the maximum bytes to read during gzip verification.
// This protects against decompression bombs (zip bombs) that could exhaust memory.
const MaxDecompressSize = 100 * 1024 * 1024 // 100MB

// OpenFile opens a log file for appending with security checks.
// symlinkErr and hardlinkErr are caller-provided sentinel errors so that
// errors.Is() matching works correctly in the calling package.
func OpenFile(path string, symlinkErr, hardlinkErr error) (*os.File, int64, error) {
	// Open file first to get a file handle, then validate the handle (not the path)
	// to prevent TOCTOU (time-of-check-time-of-use) vulnerabilities.
	// We use O_APPEND to ensure atomic appends on POSIX systems.
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, FilePermissions)
	if err != nil {
		return nil, 0, fmt.Errorf("open file: %w", err)
	}

	// Immediately validate the file handle (not the path) to prevent TOCTOU
	fileInfo, err := file.Stat()
	if err != nil {
		_ = file.Close() // best-effort close on error path
		return nil, 0, fmt.Errorf("stat file: %w", err)
	}

	// Check if the opened file is a symlink using its file handle
	// This prevents TOCTOU attacks where the path could be changed between check and use
	if fileInfo.Mode()&os.ModeSymlink != 0 {
		_ = file.Close() // best-effort close on error path
		return nil, 0, symlinkErr
	}

	// Check if the file has multiple hard links
	// Attackers can create hard links to redirect log output to sensitive files
	// or to bypass log rotation by having the same file accessible from multiple paths
	isHardlinked, err := isHardlink(file)
	if err != nil {
		_ = file.Close() // best-effort close on error path
		return nil, 0, fmt.Errorf("check hardlink: %w", err)
	}
	if isHardlinked {
		_ = file.Close() // best-effort close on error path
		return nil, 0, hardlinkErr
	}

	return file, fileInfo.Size(), nil
}

func NeedsRotation(currentSize, writeSize, maxSize int64) bool {
	return maxSize > 0 && currentSize+writeSize > maxSize
}

func RotateBackups(basePath string, maxBackups int, compress bool) {
	nextIndex := FindNextBackupIndex(basePath, compress)

	if maxBackups > 0 && nextIndex > maxBackups {
		cleanupExcessBackups(basePath, maxBackups, compress)
	}
}

type backupFileInfo struct {
	name  string
	index int
}

type backupPattern struct {
	dir      string
	prefix   string
	pattern  string
	suffix   string
	baseName string
	ext      string
}

func buildBackupPattern(basePath string, compress bool) backupPattern {
	dir := filepath.Dir(basePath)
	baseName := filepath.Base(basePath)
	ext := filepath.Ext(baseName)
	baseNameWithoutExt := strings.TrimSuffix(baseName, ext)

	suffix := ""
	if compress {
		suffix = ".gz"
	}

	prefix := baseNameWithoutExt + "_" + strings.TrimPrefix(ext, ".")
	pattern := prefix + "_%d" + ext + suffix

	return backupPattern{
		dir:      dir,
		prefix:   prefix,
		pattern:  pattern,
		suffix:   suffix,
		baseName: baseName,
		ext:      ext,
	}
}

func FindNextBackupIndex(basePath string, compress bool) int {
	bp := buildBackupPattern(basePath, compress)

	entries, err := os.ReadDir(bp.dir)
	if err != nil {
		return 1
	}

	maxIndex := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasPrefix(name, bp.prefix+"_") {
			continue
		}

		var index int
		if _, err := fmt.Sscanf(name, bp.pattern, &index); err == nil && index > maxIndex {
			maxIndex = index
		}
	}

	return maxIndex + 1
}

func cleanupExcessBackups(basePath string, maxBackups int, compress bool) {
	bp := buildBackupPattern(basePath, compress)

	entries, err := os.ReadDir(bp.dir)
	if err != nil {
		return
	}

	backups := make([]backupFileInfo, 0, 16)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasPrefix(name, bp.prefix+"_") {
			continue
		}

		var index int
		if _, err := fmt.Sscanf(name, bp.pattern, &index); err == nil {
			backups = append(backups, backupFileInfo{name: name, index: index})
		}
	}

	excessCount := len(backups) - maxBackups
	if excessCount <= 0 {
		return
	}

	sort.Slice(backups, func(i, j int) bool {
		return backups[i].index < backups[j].index
	})

	for i := 0; i < excessCount; i++ {
		filePath := filepath.Join(bp.dir, backups[i].name)
		// Intentionally ignore removal errors - this is a cleanup operation
		// and failure shouldn't affect the main logging functionality.
		// The file may have been removed by another process or be locked.
		// Error is intentionally ignored - this is best-effort cleanup.
		// Failure is not actionable (file no longer exists, was locked, etc.).
		_ = os.Remove(filePath)
	}
}

func GetBackupPath(basePath string, index int, compress bool) string {
	bp := buildBackupPattern(basePath, compress)
	baseNameWithoutExt := strings.TrimSuffix(bp.baseName, bp.ext)
	filename := fmt.Sprintf("%s_%s_%d%s%s", baseNameWithoutExt, strings.TrimPrefix(bp.ext, "."), index, bp.ext, bp.suffix)
	return filepath.Join(bp.dir, filename)
}

func CompressFile(filePath string) error {
	src, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer func() { _ = src.Close() }() // best-effort cleanup on defer

	tmpPath := filePath + ".gz.tmp"
	dst, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, FilePermissions)
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	defer func() { _ = dst.Close() }() // best-effort cleanup on defer

	gw := gzip.NewWriter(dst)
	defer func() { _ = gw.Close() }() // best-effort cleanup on defer

	if _, err := io.Copy(gw, src); err != nil {
		return fmt.Errorf("copy data: %w", err)
	}

	if err := gw.Close(); err != nil {
		return fmt.Errorf("gzip close: %w", err)
	}

	if err := dst.Close(); err != nil {
		return fmt.Errorf("dst close: %w", err)
	}

	if err := src.Close(); err != nil {
		return fmt.Errorf("src close: %w", err)
	}

	if err := verifyGzipFile(tmpPath); err != nil {
		_ = os.Remove(tmpPath) // best-effort cleanup on error
		return fmt.Errorf("verify: %w", err)
	}

	finalPath := filePath + ".gz"
	removeWithRetry(finalPath, RetryAttempts, RetryDelay)
	if err := os.Rename(tmpPath, finalPath); err != nil {
		_ = os.Remove(tmpPath) // best-effort cleanup on error
		return fmt.Errorf("rename: %w", err)
	}

	removeWithRetry(filePath, RetryAttempts, RetryDelay)
	return nil
}

func removeWithRetry(path string, attempts int, delay time.Duration) bool {
	for i := 0; i < attempts; i++ {
		if err := os.Remove(path); err == nil {
			return true
		}
		if i < attempts-1 {
			time.Sleep(delay)
		}
	}
	return false
}

func verifyGzipFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	defer func() { _ = f.Close() }() // best-effort cleanup on defer

	gr, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("gzip reader: %w", err)
	}
	defer func() { _ = gr.Close() }() // best-effort cleanup on defer

	// Limit bytes read to prevent decompression bombs
	limited := io.LimitReader(gr, MaxDecompressSize)
	_, err = io.Copy(io.Discard, limited)
	if err != nil {
		return fmt.Errorf("decompress: %w", err)
	}

	return nil
}

func CleanupOldFiles(basePath string, maxAge time.Duration) error {
	if maxAge <= 0 {
		return nil
	}

	cutoff := time.Now().Add(-maxAge)
	dir := filepath.Dir(basePath)
	baseName := filepath.Base(basePath)
	ext := filepath.Ext(baseName)
	baseNameWithoutExt := strings.TrimSuffix(baseName, ext)

	prefix := baseNameWithoutExt + "_" + strings.TrimPrefix(ext, ".")

	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read directory: %w", err)
	}

	var firstErr error
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		fileName := entry.Name()
		if !strings.HasPrefix(fileName, prefix+"_") || fileName == baseName {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("get file info %s: %w", fileName, err)
			}
			continue
		}

		if info.ModTime().Before(cutoff) {
			filePath := filepath.Join(dir, fileName)
			if removeErr := os.Remove(filePath); removeErr != nil && firstErr == nil {
				firstErr = fmt.Errorf("remove %s: %w", filePath, removeErr)
			}
		}
	}

	return firstErr
}
