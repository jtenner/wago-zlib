package wagozlib

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"strings"
	"testing"
)

func zlibStream(t testing.TB, payload []byte) []byte {
	t.Helper()
	var out bytes.Buffer
	w := zlib.NewWriter(&out)
	if _, err := w.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func TestRoundTripLevels(t *testing.T) {
	payload := bytes.Repeat([]byte("bounded-zlib\x00"), 257)
	for _, level := range []int{flate.HuffmanOnly, flate.DefaultCompression, flate.NoCompression, flate.BestSpeed, flate.BestCompression} {
		t.Run(Status(level).String(), func(t *testing.T) {
			compressed := make([]byte, len(payload)+512)
			n, status := Compress(compressed, payload, level)
			if status != StatusOK || n == 0 {
				t.Fatalf("Compress = %d, %s", n, status)
			}
			decoded := make([]byte, len(payload))
			m, status := Decompress(decoded, compressed[:n])
			if status != StatusOK || m != len(payload) || !bytes.Equal(decoded, payload) {
				t.Fatalf("Decompress = %d, %s, equal=%v", m, status, bytes.Equal(decoded, payload))
			}
		})
	}
}

func TestEmptyPayloadAndEmptyInputAreDistinct(t *testing.T) {
	compressed := make([]byte, 32)
	n, status := Compress(compressed, nil, flate.DefaultCompression)
	if status != StatusOK || n == 0 {
		t.Fatalf("empty Compress = %d, %s", n, status)
	}
	if written, status := Decompress(nil, compressed[:n]); status != StatusOK || written != 0 {
		t.Fatalf("empty-payload Decompress = %d, %s", written, status)
	}
	if written, status := Decompress(nil, nil); status != StatusTruncated || written != 0 {
		t.Fatalf("empty-input Decompress = %d, %s", written, status)
	}
}

func TestCompressionRejectsInvalidLevelTransactionally(t *testing.T) {
	dst := bytes.Repeat([]byte{0xa5}, 128)
	want := append([]byte(nil), dst...)
	if n, status := Compress(dst, []byte("x"), 10); status != StatusInvalidArgument || n != 0 {
		t.Fatalf("Compress invalid level = %d, %s", n, status)
	}
	if !bytes.Equal(dst, want) {
		t.Fatal("destination changed on invalid compression level")
	}
}

func TestOutputOverflowIsTransactional(t *testing.T) {
	payload := bytes.Repeat([]byte("abcdefgh"), 256)

	compressedDst := bytes.Repeat([]byte{0x5a}, 4)
	compressedWant := append([]byte(nil), compressedDst...)
	if n, status := Compress(compressedDst, payload, flate.DefaultCompression); status != StatusOutputTooSmall || n != 0 {
		t.Fatalf("small Compress = %d, %s", n, status)
	}
	if !bytes.Equal(compressedDst, compressedWant) {
		t.Fatal("compression overflow committed a prefix")
	}

	stream := zlibStream(t, payload)
	decodedDst := bytes.Repeat([]byte{0x6b}, len(payload)-1)
	decodedWant := append([]byte(nil), decodedDst...)
	if n, status := Decompress(decodedDst, stream); status != StatusOutputTooSmall || n != 0 {
		t.Fatalf("small Decompress = %d, %s", n, status)
	}
	if !bytes.Equal(decodedDst, decodedWant) {
		t.Fatal("decompression overflow committed a prefix")
	}
}

func TestExactFitForcesChecksumCompletion(t *testing.T) {
	payload := bytes.Repeat([]byte("x"), 4096)
	stream := zlibStream(t, payload)
	dst := make([]byte, len(payload))
	if n, status := Decompress(dst, stream); status != StatusOK || n != len(payload) || !bytes.Equal(dst, payload) {
		t.Fatalf("exact fit = %d, %s", n, status)
	}

	corrupt := append([]byte(nil), stream...)
	corrupt[len(corrupt)-1] ^= 1
	copy(dst, bytes.Repeat([]byte{0x7c}, len(dst)))
	before := append([]byte(nil), dst...)
	if n, status := Decompress(dst, corrupt); status != StatusChecksumMismatch || n != 0 {
		t.Fatalf("exact-fit checksum = %d, %s", n, status)
	}
	if !bytes.Equal(dst, before) {
		t.Fatal("checksum failure changed exact-fit destination")
	}
}

func TestMalformedInputStatusesAndNoCommit(t *testing.T) {
	payload := bytes.Repeat([]byte("payload"), 32)
	valid := zlibStream(t, payload)

	var raw bytes.Buffer
	rawWriter, err := flate.NewWriter(&raw, flate.DefaultCompression)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = rawWriter.Write(payload)
	_ = rawWriter.Close()

	var gz bytes.Buffer
	gzWriter := gzip.NewWriter(&gz)
	_, _ = gzWriter.Write(payload)
	_ = gzWriter.Close()

	corrupt := append([]byte(nil), valid...)
	corrupt[len(corrupt)-1] ^= 1
	trailing := append(append([]byte(nil), valid...), 0)
	tests := []struct {
		name string
		in   []byte
		want Status
	}{
		{"empty", nil, StatusTruncated},
		{"one-byte-header", valid[:1], StatusTruncated},
		{"truncated-body", valid[:len(valid)-2], StatusTruncated},
		{"checksum", corrupt, StatusChecksumMismatch},
		{"trailing", trailing, StatusTrailingData},
		{"raw-deflate", raw.Bytes(), StatusInvalidData},
		{"gzip", gz.Bytes(), StatusInvalidData},
		{"nonsense", []byte("not zlib"), StatusInvalidData},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dst := bytes.Repeat([]byte{0x3c}, len(payload)+32)
			before := append([]byte(nil), dst...)
			n, status := Decompress(dst, test.in)
			if status != test.want || n != 0 {
				t.Fatalf("Decompress = %d, %s; want 0, %s", n, status, test.want)
			}
			if !bytes.Equal(dst, before) {
				t.Fatal("malformed input changed destination")
			}
		})
	}
}

