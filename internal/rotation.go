package internal

import (
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// MaxDecompressSize limits the maximum bytes to read during gzip verification.
// This protects against decompression bombs (zip bombs) that could exhaust memory.
const MaxDecompressSize = 100 * 1024 * 1024 // 100MB

// OpenFileExclusive opens a log file with O_EXCL to prevent symlink TOCTOU attacks.
// Use this when reopening after rotation to prevent an attacker from placing a symlink
// between the rename and the open.
//
// If the exclusive create fails because something already exists at the path
// (e.g. a concurrent writer recreated the log file), the call falls back to
// OpenFile — which performs the full lstat/exclusive-create/SameFile symlink
// validation itself, so the fallback cannot open through a symlink either.
func OpenFileExclusive(path string, symlinkErr, hardlinkErr error) (*os.File, int64, error) {
	// Try exclusive create first — fails if file (or symlink) already exists
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, FilePermissions)
	if err != nil {
		// Exclusive create failed (file exists or symlink present) — fall back
		// to the hardened append-open, which re-validates the path end to end.
		return OpenFile(path, symlinkErr, hardlinkErr)
	}

	// The file was created by this very call, so no symlink check is needed
	// here: a symlink swapped in after the create cannot affect the handle we
	// already hold (writes follow the descriptor, not the path), and the next
	// open of the path goes through OpenFile's validation. Verify only the
	// hardlink count, which a concurrent attacker could still have raised.
	isHardlinked, err := isHardlink(file)
	if err != nil {
		_ = file.Close()
		return nil, 0, fmt.Errorf("check hardlink: %w", err)
	}
	if isHardlinked {
		_ = file.Close()
		return nil, 0, hardlinkErr
	}

	return file, 0, nil // New file, size is 0
}

