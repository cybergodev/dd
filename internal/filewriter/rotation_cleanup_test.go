package filewriter

import (
	"os"
	"path/filepath"
	"testing"
)

// TestRotateBackupsCleanupExcess tests that RotateBackups removes files beyond maxBackups
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

	// Verify all 5 files exist
	for i := 1; i <= 5; i++ {
		backupPath := GetBackupPath(basePath, i, false)
		if _, err := os.Stat(backupPath); err != nil {
			t.Fatalf("Backup file %d should exist before rotation", i)
		}
	}

	// Rotate with maxBackups=3 (should remove oldest 2 files)
	err := RotateBackups(basePath, 3, false)
	if err != nil {
		t.Fatalf("RotateBackups() error = %v", err)
	}

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

// TestRotateBackupsCleanupExcessCompressed tests cleanup with compressed files
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
	err := RotateBackups(basePath, 3, true)
	if err != nil {
		t.Fatalf("RotateBackups() error = %v", err)
	}

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

// TestRotateBackupsReducedMaxBackups tests reducing maxBackups from previous runs
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
	err := RotateBackups(basePath, 5, true)
	if err != nil {
		t.Fatalf("RotateBackups() error = %v", err)
	}

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

// TestRotateBackupsNoExcessFiles tests normal rotation without excess files
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
	err := RotateBackups(basePath, 5, false)
	if err != nil {
		t.Fatalf("RotateBackups() error = %v", err)
	}

	// Both files should still exist (no cleanup needed)
	backup1 := GetBackupPath(basePath, 1, false)
	if _, err := os.Stat(backup1); err != nil {
		t.Error("Backup file 1 should exist after rotation")
	}

	backup2 := GetBackupPath(basePath, 2, false)
	if _, err := os.Stat(backup2); err != nil {
		t.Error("Backup file 2 should exist after rotation")
	}
}

// TestRotateBackupsMaxBackupsZero tests behavior with maxBackups=0
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
	err := RotateBackups(basePath, 0, false)
	if err != nil {
		t.Fatalf("RotateBackups() error = %v", err)
	}

	// All files should still exist (no cleanup when maxBackups=0)
	for i := 1; i <= 3; i++ {
		backupPath := GetBackupPath(basePath, i, false)
		if _, err := os.Stat(backupPath); err != nil {
			t.Errorf("Backup file %d should still exist with maxBackups=0", i)
		}
	}
}
