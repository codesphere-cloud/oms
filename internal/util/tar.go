// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// ExtractTarGz takes a path to a .tar.gz file and extracts its contents
// to the specified destination directory.
func ExtractTarGz(fileIo FileIO, filename, destDir string) error {
	return ExtractTarGzSingleFile(fileIo, filename, "", destDir)
}

// getCleanTargetPath constructs a clean target path for extraction and ensures
// that it is within the specified destination directory to prevent path traversal attacks.
func getCleanTargetPath(destDir string, header *tar.Header) (string, error) {
	targetPath := filepath.Clean(filepath.Join(destDir, header.Name))
	relPath, err := filepath.Rel(destDir, targetPath)

	// Ensure target dir is inside destDir
	if err != nil || relPath == ".." || strings.HasPrefix(relPath, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("failed to extract %s: target directory outside destination directory %s", header.Name, destDir)
	}
	return targetPath, nil
}

// openTar opens a .tar file and returns a tar.Reader to read its contents.
func openTar(filename string, fileIo FileIO) (*tar.Reader, error) {
	log.Printf("Opening archive: %s", filename)
	file, err := fileIo.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to open archive: %w", err)
	}
	bufferedFile := bufio.NewReader(file)

	log.Println("Reading TAR archive contents...")
	tr := tar.NewReader(bufferedFile)
	return tr, nil
}

// openTarGz opens a .tar.gz file and returns a tar.Reader to read its contents.
func openTarGz(filename string, fileIo FileIO) (*tar.Reader, error) {
	log.Printf("Opening archive: %s", filename)
	file, err := fileIo.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to open archive: %w", err)
	}
	bufferedFile := bufio.NewReader(file)

	gzr, err := gzip.NewReader(bufferedFile)
	if err != nil {
		return nil, fmt.Errorf("failed to create gzip reader: %w", err)
	}

	log.Println("Reading TAR archive contents...")
	tr := tar.NewReader(gzr)
	return tr, nil
}

// extractEntry extracts a single tar.Header entry to the targetPath using the provided tar.Reader.
func extractEntry(header *tar.Header, targetPath string, fileIo FileIO, tr *tar.Reader) error {
	switch header.Typeflag {
	case tar.TypeDir:
		log.Printf("Creating directory: %s", targetPath)
		if err := fileIo.MkdirAll(targetPath, os.FileMode(header.Mode)); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", targetPath, err)
		}

	case tar.TypeReg:
		log.Printf("Extracting file: %s", targetPath)
		if err := fileIo.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", targetPath, err)
		}
		outFile, err := fileIo.OpenFile(targetPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(header.Mode))
		if err != nil {
			return fmt.Errorf("failed to create file %s: %w", targetPath, err)
		}
		defer CloseFileIgnoreError(outFile)

		// Copy the file content from the TAR reader to the disk file
		if _, err := io.Copy(outFile, tr); err != nil {
			return fmt.Errorf("failed to write file content to %s: %w", targetPath, err)
		}
		// Explicitly close the file handle after copying, to ensure permissions are set correctly
		// and data is flushed before the next file potentially opens a new stream.
		if err := outFile.Close(); err != nil {
			return fmt.Errorf("failed to close extracted file %s: %w", targetPath, err)
		}

	case tar.TypeSymlink, tar.TypeLink:
		if err := os.Symlink(header.Linkname, targetPath); err != nil {
			return fmt.Errorf("failed to create symbolic link %s: %w", targetPath, err)
		}

	default:
		log.Printf("Ignoring unsupported header type flag %c for %s", header.Typeflag, header.Name)
	}
	return nil
}

// ExtractTarGzSingleFile extracts a single specified file from a .tar.gz archive to the destination directory.
func ExtractTarGzSingleFile(fileIo FileIO, archiveFile, fileToExtract, destDir string) error {
	destDir = filepath.Clean(destDir)
	tr, err := openTarGz(archiveFile, fileIo)
	if err != nil {
		return err
	}
	return extractTarSingleFile(fileIo, tr, fileToExtract, destDir)
}

// ExtractTarSingleFile extracts a single specified file from a .tar archive to the destination directory.
func ExtractTarSingleFile(fileIo FileIO, archiveFile, fileToExtract, destDir string) error {
	destDir = filepath.Clean(destDir)
	tr, err := openTar(archiveFile, fileIo)
	if err != nil {
		return err
	}
	return extractTarSingleFile(fileIo, tr, fileToExtract, destDir)
}

// extractTarSingleFile extracts a single specified file from a tar.Reader to the destination directory.
func extractTarSingleFile(fileIo FileIO, tr *tar.Reader, fileToExtract, destDir string) error {
	if fileToExtract != "" {
		log.Printf("Extracting %s from archive\n", fileToExtract)
	}

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read next tar entry: %w", err)
		}

		if fileToExtract != "" && filepath.Clean(header.Name) != filepath.Clean(fileToExtract) {
			continue
		}

		// Construct the full path for the destination file/directory
		targetPath, err := getCleanTargetPath(destDir, header)
		if err != nil {
			return err
		}

		err = extractEntry(header, targetPath, fileIo, tr)
		if err != nil {
			return err
		}

		if fileToExtract != "" {
			log.Printf("File %s extracted to %s", fileToExtract, targetPath)
			return nil
		}
	}
	if fileToExtract != "" {
		return fmt.Errorf("file %s not found in archive", fileToExtract)
	}
	return nil
}

// StreamFileFromGzip creates a new streamer for a specific file in a tar.gz
func StreamFileFromGzip(reader io.Reader, filename string) (io.Reader, error) {
	uncompressedStream, err := gzip.NewReader(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to create a gzip reader: %w", err)
	}

	// Pass the decompressed stream to the tar reader.
	tarStreamer, err := streamFileFromArchive(tar.NewReader(uncompressedStream), filename)
	if tarStreamer == nil || err != nil {
		return nil, err
	}

	return tarStreamer, err
}

func streamFileFromArchive(tarReader *tar.Reader, filename string) (*tar.Reader, error) {
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			return nil, fmt.Errorf("file %s not found in archive", filename)
		}
		if err != nil {
			return nil, fmt.Errorf("failed reading tar archive: %w", err)
		}
		if header.FileInfo().Name() == filename {
			return tarReader, nil
		}
	}
}
