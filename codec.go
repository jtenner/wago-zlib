// Package wagozlib provides bounded zlib-wrapped DEFLATE compression for Go
// and a capability-gated Wago guest plugin.
//
// The implementation delegates the compression algorithm and zlib framing to
// Go's maintained standard-library compress/zlib package. It does not accept
// raw DEFLATE or gzip streams.
package wagozlib

import (
	"bytes"
	"compress/zlib"
	"errors"
	"io"
)

var errOutputTooSmall = errors.New("wagozlib: output too small")

// fixedWriter is a transaction-local, fixed-capacity sink. It never grows.
type fixedWriter struct {
	buf []byte
	n   int
}

func (w *fixedWriter) Write(p []byte) (int, error) {
	remaining := len(w.buf) - w.n
	if len(p) <= remaining {
		copy(w.buf[w.n:], p)
		w.n += len(p)
		return len(p), nil
	}
	written := copy(w.buf[w.n:], p)
	w.n += written
	return written, errOutputTooSmall
}

// Compress writes one RFC 1950 zlib stream containing src into dst.
// level accepts the levels supported by compress/zlib: -2 through 9.
//
// The operation is transactional: dst is modified only when StatusOK is
// returned. src and dst may overlap. A successful empty input still produces a
// complete, non-empty zlib stream.
func Compress(dst, src []byte, level int) (written int, status Status) {
	staged := make([]byte, len(dst))
	sink := fixedWriter{buf: staged}
	writer, err := zlib.NewWriterLevel(&sink, level)
	if err != nil {
		return 0, StatusInvalidArgument
	}
	_, writeErr := writer.Write(src)
	closeErr := writer.Close()
	if errors.Is(writeErr, errOutputTooSmall) || errors.Is(closeErr, errOutputTooSmall) {
		return 0, StatusOutputTooSmall
	}
	if writeErr != nil || closeErr != nil {
		return 0, StatusInternalError
	}
	copy(dst, staged[:sink.n])
	return sink.n, StatusOK
}

// Decompress decodes exactly one complete RFC 1950 zlib stream from src into
// dst. It rejects raw DEFLATE, gzip, preset-dictionary streams, and any bytes
// after the zlib checksum. Reading through verified EOF is mandatory, so a
// successful result has checked the Adler-32 checksum.
//
// The operation is transactional: dst is modified only when StatusOK is
// returned. src and dst may overlap. When output does not fit, the function
// returns StatusOutputTooSmall without committing a prefix.
func Decompress(dst, src []byte) (written int, status Status) {
	source := bytes.NewReader(src) // ReadByte lets compress/zlib stop exactly at EOF.
	reader, err := zlib.NewReader(source)
	if err != nil {
		return 0, decodeStatus(err)
	}
	defer reader.Close()

	staged := make([]byte, len(dst))
	written = 0
	for written < len(staged) {
		n, readErr := reader.Read(staged[written:])
		written += n
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return finishDecompress(dst, staged[:written], source, reader)
			}
			return 0, decodeStatus(readErr)
		}
		if n == 0 {
			return 0, StatusInternalError
		}
	}

	// A full destination may be an exact fit. Read one byte to distinguish that
	// case from overflow and, for the exact fit, force checksum verification.
	var extra [1]byte
	for {
		n, readErr := reader.Read(extra[:])
		if n != 0 {
			return 0, StatusOutputTooSmall
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return finishDecompress(dst, staged, source, reader)
			}
			return 0, decodeStatus(readErr)
		}
		return 0, StatusInternalError
	}
}

func finishDecompress(dst, staged []byte, source *bytes.Reader, reader io.Closer) (int, Status) {
	if source.Len() != 0 {
		return 0, StatusTrailingData
	}
	if err := reader.Close(); err != nil {
		return 0, StatusInternalError
	}
	copy(dst, staged)
	return len(staged), StatusOK
}

func decodeStatus(err error) Status {
	switch {
	case errors.Is(err, zlib.ErrDictionary):
		return StatusDictionaryRequired
	case errors.Is(err, zlib.ErrChecksum):
		return StatusChecksumMismatch
	case errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF):
		return StatusTruncated
	case errors.Is(err, zlib.ErrHeader):
		return StatusInvalidData
	default:
		return StatusInvalidData
	}
}
