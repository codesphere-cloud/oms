package util

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
)

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
