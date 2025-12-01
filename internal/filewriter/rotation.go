package filewriter

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	maxPathLength   = 4096
	retryAttempts   = 3
	retryDelay      = 10 * time.Millisecond
	verifyBufSize   = 1024
	filePermissions = 0600
)

func OpenFile(path string) (*os.File, int64, error) {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, filePermissions)
	if err != nil {
		return nil, 0, err
	}

	stat, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, 0, err
	}

	if stat.Mode()&os.ModeSymlink != 0 {
		_ = file.Close()
		return nil, 0, fmt.Errorf("symlinks not allowed")
	}

	return file, stat.Size(), nil
}

func NeedsRotation(currentSize, writeSize, maxSize int64) bool {
	return maxSize > 0 && currentSize+writeSize > maxSize
}

func RotateBackups(basePath string, maxBackups int, compress bool) error {
	nextIndex := FindNextBackupIndex(basePath, compress)

	if maxBackups > 0 && nextIndex > maxBackups {
		cleanupExcessBackups(basePath, maxBackups, compress)
	}

	return nil
}

func FindNextBackupIndex(basePath string, compress bool) int {
	maxIndex := 0
	dir := filepath.Dir(basePath)
	baseName := filepath.Base(basePath)
	ext := filepath.Ext(baseName)
	baseNameWithoutExt := strings.TrimSuffix(baseName, ext)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return 1
	}

	suffix := ""
	if compress {
		suffix = ".gz"
	}

	prefix := baseNameWithoutExt + "_" + strings.TrimPrefix(ext, ".")

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasPrefix(name, prefix+"_") {
			continue
		}

		var index int
		pattern := prefix + "_%d" + ext + suffix
		_, err := fmt.Sscanf(name, pattern, &index)
		if err == nil && index > maxIndex {
			maxIndex = index
		}
	}

	return maxIndex + 1
}

func cleanupExcessBackups(basePath string, maxBackups int, compress bool) {
	dir := filepath.Dir(basePath)
	baseName := filepath.Base(basePath)
	ext := filepath.Ext(baseName)
	baseNameWithoutExt := strings.TrimSuffix(baseName, ext)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	suffix := ""
	if compress {
		suffix = ".gz"
	}

	prefix := baseNameWithoutExt + "_" + strings.TrimPrefix(ext, ".")

	type backupFile struct {
		name  string
		index int
	}
	var backups []backupFile

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasPrefix(name, prefix+"_") {
			continue
		}

		var index int
		pattern := prefix + "_%d" + ext + suffix
		if _, err := fmt.Sscanf(name, pattern, &index); err == nil {
			backups = append(backups, backupFile{name: name, index: index})
		}
	}

	backupCount := len(backups)
	for backupCount > maxBackups {
		minIndex := backups[0].index
		minPos := 0
		for i := 1; i < backupCount; i++ {
			if backups[i].index < minIndex {
				minIndex = backups[i].index
				minPos = i
			}
		}

		filePath := filepath.Join(dir, backups[minPos].name)
		_ = os.Remove(filePath)

		backups = append(backups[:minPos], backups[minPos+1:]...)
		backupCount--
	}
}

func GetBackupPath(basePath string, index int, compress bool) string {
	dir := filepath.Dir(basePath)
	baseName := filepath.Base(basePath)
	ext := filepath.Ext(baseName)
	baseNameWithoutExt := strings.TrimSuffix(baseName, ext)

	suffix := ""
	if compress {
		suffix = ".gz"
	}

	filename := fmt.Sprintf("%s_%s_%d%s%s", baseNameWithoutExt, strings.TrimPrefix(ext, "."), index, ext, suffix)
	return filepath.Join(dir, filename)
}

func CompressFile(filePath string) error {
	src, err := os.Open(filePath)
	if err != nil {
		return err
	}

	tmpPath := filePath + ".gz.tmp"
	dst, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, filePermissions)
	if err != nil {
		src.Close()
		return err
	}

	gw := gzip.NewWriter(dst)
	_, copyErr := io.Copy(gw, src)
	closeErr := gw.Close()
	dstCloseErr := dst.Close()
	srcCloseErr := src.Close()

	if copyErr != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("copy: %w", copyErr)
	}
	if closeErr != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("gzip close: %w", closeErr)
	}
	if dstCloseErr != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("dst close: %w", dstCloseErr)
	}
	if srcCloseErr != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("src close: %w", srcCloseErr)
	}

	if err := verifyGzipFile(tmpPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("verify: %w", err)
	}

	finalPath := filePath + ".gz"
	if _, err := os.Stat(finalPath); err == nil {
		for attempt := range retryAttempts {
			if err := os.Remove(finalPath); err == nil {
				break
			}
			if attempt < retryAttempts-1 {
				time.Sleep(retryDelay)
			}
		}
	}

	if err := os.Rename(tmpPath, finalPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename: %w", err)
	}

	for attempt := range retryAttempts {
		if err := os.Remove(filePath); err == nil {
			return nil
		}
		if attempt < retryAttempts-1 {
			time.Sleep(retryDelay)
		}
	}

	return nil
}

func verifyGzipFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	gr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gr.Close()

	buf := make([]byte, verifyBufSize)
	_, err = gr.Read(buf)
	if err != nil && err != io.EOF {
		return err
	}

	return nil
}

func CleanupOldFiles(basePath string, maxAge time.Duration) {
	if maxAge <= 0 {
		return
	}

	cutoff := time.Now().Add(-maxAge)
	dir := filepath.Dir(basePath)
	baseName := filepath.Base(basePath)

	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		fileName := filepath.Base(path)
		if strings.HasPrefix(fileName, baseName) &&
			fileName != baseName &&
			info.ModTime().Before(cutoff) {
			_ = os.Remove(path)
		}

		return nil
	})
}
