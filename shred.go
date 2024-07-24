package main

import (
	"crypto/rand"
	"fmt"
	"io/ioutil"
	"os"
	"sync"
	"syscall"
	"testing"
)

// Shred securely deletes a file by overwriting it multiple times and then removing it.
func Shred(path string, passes int) error {
	file, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer file.Close()

	// Lock the file to prevent concurrent access
	err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX)
	if err != nil {
		return err
	}
	defer syscall.Flock(int(file.Fd()), syscall.LOCK_UN)

	info, err := file.Stat()
	if err != nil {
		return err
	}

	size := info.Size()
	randomData := make([]byte, size)

	// Generate random data once
	_, err = rand.Read(randomData)
	if err != nil {
		return err
	}

	// Use a temporary file for the shredding process
	tempFile, err := ioutil.TempFile("", "shredtemp")
	if err != nil {
		return err
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	// Perform the overwrite passes on the temporary file
	for i := 0; i < passes; i++ {
		_, err = tempFile.WriteAt(randomData, 0)
		if err != nil {
			return err
		}
		err = tempFile.Sync()
		if err != nil {
			return err
		}
	}

	// Ensure all data is written to the disk
	err = tempFile.Sync()
	if err != nil {
		return err
	}

	// Atomically replace the original file with the shredded temporary file
	err = os.Rename(tempFile.Name(), path)
	if err != nil {
		return err
	}

	// Finally, remove the file
	return os.Remove(path)
}

func TestShred(t *testing.T) {
	tests := []struct {
		name       string
		createFile bool
		fileSize   int64
		expectErr  bool
		fileType   string
	}{
		{"Small file", true, 128, false, "regular"},
		{"Large file", true, 1024 * 1024, false, "regular"},
		{"Empty file", true, 0, false, "regular"},
		{"Non-existent file", false, 0, true, "regular"},
		{"Read-only file", true, 128, true, "readonly"},
		{"Symbolic link", true, 128, false, "symlink"},
		{"Hard link", true, 128, false, "hardlink"},
		{"Locked file", true, 128, true, "locked"},
		{"Concurrent access", true, 128, false, "concurrent"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var path string
			var targetPath string

			if tt.createFile {
				file, err := ioutil.TempFile("", "shredtest")
				if err != nil {
					t.Fatalf("Failed to create test file: %v", err)
				}
				path = file.Name()
				file.Truncate(tt.fileSize)
				file.Close()

				if tt.fileType == "readonly" {
					err = os.Chmod(path, 0444)
					if err != nil {
						t.Fatalf("Failed to set read-only permission: %v", err)
					}
				} else if tt.fileType == "symlink" {
					targetPath = path + "_target"
					err = os.Rename(path, targetPath)
					if err != nil {
						t.Fatalf("Failed to rename file: %v", err)
					}
					err = os.Symlink(targetPath, path)
					if err != nil {
						t.Fatalf("Failed to create symlink: %v", err)
					}
				} else if tt.fileType == "hardlink" {
					targetPath = path + "_target"
					err = os.Link(path, targetPath)
					if err != nil {
						t.Fatalf("Failed to create hard link: %v", err)
					}
				} else if tt.fileType == "locked" {
					file, err := os.OpenFile(path, os.O_RDWR, 0)
					if err != nil {
						t.Fatalf("Failed to open file for locking: %v", err)
					}
					defer file.Close()
					err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
					if err != nil {
						t.Fatalf("Failed to lock file: %v", err)
					}
				}
			} else {
				path = "nonexistentfile"
			}

			if tt.fileType == "concurrent" {
				var wg sync.WaitGroup
				wg.Add(2)
				go func() {
					defer wg.Done()
					err := Shred(path, 3)
					if (err != nil) != tt.expectErr {
						t.Errorf("Shred() error = %v, expectErr %v", err, tt.expectErr)
					}
				}()
				go func() {
					defer wg.Done()
					err := Shred(path, 3)
					if (err != nil) != tt.expectErr {
						t.Errorf("Shred() error = %v, expectErr %v", err, tt.expectErr)
					}
				}()
				wg.Wait()
			} else {
				err := Shred(path, 3)
				if (err != nil) != tt.expectErr {
					t.Errorf("Shred() error = %v, expectErr %v", err, tt.expectErr)
				}
			}

			if tt.createFile && !tt.expectErr {
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Errorf("File still exists after shred: %s", path)
				}
				if tt.fileType == "symlink" {
					if _, err := os.Stat(targetPath); !os.IsNotExist(err) {
						t.Errorf("Target file of symlink still exists after shred: %s", targetPath)
					}
				}
			}
		})
	}
}

func runTests() {
	fmt.Println("Running tests...")
	m := testing.MainStart(testing.TestDeps{}, []testing.InternalTest{
		{"TestShred", TestShred},
	}, nil, nil)
	os.Exit(m.Run())
}

func main() {
	runTests()
}
