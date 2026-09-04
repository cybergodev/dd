package internal

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

var (
	testErrSymlinkNotAllowed  = errors.New("symlinks not allowed")
	testErrHardlinkNotAllowed = errors.New("hardlinks not allowed")
)

func TestOpenFile(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.log")

	// Test opening new file
	file, size, err := OpenFile(testFile, testErrSymlinkNotAllowed, testErrHardlinkNotAllowed)
	if err != nil {
		t.Fatalf("OpenFile() error = %v", err)
	}
	defer file.Close()

	if size != 0 {
		t.Errorf("New file size = %d, want 0", size)
	}

	// Write some data
	data := []byte("test data")
	n, err := file.Write(data)
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	file.Close()

	// Test opening existing file
	file2, size2, err := OpenFile(testFile, testErrSymlinkNotAllowed, testErrHardlinkNotAllowed)
	if err != nil {
		t.Fatalf("OpenFile() existing file error = %v", err)
	}
	defer file2.Close()

	if size2 != int64(n) {
		t.Errorf("Existing file size = %d, want %d", size2, n)
	}
}

func TestOpenFileMissingDirectory(t *testing.T) {
	// Opening a path in a non-existent directory must fail cleanly rather
	// than creating the directory.
	path := filepath.Join(t.TempDir(), "missing", "test.log")
	file, _, err := OpenFile(path, testErrSymlinkNotAllowed, testErrHardlinkNotAllowed)
	if file != nil {
		file.Close()
	}
	if err == nil {
		t.Error("OpenFile() should fail for a path in a missing directory")
	}
}

// TestSymlinkDetection pins symlink rejection of both secure open paths with
// REAL symlinks. The historical ModeSymlink-on-file.Stat() check could never
// fire (fstat on a descriptor that followed a symlink reports the target's
// regular-file mode), so both scenarios below previously opened — and for the
// dangling case, created — the attacker's target. Creating symlinks requires
// OS support and privilege (Windows needs developer mode or admin), so each
// subtest skips when os.Symlink fails.
func TestSymlinkDetection(t *testing.T) {
	tmpDir := t.TempDir()
	targetFile := filepath.Join(tmpDir, "target.log")
	if err := os.WriteFile(targetFile, []byte("target data"), 0o644); err != nil {
		t.Fatalf("create target file: %v", err)
	}
	missingTarget := filepath.Join(tmpDir, "does-not-exist.log")

	openers := []struct {
		name string
		open func(path string) (*os.File, int64, error)
	}{
		{"OpenFile", func(p string) (*os.File, int64, error) {
			return OpenFile(p, testErrSymlinkNotAllowed, testErrHardlinkNotAllowed)
		}},
		{"OpenFileExclusive", func(p string) (*os.File, int64, error) {
			return OpenFileExclusive(p, testErrSymlinkNotAllowed, testErrHardlinkNotAllowed)
		}},
	}

	scenarios := []struct {
		name   string
		target string
	}{
		{"existing target", targetFile},
		{"dangling target", missingTarget},
	}

	for _, opener := range openers {
		for _, scenario := range scenarios {
			t.Run(opener.name+" "+scenario.name, func(t *testing.T) {
				linkPath := filepath.Join(tmpDir,
					"link_"+opener.name+"_"+scenario.name+".log")
				if err := os.Symlink(scenario.target, linkPath); err != nil {
					t.Skipf("cannot create symlink (OS support/privilege required): %v", err)
				}

				file, _, err := opener.open(linkPath)
				if file != nil {
					file.Close()
				}
				if !errors.Is(err, testErrSymlinkNotAllowed) {
					t.Fatalf("%s(symlink) error = %v, want testErrSymlinkNotAllowed", opener.name, err)
				}

				// The dangling case must not create the symlink's target as a
				// side effect of the (rejected) open attempt.
				if _, serr := os.Lstat(missingTarget); scenario.target == missingTarget && serr == nil {
					t.Error("open through dangling symlink created the target file")
				}
			})
		}
	}

	// The existing-target scenario must also leave the target's content
	// untouched (rejection happens before any write).
	data, err := os.ReadFile(targetFile)
	if err != nil {
		t.Fatalf("read target file: %v", err)
	}
	if string(data) != "target data" {
		t.Errorf("target file content = %q, want %q", data, "target data")
	}
}

