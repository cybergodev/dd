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

func TestOpenFileRejectsHardlink(t *testing.T) {
	tmpDir := t.TempDir()
	originalFile := filepath.Join(tmpDir, "original.log")
	hardlinkFile := filepath.Join(tmpDir, "hardlink.log")

	// Create the original file
	err := os.WriteFile(originalFile, []byte("test data"), 0644)
	if err != nil {
		t.Fatalf("Failed to create original file: %v", err)
	}

	// Create a hard link to the original file
	err = os.Link(originalFile, hardlinkFile)
	if err != nil {
		// On some systems (like Windows without admin rights), hard links may not be supported
		// Skip the test in this case
		t.Skipf("Cannot create hard link (may not be supported on this system): %v", err)
	}

	// Attempt to open the hardlinked file - should be rejected
	file, _, err := OpenFile(hardlinkFile, testErrSymlinkNotAllowed, testErrHardlinkNotAllowed)
	if file != nil {
		file.Close()
	}
	if err == nil {
		t.Error("OpenFile() should reject hardlinked files")
	}
	// The error should mention hardlinks
	if err != nil && !containsString(err.Error(), "hardlink") {
		t.Errorf("OpenFile() error should mention 'hardlink', got: %v", err)
	}
}

func TestIsHardlink(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.log")

	// Create a normal file
	err := os.WriteFile(testFile, []byte("test data"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Open the file
	file, err := os.Open(testFile)
	if err != nil {
		t.Fatalf("Failed to open file: %v", err)
	}
	defer file.Close()

	// A normal file should not be detected as a hardlink
	isHardlinked, err := isHardlink(file)
	if err != nil {
		t.Fatalf("isHardlink() error = %v", err)
	}
	if isHardlinked {
		t.Error("Normal file should not be detected as hardlinked")
	}
}

func TestIsHardlinkWithHardlink(t *testing.T) {
	tmpDir := t.TempDir()
	originalFile := filepath.Join(tmpDir, "original.log")
	hardlinkFile := filepath.Join(tmpDir, "hardlink.log")

	// Create the original file
	err := os.WriteFile(originalFile, []byte("test data"), 0644)
	if err != nil {
		t.Fatalf("Failed to create original file: %v", err)
	}

	// Create a hard link
	err = os.Link(originalFile, hardlinkFile)
	if err != nil {
		t.Skipf("Cannot create hard link: %v", err)
	}

	// Open the hardlinked file
	file, err := os.Open(hardlinkFile)
	if err != nil {
		t.Fatalf("Failed to open hardlinked file: %v", err)
	}
	defer file.Close()

	// A hardlinked file should be detected
	isHardlinked, err := isHardlink(file)
	if err != nil {
		t.Fatalf("isHardlink() error = %v", err)
	}
	if !isHardlinked {
		t.Error("Hardlinked file should be detected as having multiple links")
	}
}

// Helper function to check if a string contains a substring
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
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

func TestRotateBackups(t *testing.T) {
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test.log")

	// Create some backup files with new naming scheme
	backup1 := GetBackupPath(basePath, 1, false)
	backup2 := GetBackupPath(basePath, 2, false)

	// Create files
	err := os.WriteFile(backup1, []byte("backup1"), 0644)
	if err != nil {
		t.Fatalf("Failed to create backup1: %v", err)
	}

	err = os.WriteFile(backup2, []byte("backup2"), 0644)
	if err != nil {
		t.Fatalf("Failed to create backup2: %v", err)
	}

	// Rotate with max 3 backups (should not remove anything yet)
	RotateBackups(basePath, 3, false)

	// Check that existing files still exist
	if _, err := os.Stat(backup1); err != nil {
		t.Errorf("backup1 should still exist")
	}

	if _, err := os.Stat(backup2); err != nil {
		t.Errorf("backup2 should still exist")
	}

	// Create more backups to exceed maxBackups
	backup3 := GetBackupPath(basePath, 3, false)
	backup4 := GetBackupPath(basePath, 4, false)

	err = os.WriteFile(backup3, []byte("backup3"), 0644)
	if err != nil {
		t.Fatalf("Failed to create backup3: %v", err)
	}

	err = os.WriteFile(backup4, []byte("backup4"), 0644)
	if err != nil {
		t.Fatalf("Failed to create backup4: %v", err)
	}

	// Rotate with max 3 backups (should remove oldest)
	RotateBackups(basePath, 3, false)

	// Check that oldest backup was removed
	if _, err := os.Stat(backup1); !os.IsNotExist(err) {
		t.Errorf("backup1 should be removed (oldest)")
	}

	// Check that newer backups still exist
	if _, err := os.Stat(backup2); err != nil {
		t.Errorf("backup2 should still exist")
	}

	if _, err := os.Stat(backup3); err != nil {
		t.Errorf("backup3 should still exist")
	}

	if _, err := os.Stat(backup4); err != nil {
		t.Errorf("backup4 should still exist")
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

	// Create a file
	testFile := basePath + ".1"
	err := os.WriteFile(testFile, []byte("test"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Cleanup with zero age (should not remove anything)
	if err := CleanupOldFiles(basePath, 0); err != nil {
		t.Fatalf("CleanupOldFiles failed: %v", err)
	}

	// File should still exist
	if _, err := os.Stat(testFile); err != nil {
		t.Error("File should not be removed with zero maxAge")
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
	index := FindNextBackupIndex(basePath, false)
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
	index = FindNextBackupIndex(basePath, false)
	if index != 6 {
		t.Errorf("FindNextBackupIndex() with backups 1,2,5 = %d, want 6", index)
	}

	// Test with compressed files
	backup1gz := GetBackupPath(basePath, 1, true)
	err = os.WriteFile(backup1gz, []byte("backup1gz"), 0644)
	if err != nil {
		t.Fatalf("Failed to create backup1gz: %v", err)
	}

	index = FindNextBackupIndex(basePath, true)
	if index != 2 {
		t.Errorf("FindNextBackupIndex() with compressed backup 1 = %d, want 2", index)
	}
}

// ============================================================================
// ROTATION CLEANUP TESTS (Consolidated from rotation_cleanup_test.go)
// ============================================================================

func TestRotateBackupsCleanupExcess(t *testing.T) {
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test.log")

	// Create 5 backup files
	for i := 1; i <= 5; i++ {
		backupPath := GetBackupPath(basePath, i, false)
		err := os.WriteFile(backupPath, []byte("backup"), 0644)
		if err != nil {
			t.Fatalf("Failed to create backup file %d: %v", i, err)
		}
	}

	// Rotate with maxBackups=3 (should remove oldest 2 files)
	RotateBackups(basePath, 3, false)

	// Files 1 and 2 should be deleted (oldest)
	for i := 1; i <= 2; i++ {
		backupPath := GetBackupPath(basePath, i, false)
		if _, err := os.Stat(backupPath); !os.IsNotExist(err) {
			t.Errorf("Backup file %d should be deleted (oldest, beyond maxBackups)", i)
		}
	}

	// Files 3, 4, and 5 should still exist (newest 3)
	for i := 3; i <= 5; i++ {
		backupPath := GetBackupPath(basePath, i, false)
		if _, err := os.Stat(backupPath); err != nil {
			t.Errorf("Backup file %d should exist after rotation", i)
		}
	}
}

func TestRotateBackupsCleanupExcessCompressed(t *testing.T) {
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test.log")

	// Create 7 compressed backup files
	for i := 1; i <= 7; i++ {
		backupPath := GetBackupPath(basePath, i, true)
		err := os.WriteFile(backupPath, []byte("backup"), 0644)
		if err != nil {
			t.Fatalf("Failed to create backup file %d: %v", i, err)
		}
	}

	// Rotate with maxBackups=3 (should keep newest 3)
	RotateBackups(basePath, 3, true)

	// Files 1-4 should be deleted (oldest)
	for i := 1; i <= 4; i++ {
		backupPath := GetBackupPath(basePath, i, true)
		if _, err := os.Stat(backupPath); !os.IsNotExist(err) {
			t.Errorf("Compressed backup file %d should be deleted (oldest, beyond maxBackups)", i)
		}
	}

	// Files 5, 6, and 7 should exist (newest 3)
	for i := 5; i <= 7; i++ {
		backupPath := GetBackupPath(basePath, i, true)
		if _, err := os.Stat(backupPath); err != nil {
			t.Errorf("Compressed backup file %d should exist after rotation", i)
		}
	}
}

func TestRotateBackupsReducedMaxBackups(t *testing.T) {
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test.log")

	// Simulate previous run with maxBackups=10
	for i := 1; i <= 10; i++ {
		backupPath := GetBackupPath(basePath, i, true)
		err := os.WriteFile(backupPath, []byte("backup"), 0644)
		if err != nil {
			t.Fatalf("Failed to create backup file %d: %v", i, err)
		}
	}

	// Now rotate with reduced maxBackups=5 (should keep newest 5)
	RotateBackups(basePath, 5, true)

	// Files 1-5 should be deleted (oldest)
	for i := 1; i <= 5; i++ {
		backupPath := GetBackupPath(basePath, i, true)
		if _, err := os.Stat(backupPath); !os.IsNotExist(err) {
			t.Errorf("Backup file %d should be deleted after reducing maxBackups (oldest)", i)
		}
	}

	// Files 6-10 should exist (newest 5)
	for i := 6; i <= 10; i++ {
		backupPath := GetBackupPath(basePath, i, true)
		if _, err := os.Stat(backupPath); err != nil {
			t.Errorf("Backup file %d should exist after rotation", i)
		}
	}
}

func TestRotateBackupsNoExcessFiles(t *testing.T) {
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test.log")

	// Create only 2 backup files (less than maxBackups)
	for i := 1; i <= 2; i++ {
		backupPath := GetBackupPath(basePath, i, false)
		err := os.WriteFile(backupPath, []byte("backup"), 0644)
		if err != nil {
			t.Fatalf("Failed to create backup file %d: %v", i, err)
		}
	}

	// Rotate with maxBackups=5 (no cleanup needed)
	RotateBackups(basePath, 5, false)

	// Both files should still exist (no cleanup needed)
	for i := 1; i <= 2; i++ {
		backupPath := GetBackupPath(basePath, i, false)
		if _, err := os.Stat(backupPath); err != nil {
			t.Errorf("Backup file %d should exist after rotation", i)
		}
	}
}

func TestRotateBackupsMaxBackupsZero(t *testing.T) {
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test.log")

	// Create some backup files
	for i := 1; i <= 3; i++ {
		backupPath := GetBackupPath(basePath, i, false)
		err := os.WriteFile(backupPath, []byte("backup"), 0644)
		if err != nil {
			t.Fatalf("Failed to create backup file %d: %v", i, err)
		}
	}

	// Rotate with maxBackups=0 (unlimited)
	RotateBackups(basePath, 0, false)

	// All files should still exist (no cleanup when maxBackups=0)
	for i := 1; i <= 3; i++ {
		backupPath := GetBackupPath(basePath, i, false)
		if _, err := os.Stat(backupPath); err != nil {
			t.Errorf("Backup file %d should still exist with maxBackups=0", i)
		}
	}
}