// OpenFile opens a log file for appending with security checks.
// symlinkErr and hardlinkErr are caller-provided sentinel errors so that
// errors.Is() matching works correctly in the calling package.
//
// SECURITY (symlink defense, two phases, portable across Unix and Windows):
//
//  1. The PATH is lstat'ed before opening. os.Lstat does not follow symlinks,
//     so a path that IS a symlink is rejected before any file is opened. When
//     the path does not exist yet, the open uses O_CREATE|O_EXCL: O_EXCL
//     refuses to open (or create through) anything that appears at the path
//     in the lstat-to-open window, including a racing symlink — a plain
//     O_CREATE open through a dangling symlink would instead create the
//     attacker's target file.
//
//  2. After the open, the opened handle (fstat) is compared against a fresh
//     lstat of the path with os.SameFile. If the path was swapped for a
//     symlink (or any different file) while the open was in flight, the
//     identity comparison fails and the handle is rejected.
//
// The previous check — ModeSymlink on file.Stat() — could never fire: fstat
// on a descriptor obtained by following a symlink reports the TARGET's
// regular-file mode, never the link's.
func OpenFile(path string, symlinkErr, hardlinkErr error) (*os.File, int64, error) {
	// Phase 1: resolve the path without following it. fs.ErrNotExist is the
	// only tolerated lstat failure (a genuinely new log file); anything else
	// (permission denied, I/O error, ...) fails the open outright.
	pathInfo, lerr := os.Lstat(path)
	if lerr != nil && !errors.Is(lerr, fs.ErrNotExist) {
		return nil, 0, fmt.Errorf("lstat file: %w", lerr)
	}
	newFile := lerr != nil
	if !newFile && pathInfo.Mode()&os.ModeSymlink != 0 {
		return nil, 0, symlinkErr
	}

	// Open the file handle. O_APPEND preserves atomic appends on POSIX.
	// For an existing regular file no O_CREATE is passed: if the file is
	// removed and replaced by a symlink in the lstat-to-open window, the open
	// then either fails (dangling target) or follows the symlink and is
	// rejected by the phase-2 recheck — it can never create through it.
	// For a new file, O_EXCL keeps the create exclusive: nothing may appear
	// at the path between the lstat and the open.
	flag := os.O_WRONLY | os.O_APPEND
	if newFile {
		flag |= os.O_CREATE | os.O_EXCL
	}
	file, err := os.OpenFile(path, flag, FilePermissions)
	if err != nil {
		return nil, 0, fmt.Errorf("open file: %w", err)
	}

	fileInfo, err := file.Stat()
	if err != nil {
		_ = file.Close() // best-effort close on error path
		return nil, 0, fmt.Errorf("stat file: %w", err)
	}

	// Phase 2: the path must still name the file we opened. os.SameFile
	// compares inode identity (dev/ino on Unix, volume+file-index on
	// Windows); a symlink swapped in during the open window is caught either
	// by its ModeSymlink (lstat does not follow it) or by the identity
	// mismatch with the descriptor's fstat. An lstat ENOENT here means the
	// path was removed after our open: the held handle is still the file we
	// validated, so writing may continue and the next rotation recreates the
	// path. Other transient lstat errors are ignored — the phase-1 gate is
	// the authoritative symlink rejection.
	if recheck, rerr := os.Lstat(path); rerr == nil {
		if recheck.Mode()&os.ModeSymlink != 0 || !os.SameFile(recheck, fileInfo) {
			_ = file.Close() // best-effort close on error path
			return nil, 0, symlinkErr
		}
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
	nextIndex := FindNextBackupIndex(basePath)

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

// FindNextBackupIndex returns the next unused backup index for basePath.
//
// The scan always parses with the UNCOMPRESSED pattern: fmt.Sscanf tolerates
// trailing input, so "app_log_5.log" also matches a name like
// "app_log_5.log.gz". Parsing with the compressed pattern in compress mode
// would hide an in-flight uncompressed backup (compression runs on a
// background goroutine) — the next rotation would then reuse its index and
// os.Rename would clobber the file the compressor is about to read (data
// loss on POSIX) or fail outright (Windows refuses rename onto an existing
// file). The uncompressed pattern matches both suffixes and never reuses a
// live index.
func FindNextBackupIndex(basePath string) int {
	bp := buildBackupPattern(basePath, false)

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
		// Best-effort cleanup with retry: on Windows a briefly-locked backup
		// (scanner, indexer) fails a bare os.Remove and would silently never
		// be cleaned up — the same condition removeWithRetry exists for in
		// CompressFile. Failures are still not actionable here.
		removeWithRetry(filePath, RetryAttempts, RetryDelay)
	}
}

func GetBackupPath(basePath string, index int, compress bool) string {
	bp := buildBackupPattern(basePath, compress)
	// The pattern already encodes the exact name shape ("<base>_<ext>_<n><ext><suffix>");
	// formatting through it keeps this the single definition of the naming scheme.
	return filepath.Join(bp.dir, fmt.Sprintf(bp.pattern, index))
}

func CompressFile(filePath string) error {
	src, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer func() { _ = src.Close() }() // best-effort cleanup on defer

	tmpPath := filePath + ".gz.tmp"

	// removeTmpOnExit stays true until the final rename is attempted. Error
	// paths before that point used to leak a partial .gz.tmp per attempt
	// (nothing else matches that name). Once the pre-rename removal of
	// finalPath has happened, the temp is the ONLY surviving copy of the
	// backup's data and must be kept even if the rename fails — the next
	// compression attempt truncates and reuses the same temp path.
	removeTmpOnExit := true
	defer func() {
		if removeTmpOnExit {
			_ = os.Remove(tmpPath) // best-effort cleanup on error
		}
	}()

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
		return fmt.Errorf("verify: %w", err)
	}

	finalPath := filePath + ".gz"
	removeWithRetry(finalPath, RetryAttempts, RetryDelay)
	removeTmpOnExit = false // past the pre-rename delete, the temp is the only copy
	if err := os.Rename(tmpPath, finalPath); err != nil {
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

	// Derive dir/prefix from the shared backup-pattern definition so retention
	// can never drift from the naming scheme used to create the files.
	bp := buildBackupPattern(basePath, false)
	dir := bp.dir
	baseName := bp.baseName
	prefix := bp.prefix

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