// TestHardlinkDetection consolidates the hardlink scenarios: a normal file
// opens cleanly through both secure open paths, a hardlinked file is
// rejected by both, and the underlying isHardlink check agrees. Creating
// hard links requires OS support (not always available on Windows), so the
// hardlink half is skipped when os.Link fails.
func TestHardlinkDetection(t *testing.T) {
	tmpDir := t.TempDir()
	originalFile := filepath.Join(tmpDir, "original.log")
	if err := os.WriteFile(originalFile, []byte("test data"), 0o644); err != nil {
		t.Fatalf("create original file: %v", err)
	}

	openers := []struct {
		name string
		open func(path string) (*os.File, int64, error)
	}{
		{"OpenFile", func(p string) (*os.File, int64, error) {
			return OpenFile(p, testErrSymlinkNotAllowed, testErrHardlinkNotAllowed)
		}},
		{"OpenFileExclusive", func(p string) (*os.File, int64, error) {
			return OpenFileExclusive(p, testErrSymlinkNotAllowed, testErrHardlinkNotAllowed)
		}},
	}

	for _, opener := range openers {
		t.Run(opener.name+" normal file", func(t *testing.T) {
			normalFile := filepath.Join(tmpDir, "normal_"+opener.name+".log")
			if err := os.WriteFile(normalFile, []byte("12345"), 0o644); err != nil {
				t.Fatalf("create normal file: %v", err)
			}

			file, size, err := opener.open(normalFile)
			if err != nil {
				t.Fatalf("open normal file: %v", err)
			}
			file.Close()
			if size != 5 {
				t.Errorf("normal file size = %d, want 5", size)
			}
		})
	}

	// Unit level: a normal file must NOT be flagged.
	normalHandle, err := os.Open(originalFile)
	if err != nil {
		t.Fatalf("open original file: %v", err)
	}
	isHardlinked, err := isHardlink(normalHandle)
	normalHandle.Close()
	if err != nil {
		t.Fatalf("isHardlink() error = %v", err)
	}
	if isHardlinked {
		t.Error("Normal file should not be detected as hardlinked")
	}

	hardlinkFile := filepath.Join(tmpDir, "hardlink.log")
	if err := os.Link(originalFile, hardlinkFile); err != nil {
		t.Skipf("cannot create hard link (not supported on this system): %v", err)
	}

	// Unit level: Nlink > 1 is detected.
	linkHandle, err := os.Open(hardlinkFile)
	if err != nil {
		t.Fatalf("open hardlinked file: %v", err)
	}
	defer linkHandle.Close()
	isHardlinked, err = isHardlink(linkHandle)
	if err != nil {
		t.Fatalf("isHardlink() error = %v", err)
	}
	if !isHardlinked {
		t.Error("Hardlinked file should be detected as having multiple links")
	}

	for _, opener := range openers {
		t.Run(opener.name+" rejects hardlink", func(t *testing.T) {
			file, _, err := opener.open(hardlinkFile)
			if file != nil {
				file.Close()
			}
			if !errors.Is(err, testErrHardlinkNotAllowed) {
				t.Errorf("open hardlinked file error = %v, want %v", err, testErrHardlinkNotAllowed)
			}
		})
	}
}

