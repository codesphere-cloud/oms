package util

import "os"

type FileWriter interface {
	Create(filename string) (*os.File, error)
}

type FilesystemWriter struct{}

func NewFilesystemWriter() *FilesystemWriter {
	return &FilesystemWriter{}
}

func (fs *FilesystemWriter) Create(filename string) (*os.File, error) {
	return os.Create(filename)
}