func TestEveryProperStreamPrefixIsTruncated(t *testing.T) {
	payload := make([]byte, 4096)
	for i := range payload {
		payload[i] = byte(i*131 + 17)
	}
	stream := zlibStream(t, payload)
	for end := 0; end < len(stream); end++ {
		if _, status := Decompress(make([]byte, len(payload)), stream[:end]); status != StatusTruncated {
			t.Fatalf("prefix %d/%d = %s; want truncated", end, len(stream), status)
		}
	}
}

func TestPresetDictionaryRejected(t *testing.T) {
	var stream bytes.Buffer
	w, err := zlib.NewWriterLevelDict(&stream, flate.DefaultCompression, []byte("shared dictionary"))
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.Write([]byte("shared dictionary payload"))
	_ = w.Close()
	if n, status := Decompress(make([]byte, 64), stream.Bytes()); status != StatusDictionaryRequired || n != 0 {
		t.Fatalf("dictionary stream = %d, %s", n, status)
	}
}

func TestOverlappingBuffers(t *testing.T) {
	payload := bytes.Repeat([]byte("overlap"), 128)
	backing := make([]byte, len(payload)+1024)
	copy(backing, payload)
	n, status := Compress(backing[64:], backing[:len(payload)], flate.DefaultCompression)
	if status != StatusOK {
		t.Fatalf("overlapping Compress = %d, %s", n, status)
	}
	stream := append([]byte(nil), backing[64:64+n]...)

	backing = make([]byte, len(payload)+len(stream)+64)
	copy(backing[32:], stream)
	m, status := Decompress(backing[:len(payload)], backing[32:32+len(stream)])
	if status != StatusOK || m != len(payload) || !bytes.Equal(backing[:m], payload) {
		t.Fatalf("overlapping Decompress = %d, %s", m, status)
	}
}

func TestStatusStringsCoverABI(t *testing.T) {
	for status := StatusOK; status <= StatusInternalError; status++ {
		if got := status.String(); got == "" || strings.HasPrefix(got, "status(") {
			t.Fatalf("status %d string = %q", status, got)
		}
	}
	if got := Status(99).String(); got != "status(99)" {
		t.Fatalf("unknown status string = %q", got)
	}
}

func BenchmarkCompress1KiB(b *testing.B) {
	src := bytes.Repeat([]byte("abcdefgh"), 128)
	dst := make([]byte, 2<<10)
	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	for i := 0; i < b.N; i++ {
		if _, status := Compress(dst, src, flate.DefaultCompression); status != StatusOK {
			b.Fatal(status)
		}
	}
}

func BenchmarkDecompress1KiB(b *testing.B) {
	src := bytes.Repeat([]byte("abcdefgh"), 128)
	stream := zlibStream(b, src)
	dst := make([]byte, len(src))
	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	for i := 0; i < b.N; i++ {
		if _, status := Decompress(dst, stream); status != StatusOK {
			b.Fatal(status)
		}
	}
}
