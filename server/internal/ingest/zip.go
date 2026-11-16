package ingest

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"os"
	"path"
	"strings"
)

const (
	CodeZipUnsafe          = "zip_unsafe"
	CodeUnsupportedPayload = "unsupported_payload"
	CodeDiskFull           = "disk_full"
	CodeCommitFailed       = "commit_failed"
)

// Error is a stable ingest failure. The text does not include health values.
type Error struct {
	Code string
	Msg  string
}

func (e *Error) Error() string {
	if e.Msg == "" {
		return e.Code
	}
	return e.Code + ": " + e.Msg
}

// Limits bound a zip payload.
type Limits struct {
	MaxEntries int
	MaxBytes   int64
}

// Classify reports "zip" or "xml".
func Classify(filePath string) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close()
	buf := make([]byte, 8)
	n, err := io.ReadFull(f, buf)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return "", err
	}
	buf = buf[:n]
	if isZip(buf) {
		return "zip", nil
	}
	if looksLikeXML(buf) {
		return "xml", nil
	}
	return "", &Error{Code: CodeUnsupportedPayload}
}

// OpenXML returns a reader for export.xml. The caller must close it.
func OpenXML(filePath string, limits Limits) (io.ReadCloser, error) {
	kind, err := Classify(filePath)
	if err != nil {
		return nil, err
	}
	if kind == "xml" {
		f, err := os.Open(filePath)
		if err != nil {
			return nil, err
		}
		return &limitedReadCloser{Reader: &limited{r: f, max: limits.MaxBytes}, closer: f}, nil
	}
	zr, err := zip.OpenReader(filePath)
	if err != nil {
		return nil, &Error{Code: CodeZipUnsafe, Msg: "open"}
	}
	if limits.MaxEntries > 0 && len(zr.File) > limits.MaxEntries {
		_ = zr.Close()
		return nil, &Error{Code: CodeZipUnsafe, Msg: "entries"}
	}
	var total uint64
	var match *zip.File
	for _, file := range zr.File {
		if unsafeZipName(file.Name) {
			_ = zr.Close()
			return nil, &Error{Code: CodeZipUnsafe, Msg: "name"}
		}
		total += file.UncompressedSize64
		if path.Base(strings.ReplaceAll(file.Name, "\\", "/")) == "export.xml" && match == nil {
			match = file
		}
	}
	if limits.MaxBytes > 0 && int64(total) > limits.MaxBytes {
		_ = zr.Close()
		return nil, &Error{Code: CodeZipUnsafe, Msg: "size"}
	}
	if match == nil {
		_ = zr.Close()
		return nil, &Error{Code: CodeZipUnsafe, Msg: "export.xml"}
	}
	rc, err := match.Open()
	if err != nil {
		_ = zr.Close()
		return nil, &Error{Code: CodeZipUnsafe, Msg: "read"}
	}
	return &limitedReadCloser{Reader: &limited{r: rc, max: limits.MaxBytes}, closer: multiCloser{rc, zr}}, nil
}

func isZip(buf []byte) bool {
	return bytes.HasPrefix(buf, []byte{'P', 'K', 0x03, 0x04}) ||
		bytes.HasPrefix(buf, []byte{'P', 'K', 0x05, 0x06}) ||
		bytes.HasPrefix(buf, []byte{'P', 'K', 0x07, 0x08})
}

func looksLikeXML(buf []byte) bool {
	if bytes.HasPrefix(buf, []byte{0xEF, 0xBB, 0xBF}) {
		buf = buf[3:]
	}
	buf = bytes.TrimLeft(buf, " \t\r\n")
	return bytes.HasPrefix(buf, []byte("<"))
}

func unsafeZipName(name string) bool {
	slash := strings.ReplaceAll(name, "\\", "/")
	if slash == "" || strings.HasPrefix(slash, "/") || strings.Contains(slash, ":") {
		return true
	}
	cleaned := path.Clean(slash)
	return cleaned == ".." || strings.HasPrefix(cleaned, "../") || strings.HasPrefix(cleaned, "/")
}

type limited struct {
	r   io.Reader
	n   int64
	max int64
}

func (l *limited) Read(p []byte) (int, error) {
	if l.max > 0 && l.n >= l.max {
		return 0, &Error{Code: CodeZipUnsafe, Msg: "size"}
	}
	if l.max > 0 && int64(len(p)) > l.max-l.n {
		p = p[:l.max-l.n]
	}
	n, err := l.r.Read(p)
	l.n += int64(n)
	return n, err
}

type limitedReadCloser struct {
	io.Reader
	closer io.Closer
}

func (l *limitedReadCloser) Close() error { return l.closer.Close() }

type multiCloser []io.Closer

func (m multiCloser) Close() error {
	var first error
	for _, c := range m {
		if err := c.Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}
