// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"fmt"
	"os"
)

type FileIO interface {
	Create(filename string) (*os.File, error)
	Open(filename string) (*os.File, error)
	OpenAppend(filename string) (*os.File, error)
	Exists(filename string) bool
	IsDirectory(filename string) (bool, error)
	MkdirAll(path string, perm os.FileMode) error
	OpenFile(name string, flag int, perm os.FileMode) (*os.File, error)
	WriteFile(filename string, data []byte, perm os.FileMode) error
	ReadFile(filename string) ([]byte, error)
	ReadDir(dirname string) ([]os.DirEntry, error)
	ReadFile(filename string) ([]byte, error)
	CreateAndWrite(filePath string, data []byte, fileType string) error
}

type FilesystemWriter struct{}

func NewFilesystemWriter() *FilesystemWriter {
	return &FilesystemWriter{}
}

func (fs *FilesystemWriter) Create(filename string) (*os.File, error) {
	return os.Create(filename)
}

func (fs *FilesystemWriter) CreateAndWrite(filePath string, data []byte, fileType string) error {
	file, err := fs.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create %s file: %w", fileType, err)
	}
	defer CloseFileIgnoreError(file)

	if _, err = file.Write(data); err != nil {
		return fmt.Errorf("failed to write %s file: %w", fileType, err)
	}

	fmt.Printf("\n%s file created: %s\n", fileType, filePath)
	return nil
}

func (fs *FilesystemWriter) Open(filename string) (*os.File, error) {
	return os.Open(filename)
}

func (fs *FilesystemWriter) OpenAppend(filename string) (*os.File, error) {
	return os.OpenFile(filename, os.O_APPEND|os.O_WRONLY, 0644)
}

func (fs *FilesystemWriter) Exists(path string) bool {
	_, err := os.Stat(path)
	if err != nil {
		// stat failed, assume file doesn't exist
		return false
	}
	return true
}

func (fs *FilesystemWriter) IsDirectory(path string) (bool, error) {
	fileInfo, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	return fileInfo.IsDir(), err
}

func (fs *FilesystemWriter) MkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}

func (fs *FilesystemWriter) OpenFile(name string, flag int, perm os.FileMode) (*os.File, error) {
	return os.OpenFile(name, flag, perm)
}

func (fs *FilesystemWriter) WriteFile(filename string, data []byte, perm os.FileMode) error {
	return os.WriteFile(filename, data, perm)
}

func (fs *FilesystemWriter) ReadFile(filename string) ([]byte, error) {
	return os.ReadFile(filename)
}

func (fs *FilesystemWriter) ReadDir(dirname string) ([]os.DirEntry, error) {
	return os.ReadDir(dirname)
}

func (fs *FilesystemWriter) ReadFile(filename string) ([]byte, error) {
	return os.ReadFile(filename)
}

type ClosableFile interface {
	Close() error
}

// Close file and ignore error. This function is to be used with defer only.
func CloseFileIgnoreError(f ClosableFile) {
	_ = f.Close()
}
