package main

import (
    "fmt"
    "io/ioutil"
    "os"
    "sync"
    "syscall"
)

func main() {
    fmt.Println("Running tests...")
    testCases := []struct {
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

    for _, tt := range testCases {
        fmt.Printf("Running test: %s\n", tt.name)
        var path string
        var targetPath string

        if tt.createFile {
            file, err := ioutil.TempFile("", "shredtest")
            if err != nil {
                fmt.Printf("Failed to create test file: %v\n", err)
                continue
            }
            path = file.Name()
            file.Truncate(tt.fileSize)
            file.Close()

            if tt.fileType == "readonly" {
                err = os.Chmod(path, 0444)
                if err != nil {
                    fmt.Printf("Failed to set read-only permission: %v\n", err)
                    continue
                }
            } else if tt.fileType == "symlink" {
                targetPath = path + "_target"
                err = os.Rename(path, targetPath)
                if err != nil {
                    fmt.Printf("Failed to rename file: %v\n", err)
                    continue
                }
                err = os.Symlink(targetPath, path)
                if err != nil {
                    fmt.Printf("Failed to create symlink: %v\n", err)
                    continue
                }
            } else if tt.fileType == "hardlink" {
                targetPath = path + "_target"
                err = os.Link(path, targetPath)
                if err != nil {
                    fmt.Printf("Failed to create hard link: %v\n", err)
                    continue
                }
            } else if tt.fileType == "locked" {
                file, err := os.OpenFile(path, os.O_RDWR, 0)
                if err != nil {
                    fmt.Printf("Failed to open file for locking: %v\n", err)
                    continue
                }
                defer file.Close()
                err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
                if err != nil {
                    fmt.Printf("Failed to lock file: %v\n", err)
                    continue
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
                    fmt.Printf("Shred() error = %v, expectErr %v\n", err, tt.expectErr)
                }
            }()
            go func() {
                defer wg.Done()
                err := Shred(path, 3)
                if (err != nil) != tt.expectErr {
                    fmt.Printf("Shred() error = %v, expectErr %v\n", err, tt.expectErr)
                }
            }()
            wg.Wait()
        } else {
            err := Shred(path, 3)
            if (err != nil) != tt.expectErr {
                fmt.Printf("Shred() error = %v, expectErr %v\n", err, tt.expectErr)
            }
        }

        if tt.createFile && !tt.expectErr {
            if _, err := os.Stat(path); !os.IsNotExist(err) {
                fmt.Printf("File still exists after shred: %s\n", path)
            }
            if tt.fileType == "symlink" {
                if _, err := os.Stat(targetPath); !os.IsNotExist(err) {
                    fmt.Printf("Target file of symlink still exists after shred: %s\n", targetPath)
                }
            }
        }
    }
}
