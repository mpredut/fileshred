package main

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"syscall"
)

// Metadata to track progress
type ShredMetadata struct {
	Pass         int64
	TempPath     string
	OriginalPath string
}

// Save metadata to a file
func saveMetadata(metadata ShredMetadata) error {
	file, err := os.Create(metadata.OriginalPath + ".shredmeta")
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	return encoder.Encode(metadata)
}

// Load metadata from a file
func loadMetadata(path string) (ShredMetadata, error) {
	file, err := os.Open(path + ".shredmeta")
	if err != nil {
		return ShredMetadata{}, err
	}
	defer file.Close()

	var metadata ShredMetadata
	decoder := json.NewDecoder(file)
	err = decoder.Decode(&metadata)
	return metadata, err
}

// Check if another process is trying to access the file
func isFileLocked(path string) bool {
	file, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return false
	}
	defer file.Close()

	err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err != nil {
		return true
	}
	defer syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
	return false
}

// Generate a random string of a given length
func randomString(length int) (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}

	for i := range b {
		b[i] = charset[int(b[i])%len(charset)]
	}

	return string(b), nil
}

// Overwrite the entire device for SSDs
func overwriteDevice(devicePath string) error {
	device, err := os.OpenFile(devicePath, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer device.Close()

	info, err := device.Stat()
	if err != nil {
		return err
	}

	size := info.Size()
	randomData := make([]byte, 4096) // Use 4KB blocks for writing

	for written := int64(0); written < size; written += 4096 {
		_, err = rand.Read(randomData)
		if err != nil {
			return err
		}

		_, err = device.WriteAt(randomData, written)
		if err != nil {
			return err
		}
	}

	return nil
}

func Shred(path string, passes int64) error {
	// File size verification
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.Size() > 1024*1024*1024 { // 1GB limit
		return fmt.Errorf("file size exceeds the allowed limit")
	}

	// Load metadata if it exists
	metadata, err := loadMetadata(path)
	if err != nil {
		metadata = ShredMetadata{Pass: 0, TempPath: "", OriginalPath: path}
	}

	// Rename the file to a temporary name if not already done
	if metadata.TempPath == "" {
		tempPath := path + ".tmp"
		err = os.Rename(path, tempPath)
		if err != nil {
			return err
		}
		metadata.TempPath = tempPath
		err = saveMetadata(metadata)
		if err != nil {
			return err
		}
	}

	// Check if another process is locking the temporary file
	if isFileLocked(metadata.TempPath) {
		fmt.Println("Temporary file is locked by another process: ", metadata.TempPath)
		return fmt.Errorf("temporary file is locked by another process")
	}

	// Open the temporary file for writing
	tempFile, err := os.OpenFile(metadata.TempPath, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer tempFile.Close()

	// Acquire the lock on the temporary file
	err = syscall.Flock(int(tempFile.Fd()), syscall.LOCK_EX)
	if err != nil {
		return err
	}
	defer syscall.Flock(int(tempFile.Fd()), syscall.LOCK_UN)

	// Overwrite the file contents multiple times
	randomData := make([]byte, info.Size())
	for i := metadata.Pass; i < passes; i++ {
		_, err = rand.Read(randomData)
		if err != nil {
			return err
		}

		_, err = tempFile.WriteAt(randomData, 0)
		if err != nil {
			return err
		}

		// Save progress to metadata file
		metadata.Pass = i + 1
		err = saveMetadata(metadata)
		if err != nil {
			return err
		}
	}

	// Rename the file to random names multiple times
	for i := 0; i < 10; i++ { // Adjust the number of renames as needed
		newName, err := randomString(12)
		if err != nil {
			return err
		}

		newPath := metadata.TempPath + "." + newName
		err = os.Rename(metadata.TempPath, newPath)
		if err != nil {
			return err
		}

		metadata.TempPath = newPath
		err = saveMetadata(metadata)
		if err != nil {
			return err
		}
	}

	// Truncate the temporary file to 0 bytes
	err = tempFile.Truncate(0)
	if err != nil {
		return err
	}

	// Remove the metadata file
	err = os.Remove(metadata.OriginalPath + ".shredmeta")
	if err != nil {
		return err
	}

	// Remove the original file
	err = os.Remove(metadata.TempPath)
	if err != nil {
		return err
	}

	// Overwrite the entire device for SSDs
	// Note: Identify the device path where the file resides
	//devicePath := "/dev/sdX" // Placeholder, should be identified dynamically
	//err = overwriteDevice(devicePath)
	//if err != nil {
		//return err
	//}

	return nil
}
