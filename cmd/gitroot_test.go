package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func Test_gitRoot(t *testing.T) {
	// Create temporary directory structure
	tempDir, err := os.MkdirTemp("", "gitroot-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	{
		abs := filepath.Clean(tempDir)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("dir=%q abs=%q", tempDir, abs)
	}

	// Create .git directory in root
	if err := os.Mkdir(filepath.Join(tempDir, ".git"), 0755); err != nil {
		t.Fatal(err)
	}

	// Create test subdirectories
	dirs := []string{
		filepath.Join(tempDir, "subdir"),
		filepath.Join(tempDir, "subdir", "nested"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}

	tests := []struct {
		name        string
		startIn     string // Directory to run test from
		wantRoot    string // Expected git root
		wantCurrent string // Expected current directory
		wantErr     bool
	}{
		{
			name:        "in git root",
			startIn:     tempDir,
			wantRoot:    tempDir,
			wantCurrent: tempDir,
		},
		{
			name:        "in subdirectory",
			startIn:     filepath.Join(tempDir, "subdir"),
			wantRoot:    tempDir,
			wantCurrent: filepath.Join(tempDir, "subdir"),
		},
		{
			name:        "in nested subdirectory",
			startIn:     filepath.Join(tempDir, "subdir", "nested"),
			wantRoot:    tempDir,
			wantCurrent: filepath.Join(tempDir, "subdir", "nested"),
		},
	}

	// Save original working directory
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(origWd); err != nil {
			panic(err)
		}
	}()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := os.Chdir(tt.startIn); err != nil {
				t.Fatal(err)
			}

			gotRoot, gotCurrent, err := gitRoot()
			if (err != nil) != tt.wantErr {
				t.Errorf("gitRoot() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotRoot != tt.wantRoot {
				t.Errorf("gitRoot() got [root] = %v, want %v", gotRoot, tt.wantRoot)
			}
			if gotCurrent != tt.wantCurrent {
				t.Errorf("gitRoot() got [current] = %v, want %v", gotCurrent, tt.wantCurrent)
			}
		})
	}
}
