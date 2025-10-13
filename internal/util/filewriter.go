// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"os"
)

type FileIO interface {
	MkdirAll(path string, perm os.FileMode) error
	OpenFile(name string, flag int, perm os.FileMode) (*os.File, error)
	Open(filename string) (*os.File, error)
	Create(filename string) (*os.File, error)
	IsDirectory(filename string) (bool, error)
	Exists(filename string) bool
	ReadDir(dirname string) ([]os.DirEntry, error)
}

type FilesystemWriter struct{}

func NewFilesystemWriter() *FilesystemWriter {
	return &FilesystemWriter{}
}

func (fs *FilesystemWriter) Create(filename string) (*os.File, error) {
	return os.Create(filename)
}

func (fs *FilesystemWriter) Open(filename string) (*os.File, error) {
	return os.Open(filename)
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

func (fs *FilesystemWriter) ReadDir(dirname string) ([]os.DirEntry, error) {
	return os.ReadDir(dirname)
}

type ClosableFile interface {
	Close() error
}

// Close file and ignore error. This function is to be used with defer only.
func CloseFileIgnoreError(f ClosableFile) {
	_ = f.Close()
}
