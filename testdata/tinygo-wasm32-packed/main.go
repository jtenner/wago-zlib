// Package main is an executable TinyGo-produced Wasm32 qualification guest for
// the single-result packed zlib ABI. The generated module is not checked in.
package main

import "unsafe"

const (
	statusOK              = uint32(0)
	statusInvalidArgument = uint32(1)
	statusOutputTooSmall  = uint32(4)
	statusTruncated       = uint32(6)
	statusChecksum        = uint32(7)
	sentinel              = byte(0xa5)
)

var (
	source     = [16]byte{'z', 'l', 'i', 'b', '-', 'w', 'a', 'g', 'o'}
	compressed [512]byte
	decoded    [64]byte
	overlap    [512]byte
	small      [8]byte
)

//go:wasmimport wago_zlib.wasm32 abi_version
func abiVersion() int32

//go:wasmimport wago_zlib.wasm32 compress_packed
func zlibCompress(sourceOffset, sourceLength, destinationOffset, destinationCapacity uint32, level int32) uint64

//go:wasmimport wago_zlib.wasm32 decompress_packed
func zlibDecompress(sourceOffset, sourceLength, destinationOffset, destinationCapacity uint32) uint64

func offset(data *byte) uint32 {
	return uint32(uintptr(unsafe.Pointer(data)))
}

func status(result uint64) uint32 {
	return uint32(result)
}

func written(result uint64) uint32 {
	return uint32(result >> 32)
}

func fill(data []byte, value byte) {
	for i := range data {
		data[i] = value
	}
}

func failureIs(result uint64, want uint32) bool {
	return status(result) == want && written(result) == 0
}

func makeStream() uint32 {
	fill(compressed[:], 0)
	result := zlibCompress(
		offset(&source[0]), 9,
		offset(&compressed[0]), uint32(len(compressed)), -1,
	)
	if status(result) != statusOK {
		return 0
	}
	return written(result)
}

//go:export roundtrip
func roundtrip() uint32 {
	if abiVersion() != 1 {
		return 1
	}
	compressedLength := makeStream()
	if compressedLength == 0 {
		return 2
	}
	fill(decoded[:], sentinel)
	result := zlibDecompress(
		offset(&compressed[0]), compressedLength,
		offset(&decoded[0]), uint32(len(decoded)),
	)
	if status(result) != statusOK || written(result) != 9 {
		return 3
	}
	for i := uint32(0); i < 9; i++ {
		if decoded[i] != source[i] {
			return 4
		}
	}
	return 0
}

//go:export empty
func empty() uint32 {
	result := zlibCompress(
		offset(&source[0]), 0,
		offset(&compressed[0]), uint32(len(compressed)), -1,
	)
	if status(result) != statusOK || written(result) == 0 {
		return 1
	}
	decoded[0] = sentinel
	result = zlibDecompress(
		offset(&compressed[0]), written(result),
		offset(&decoded[0]), 0,
	)
	if status(result) != statusOK || written(result) != 0 || decoded[0] != sentinel {
		return 2
	}
	return 0
}

//go:export overlap_case
func overlapCase() uint32 {
	fill(overlap[:], 0)
	for i := uint32(0); i < 9; i++ {
		overlap[i] = source[i]
	}
	result := zlibCompress(
		offset(&overlap[0]), 9,
		offset(&overlap[4]), uint32(len(overlap)-4), -1,
	)
	if status(result) != statusOK || written(result) == 0 {
		return 1
	}
	result = zlibDecompress(
		offset(&overlap[4]), written(result),
		offset(&overlap[0]), 9,
	)
	if status(result) != statusOK || written(result) != 9 {
		return 2
	}
	for i := uint32(0); i < 9; i++ {
		if overlap[i] != source[i] {
			return 3
		}
	}
	return 0
}

//go:export output_small
func outputSmall() uint32 {
	fill(small[:], sentinel)
	result := zlibCompress(
		offset(&source[0]), 0,
		offset(&small[0]), 1, -1,
	)
	if !failureIs(result, statusOutputTooSmall) {
		return 1
	}
	if small[0] != sentinel {
		return 2
	}
	compressedLength := makeStream()
	if compressedLength == 0 {
		return 3
	}
	fill(small[:], sentinel)
	result = zlibDecompress(
		offset(&compressed[0]), compressedLength,
		offset(&small[0]), 1,
	)
	if !failureIs(result, statusOutputTooSmall) {
		return 4
	}
	for i := range small {
		if small[i] != sentinel {
			return 5
		}
	}
	return 0
}

//go:export bounds
func bounds() uint32 {
	fill(small[:], sentinel)
	result := zlibCompress(
		^uint32(0)-3, 8,
		offset(&small[0]), uint32(len(small)), -1,
	)
	if !failureIs(result, statusInvalidArgument) {
		return 1
	}
	for i := range small {
		if small[i] != sentinel {
			return 2
		}
	}
	return 0
}

//go:export checksum
func checksum() uint32 {
	compressedLength := makeStream()
	if compressedLength == 0 {
		return 1
	}
	compressed[compressedLength-1] ^= 1
	fill(decoded[:], sentinel)
	result := zlibDecompress(
		offset(&compressed[0]), compressedLength,
		offset(&decoded[0]), uint32(len(decoded)),
	)
	if !failureIs(result, statusChecksum) {
		return 2
	}
	for i := range decoded {
		if decoded[i] != sentinel {
			return 3
		}
	}
	return 0
}

//go:export truncated
func truncated() uint32 {
	compressedLength := makeStream()
	if compressedLength < 2 {
		return 1
	}
	fill(decoded[:], sentinel)
	result := zlibDecompress(
		offset(&compressed[0]), compressedLength-1,
		offset(&decoded[0]), uint32(len(decoded)),
	)
	if !failureIs(result, statusTruncated) {
		return 2
	}
	for i := range decoded {
		if decoded[i] != sentinel {
			return 3
		}
	}
	return 0
}

//go:export invalid_level
func invalidLevel() uint32 {
	fill(small[:], sentinel)
	result := zlibCompress(
		offset(&source[0]), 9,
		offset(&small[0]), uint32(len(small)), 99,
	)
	if !failureIs(result, statusInvalidArgument) {
		return 1
	}
	for i := range small {
		if small[i] != sentinel {
			return 2
		}
	}
	return 0
}

func main() {}
