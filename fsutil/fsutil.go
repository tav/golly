// Public Domain (-) 2012 The Golly Authors.
// See the Golly UNLICENSE file for details.

// Package fsutil implements utility functions for querying the filesystem.
package fsutil

import (
	"fmt"
	"os"
)

type NotFound struct {
	path string
}

func (err *NotFound) Error() string {
	return fmt.Sprintf("not found: %s", err.path)
}

type NotFile struct {
	path string
}

func (err *NotFile) Error() string {
	return fmt.Sprintf("directory found instead of file at: %s", err.path)
}

// Exists returns whether a link exists at a given filesystem path.
func Exists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, &NotFound{path}
	}
	return false, err
}

// FileExists returns whether a file exists at a given filesystem path.
func FileExists(path string) (bool, error) {
	info, err := os.Stat(path)
	if err == nil {
		if !info.IsDir() {
			return true, nil
		}
		return false, &NotFile{path}
	}
	if os.IsNotExist(err) {
		return false, &NotFound{path}
	}
	return false, err
}
