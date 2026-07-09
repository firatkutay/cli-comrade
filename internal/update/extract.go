package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"strings"
)

// ExtractBinary reads binaryName's file content out of archive — a
// .tar.gz (linux/darwin) or .zip (windows) archive, dispatched purely on
// archiveName's extension, matching the format/format_overrides split in
// .goreleaser.yaml's archives block. Only the standard library
// (archive/tar, archive/zip, compress/gzip) is used — no new dependency.
func ExtractBinary(archive []byte, archiveName, binaryName string) ([]byte, error) {
	switch {
	case strings.HasSuffix(archiveName, ".tar.gz"):
		return extractFromTarGz(archive, binaryName)
	case strings.HasSuffix(archiveName, ".zip"):
		return extractFromZip(archive, binaryName)
	default:
		return nil, fmt.Errorf("update: unrecognized archive format for %s", archiveName)
	}
}

func extractFromTarGz(archive []byte, binaryName string) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, fmt.Errorf("update: open gzip archive: %w", err)
	}
	defer func() { _ = gz.Close() }()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("update: read tar archive: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg || hdr.Name != binaryName {
			continue
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			return nil, fmt.Errorf("update: read %s from tar archive: %w", binaryName, err)
		}
		return data, nil
	}
	return nil, fmt.Errorf("update: %s not found in tar archive", binaryName)
}

func extractFromZip(archive []byte, binaryName string) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return nil, fmt.Errorf("update: open zip archive: %w", err)
	}
	for _, f := range zr.File {
		if f.Name != binaryName {
			continue
		}
		data, err := readZipEntry(f)
		if err != nil {
			return nil, err
		}
		return data, nil
	}
	return nil, fmt.Errorf("update: %s not found in zip archive", binaryName)
}

// readZipEntry opens and fully reads one *zip.File, closing it before
// returning regardless of outcome — split out of extractFromZip so the
// open/close/read triple never needs a defer inside that function's
// loop.
func readZipEntry(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, fmt.Errorf("update: open %s in zip archive: %w", f.Name, err)
	}
	defer func() { _ = rc.Close() }()

	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, fmt.Errorf("update: read %s from zip archive: %w", f.Name, err)
	}
	return data, nil
}