func TestNeedsRotation(t *testing.T) {
	tests := []struct {
		name        string
		currentSize int64
		writeSize   int64
		maxSize     int64
		want        bool
	}{
		{
			name:        "no rotation needed",
			currentSize: 100,
			writeSize:   50,
			maxSize:     200,
			want:        false,
		},
		{
			name:        "rotation needed",
			currentSize: 100,
			writeSize:   150,
			maxSize:     200,
			want:        true,
		},
		{
			name:        "max size zero",
			currentSize: 100,
			writeSize:   150,
			maxSize:     0,
			want:        false,
		},
		{
			name:        "exact limit",
			currentSize: 100,
			writeSize:   100,
			maxSize:     200,
			want:        false,
		},
		{
			name:        "exceed by one",
			currentSize: 100,
			writeSize:   101,
			maxSize:     200,
			want:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NeedsRotation(tt.currentSize, tt.writeSize, tt.maxSize)
			if got != tt.want {
				t.Errorf("NeedsRotation() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetBackupPath(t *testing.T) {
	tests := []struct {
		name     string
		basePath string
		index    int
		compress bool
		want     string
	}{
		{
			name:     "simple file",
			basePath: "test.log",
			index:    1,
			compress: false,
			want:     "test_log_1.log",
		},
		{
			name:     "compressed file",
			basePath: "test.log",
			index:    2,
			compress: true,
			want:     "test_log_2.log.gz",
		},
		{
			name:     "no extension",
			basePath: "test",
			index:    3,
			compress: false,
			want:     "test__3",
		},
		{
			name:     "path with directory",
			basePath: filepath.Join("var", "log", "app.log"),
			index:    1,
			compress: false,
			want:     filepath.Join("var", "log", "app_log_1.log"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetBackupPath(tt.basePath, tt.index, tt.compress)
			if got != tt.want {
				t.Errorf("GetBackupPath() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestOpenFileExclusive covers the rotation-reopen path: a fresh path is
// created exclusively (O_EXCL, size 0), while a pre-existing path falls back
// to OpenFile and reports the existing size.
func TestOpenFileExclusive(t *testing.T) {
	tmpDir := t.TempDir()
	fresh := filepath.Join(tmpDir, "fresh.log")

	// Fresh path: exclusive create succeeds, reports a new (size-0) file.
	file, size, err := OpenFileExclusive(fresh, testErrSymlinkNotAllowed, testErrHardlinkNotAllowed)
	if err != nil {
		t.Fatalf("OpenFileExclusive(fresh) error = %v", err)
	}
	if size != 0 {
		t.Errorf("fresh file size = %d, want 0", size)
	}
	if _, err := file.Write([]byte("payload")); err != nil {
		t.Fatalf("write error = %v", err)
	}
	file.Close()

	// Pre-existing file: O_EXCL fails, falls back to OpenFile, which reports
	// the existing size.
	file2, size2, err := OpenFileExclusive(fresh, testErrSymlinkNotAllowed, testErrHardlinkNotAllowed)
	if err != nil {
		t.Fatalf("OpenFileExclusive(existing) error = %v", err)
	}
	file2.Close()
	if size2 != 7 {
		t.Errorf("existing file size = %d, want 7", size2)
	}
}

// TestRotateBackupsCleanup consolidates the excess-backup scenarios: when
// backups exceed maxBackups the oldest are removed and the newest are kept;
// at or under the limit nothing is removed; maxBackups=0 disables cleanup
// entirely.
func TestRotateBackupsCleanup(t *testing.T) {
	tests := []struct {
		name        string
		createCount int // backups to create before rotating
		maxBackups  int // limit passed to RotateBackups (0 = unlimited)
		compress    bool
		wantDeleted int // number of oldest backups that must be gone; the rest must remain
	}{
		{"under limit keeps all", 2, 5, false, 0},
		{"over limit removes oldest", 5, 3, false, 2},
		{"compressed over limit removes oldest", 7, 3, true, 4},
		{"shrunk limit prunes backlog", 10, 5, true, 5},
		{"zero means unlimited", 3, 0, false, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			basePath := filepath.Join(tmpDir, "test.log")

			for i := 1; i <= tt.createCount; i++ {
				backupPath := GetBackupPath(basePath, i, tt.compress)
				if err := os.WriteFile(backupPath, []byte("backup"), 0o644); err != nil {
					t.Fatalf("create backup %d: %v", i, err)
				}
			}

			RotateBackups(basePath, tt.maxBackups, tt.compress)

			for i := 1; i <= tt.wantDeleted; i++ {
				backupPath := GetBackupPath(basePath, i, tt.compress)
				if _, err := os.Stat(backupPath); !os.IsNotExist(err) {
					t.Errorf("backup %d should be deleted (oldest, beyond maxBackups)", i)
				}
			}
			for i := tt.wantDeleted + 1; i <= tt.createCount; i++ {
				backupPath := GetBackupPath(basePath, i, tt.compress)
				if _, err := os.Stat(backupPath); err != nil {
					t.Errorf("backup %d should exist after rotation", i)
				}
			}
		})
	}
}

func TestCompressFile(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.log")
	testData := []byte("test data for compression")

	// Create test file
	err := os.WriteFile(testFile, testData, 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Compress it
	err = CompressFile(testFile)
	if err != nil {
		t.Fatalf("CompressFile() error = %v", err)
	}

	// Check that compressed file exists
	compressedFile := testFile + ".gz"
	if _, err := os.Stat(compressedFile); err != nil {
		t.Errorf("Compressed file should exist: %v", err)
	}

	// Check that original file is removed
	if _, err := os.Stat(testFile); !os.IsNotExist(err) {
		t.Errorf("Original file should be removed after compression")
	}

	// Verify compressed file is valid by trying to read it
	err = verifyGzipFile(compressedFile)
	if err != nil {
		t.Errorf("Compressed file verification failed: %v", err)
	}
}

func TestCompressFileErrors(t *testing.T) {
	tmpDir := t.TempDir()

	// Test with non-existent file
	err := CompressFile(filepath.Join(tmpDir, "nonexistent.log"))
	if err == nil {
		t.Error("CompressFile() should fail with non-existent file")
	}

	// Test with directory instead of file
	err = CompressFile(tmpDir)
	if err == nil {
		t.Error("CompressFile() should fail with directory")
	}
}

func TestCleanupOldFiles(t *testing.T) {
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test.log")

	// Create some old backup files using proper naming pattern
	oldFile1 := GetBackupPath(basePath, 1, false)
	oldFile2 := GetBackupPath(basePath, 2, false)
	newFile := GetBackupPath(basePath, 3, false)

	// Create files with different ages
	err := os.WriteFile(oldFile1, []byte("old1"), 0644)
	if err != nil {
		t.Fatalf("Failed to create old file: %v", err)
	}

	err = os.WriteFile(oldFile2, []byte("old2"), 0644)
	if err != nil {
		t.Fatalf("Failed to create old file: %v", err)
	}

	err = os.WriteFile(newFile, []byte("new"), 0644)
	if err != nil {
		t.Fatalf("Failed to create new file: %v", err)
	}

	// Make old files actually old
	oldTime := time.Now().Add(-2 * time.Hour)
	err = os.Chtimes(oldFile1, oldTime, oldTime)
	if err != nil {
		t.Fatalf("Failed to change file time: %v", err)
	}

	err = os.Chtimes(oldFile2, oldTime, oldTime)
	if err != nil {
		t.Fatalf("Failed to change file time: %v", err)
	}

	// Cleanup files older than 1 hour
	if err := CleanupOldFiles(basePath, time.Hour); err != nil {
		t.Fatalf("CleanupOldFiles failed: %v", err)
	}

	// Check that old files are removed
	if _, err := os.Stat(oldFile1); !os.IsNotExist(err) {
		t.Error("Old file should be removed")
	}

	if _, err := os.Stat(oldFile2); !os.IsNotExist(err) {
		t.Error("Old file should be removed")
	}

	// Check that new file still exists
	if _, err := os.Stat(newFile); err != nil {
		t.Error("New file should still exist")
	}
}

func TestCleanupOldFilesZeroAge(t *testing.T) {
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test.log")

	// A genuinely old backup that WOULD match the cleanup glob: maxAge <= 0
	// disables age-based cleanup entirely, so even this file must survive.
	// (The previous version created "test.log.1", which never matches the
	// backup naming scheme and could not fail under any maxAge behavior.)
	backup := GetBackupPath(basePath, 1, false)
	if err := os.WriteFile(backup, []byte("test"), 0o644); err != nil {
		t.Fatalf("Failed to create backup file: %v", err)
	}
	oldTime := time.Now().Add(-24 * time.Hour)
	if err := os.Chtimes(backup, oldTime, oldTime); err != nil {
		t.Fatalf("Failed to age backup file: %v", err)
	}

	if err := CleanupOldFiles(basePath, 0); err != nil {
		t.Fatalf("CleanupOldFiles failed: %v", err)
	}

	if _, err := os.Stat(backup); err != nil {
		t.Error("Old backup should survive when maxAge=0 (cleanup disabled)")
	}
}

func TestVerifyGzipFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Test with non-existent file
	err := verifyGzipFile(filepath.Join(tmpDir, "nonexistent.gz"))
	if err == nil {
		t.Error("verifyGzipFile() should fail with non-existent file")
	}

	// Test with invalid gzip file
	invalidFile := filepath.Join(tmpDir, "invalid.gz")
	err = os.WriteFile(invalidFile, []byte("not gzip"), 0644)
	if err != nil {
		t.Fatalf("Failed to create invalid file: %v", err)
	}

	err = verifyGzipFile(invalidFile)
	if err == nil {
		t.Error("verifyGzipFile() should fail with invalid gzip file")
	}
}

func TestFindNextBackupIndex(t *testing.T) {
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test.log")

	// Test with no existing backups
	index := FindNextBackupIndex(basePath)
	if index != 1 {
		t.Errorf("FindNextBackupIndex() with no backups = %d, want 1", index)
	}

	// Create some backup files
	backup1 := GetBackupPath(basePath, 1, false)
	backup2 := GetBackupPath(basePath, 2, false)
	backup5 := GetBackupPath(basePath, 5, false)

	err := os.WriteFile(backup1, []byte("backup1"), 0644)
	if err != nil {
		t.Fatalf("Failed to create backup1: %v", err)
	}

	err = os.WriteFile(backup2, []byte("backup2"), 0644)
	if err != nil {
		t.Fatalf("Failed to create backup2: %v", err)
	}

	err = os.WriteFile(backup5, []byte("backup5"), 0644)
	if err != nil {
		t.Fatalf("Failed to create backup5: %v", err)
	}

	// Test with existing backups (should return 6, next after highest)
	index = FindNextBackupIndex(basePath)
	if index != 6 {
		t.Errorf("FindNextBackupIndex() with backups 1,2,5 = %d, want 6", index)
	}

	// A settled .gz backup alongside the uncompressed ones must not lower the
	// answer: the uncompressed pattern also matches ".gz" names, so index 5
	// stays the highest live index (never reuse a live index).
	backup1gz := GetBackupPath(basePath, 1, true)
	err = os.WriteFile(backup1gz, []byte("backup1gz"), 0644)
	if err != nil {
		t.Fatalf("Failed to create backup1gz: %v", err)
	}

	index = FindNextBackupIndex(basePath)
	if index != 6 {
		t.Errorf("FindNextBackupIndex() with mixed .log and .gz backups = %d, want 6", index)
	}

	// Compressed-only directory: .gz backups alone are also counted.
	tmpDir2 := t.TempDir()
	basePath2 := filepath.Join(tmpDir2, "test.log")
	onlyGz := GetBackupPath(basePath2, 1, true)
	if err := os.WriteFile(onlyGz, []byte("backup1gz"), 0644); err != nil {
		t.Fatalf("Failed to create onlyGz: %v", err)
	}

	if index = FindNextBackupIndex(basePath2); index != 2 {
		t.Errorf("FindNextBackupIndex() with compressed backup 1 = %d, want 2", index)
	}

	// Regression for the compress-mode index-reuse race: settled .gz backups
	// (1, 2) plus an IN-FLIGHT uncompressed backup (3, still being compressed).
	// Scanning with the .gz pattern alone returned 3, so the next rotation
	// renamed onto test_log_3.log — clobbering the file the compression
	// goroutine was about to read. The highest live index must win.
	tmpDir3 := t.TempDir()
	basePath3 := filepath.Join(tmpDir3, "test.log")
	for _, i := range []int{1, 2} {
		gz := GetBackupPath(basePath3, i, true)
		if err := os.WriteFile(gz, []byte("gz"), 0644); err != nil {
			t.Fatalf("Failed to create gz backup %d: %v", i, err)
		}
	}
	inFlight := GetBackupPath(basePath3, 3, false)
	if err := os.WriteFile(inFlight, []byte("in-flight"), 0644); err != nil {
		t.Fatalf("Failed to create in-flight backup: %v", err)
	}

	if index = FindNextBackupIndex(basePath3); index != 4 {
		t.Errorf("FindNextBackupIndex() with in-flight uncompressed backup = %d, want 4", index)
	}
}
