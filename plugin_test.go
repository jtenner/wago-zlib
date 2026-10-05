package wagozlib

import (
	"bytes"
	"compress/flate"
	"context"
	"encoding/json"
	"testing"

	wago "github.com/wago-org/wago"
)

const (
	wasmI32 = byte(0x7f)
	wasmI64 = byte(0x7e)
)

func wasmULEB(v uint64) []byte {
	var out []byte
	for {
		b := byte(v & 0x7f)
		v >>= 7
		if v != 0 {
			b |= 0x80
		}
		out = append(out, b)
		if v == 0 {
			return out
		}
	}
}

func wasmSLEB32(v int32) []byte {
	var out []byte
	for {
		b := byte(v & 0x7f)
		v >>= 7
		done := (v == 0 && b&0x40 == 0) || (v == -1 && b&0x40 != 0)
		if !done {
			b |= 0x80
		}
		out = append(out, b)
		if done {
			return out
		}
	}
}

func wasmName(s string) []byte {
	return append(wasmULEB(uint64(len(s))), s...)
}

func wasmVec(entries ...[]byte) []byte {
	out := wasmULEB(uint64(len(entries)))
	for _, entry := range entries {
		out = append(out, entry...)
	}
	return out
}

func wasmSection(id byte, payload []byte) []byte {
	out := []byte{id}
	out = append(out, wasmULEB(uint64(len(payload)))...)
	return append(out, payload...)
}

func wasmFuncType(params, results []byte) []byte {
	out := []byte{0x60}
	out = append(out, wasmULEB(uint64(len(params)))...)
	out = append(out, params...)
	out = append(out, wasmULEB(uint64(len(results)))...)
	return append(out, results...)
}

func wasmFuncTypeLogical(params []byte, paramCount uint64, results []byte, resultCount uint64) []byte {
	out := []byte{0x60}
	out = append(out, wasmULEB(paramCount)...)
	out = append(out, params...)
	out = append(out, wasmULEB(resultCount)...)
	return append(out, results...)
}

func wasmImportFunc(module, name string, typeIndex uint64) []byte {
	out := append(wasmName(module), wasmName(name)...)
	out = append(out, 0x00)
	return append(out, wasmULEB(typeIndex)...)
}

func wasmExport(name string, kind byte, index uint64) []byte {
	out := wasmName(name)
	out = append(out, kind)
	return append(out, wasmULEB(index)...)
}

func wasmModule(sections ...[]byte) []byte {
	out := []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}
	for _, section := range sections {
		out = append(out, section...)
	}
	return out
}

func linearMemoryGuestModule(module string, memory64 bool) []byte {
	pointer := wasmI32
	compressResults := []byte{wasmI32, wasmI32}
	memoryType := []byte{0x00, 0x01}
	if memory64 {
		pointer = wasmI64
		compressResults = []byte{wasmI32, wasmI64}
		memoryType = []byte{0x04, 0x01}
	}
	abiType := wasmFuncType(nil, []byte{wasmI32})
	compressType := wasmFuncType([]byte{pointer, pointer, pointer, pointer, wasmI32}, compressResults)
	decompressType := wasmFuncType([]byte{pointer, pointer, pointer, pointer}, compressResults)
	return wasmModule(
		wasmSection(1, wasmVec(abiType, compressType, decompressType)),
		wasmSection(2, wasmVec(
			wasmImportFunc(module, "abi_version", 0),
			wasmImportFunc(module, "compress", 1),
			wasmImportFunc(module, "decompress", 2),
		)),
		wasmSection(5, wasmVec(memoryType)),
		wasmSection(7, wasmVec(
			wasmExport("abi_version", 0x00, 0),
			wasmExport("compress", 0x00, 1),
			wasmExport("decompress", 0x00, 2),
			wasmExport("memory", 0x02, 0),
		)),
	)
}

func gcGuestModule() []byte {
	arrayType := []byte{0x5e, 0x78, 0x01} // (array (mut i8))
	refArray := []byte{0x63, 0x00}        // (ref null 0)
	abiType := wasmFuncType(nil, []byte{wasmI32})
	compressParams := append(append([]byte{}, refArray...), wasmI32, wasmI32)
	compressParams = append(compressParams, refArray...)
	compressParams = append(compressParams, wasmI32, wasmI32, wasmI32)
	compressType := wasmFuncTypeLogical(compressParams, 7, []byte{wasmI32, wasmI32}, 2)
	decompressParams := append(append([]byte{}, refArray...), wasmI32, wasmI32)
	decompressParams = append(decompressParams, refArray...)
	decompressParams = append(decompressParams, wasmI32, wasmI32)
	decompressType := wasmFuncTypeLogical(decompressParams, 6, []byte{wasmI32, wasmI32}, 2)
	runType := wasmFuncType(nil, []byte{wasmI32})
	errorType := wasmFuncType(nil, []byte{wasmI32, wasmI32})

	roundTripBody := []byte{
		0x02, 0x03, 0x63, 0x00, 0x01, 0x7f, // 3 array refs, 1 i32 local
		0x41, 0x01, 0x41, 0x02, 0x41, 0x03, 0xfb, 0x08, 0x00, 0x03, 0x21, 0x00, // src
		0x41, 0x20, 0xfb, 0x07, 0x00, 0x21, 0x01, // compressed[32]
		0x20, 0x00, 0x41, 0x00, 0x41, 0x03,
		0x20, 0x01, 0x41, 0x00, 0x41, 0x20, 0x41, 0x7f, 0x10, 0x01, // compress
		0x21, 0x03, 0x1a, // save written, drop status
		0x41, 0x03, 0xfb, 0x07, 0x00, 0x21, 0x02, // decoded[3]
		0x20, 0x01, 0x41, 0x00, 0x20, 0x03,
		0x20, 0x02, 0x41, 0x00, 0x41, 0x03, 0x10, 0x02, // decompress
		0x1a, 0x1a, // drop written and status
		0x20, 0x02, 0x41, 0x00, 0xfb, 0x0d, 0x00, 0x41, 0x01, 0x46,
		0x20, 0x02, 0x41, 0x01, 0xfb, 0x0d, 0x00, 0x41, 0x02, 0x46, 0x71,
		0x20, 0x02, 0x41, 0x02, 0xfb, 0x0d, 0x00, 0x41, 0x03, 0x46, 0x71,
		0x0b,
	}
	overlapBody := []byte{
		0x02, 0x01, 0x63, 0x00, 0x01, 0x7f, // 1 array ref, 1 i32 local
		0x41, 0x20, 0xfb, 0x07, 0x00, 0x21, 0x00, // bytes[32]
		0x20, 0x00, 0x41, 0x00, 0x41, 0x01, 0xfb, 0x0e, 0x00,
		0x20, 0x00, 0x41, 0x01, 0x41, 0x02, 0xfb, 0x0e, 0x00,
		0x20, 0x00, 0x41, 0x02, 0x41, 0x03, 0xfb, 0x0e, 0x00,
		0x20, 0x00, 0x41, 0x00, 0x41, 0x03,
		0x20, 0x00, 0x41, 0x01, 0x41, 0x1f, 0x41, 0x7f, 0x10, 0x01, // overlapping compress
		0x21, 0x01, 0x1a,
		0x20, 0x00, 0x41, 0x01, 0x20, 0x01,
		0x20, 0x00, 0x41, 0x00, 0x41, 0x03, 0x10, 0x02, // overlapping decompress
		0x1a, 0x1a,
		0x20, 0x00, 0x41, 0x00, 0xfb, 0x0d, 0x00, 0x41, 0x01, 0x46,
		0x20, 0x00, 0x41, 0x01, 0xfb, 0x0d, 0x00, 0x41, 0x02, 0x46, 0x71,
		0x20, 0x00, 0x41, 0x02, 0xfb, 0x0d, 0x00, 0x41, 0x03, 0x46, 0x71,
		0x0b,
	}
	boundsBody := []byte{
		0x01, 0x01, 0x63, 0x00, // 1 array ref
		0x41, 0x09, 0x41, 0x04, 0xfb, 0x06, 0x00, 0x21, 0x00, // four 9 bytes
		0x20, 0x00, 0x41, 0x00, 0x41, 0x05,
		0x20, 0x00, 0x41, 0x00, 0x41, 0x04, 0x10, 0x02,
		0x1a,                                                       // drop written; keep status
		0x20, 0x00, 0x41, 0x00, 0xfb, 0x0d, 0x00, 0x41, 0x09, 0x46, // unchanged
		0x0b,
	}
	malformedBody := []byte{
		0x01, 0x02, 0x63, 0x00, // 2 array refs
		0x41, 0x01, 0x41, 0x02, 0x41, 0x03, 0xfb, 0x08, 0x00, 0x03, 0x21, 0x00,
		0x41, 0x09, 0x41, 0x09, 0x41, 0x09, 0xfb, 0x08, 0x00, 0x03, 0x21, 0x01,
		0x20, 0x00, 0x41, 0x00, 0x41, 0x03,
		0x20, 0x01, 0x41, 0x00, 0x41, 0x03, 0x10, 0x02,
		0x1a, // drop written; keep status
		0x20, 0x01, 0x41, 0x00, 0xfb, 0x0d, 0x00, 0x41, 0x09, 0x46,
		0x20, 0x01, 0x41, 0x01, 0xfb, 0x0d, 0x00, 0x41, 0x09, 0x46, 0x71,
		0x20, 0x01, 0x41, 0x02, 0xfb, 0x0d, 0x00, 0x41, 0x09, 0x46, 0x71,
		0x0b,
	}
	code := func(body []byte) []byte { return append(wasmULEB(uint64(len(body))), body...) }
	return wasmModule(
		wasmSection(1, wasmVec(arrayType, abiType, compressType, decompressType, runType, errorType)),
		wasmSection(2, wasmVec(
			wasmImportFunc(ModuleGC, "abi_version", 1),
			wasmImportFunc(ModuleGC, "compress", 2),
			wasmImportFunc(ModuleGC, "decompress", 3),
		)),
		wasmSection(3, wasmVec(wasmULEB(4), wasmULEB(4), wasmULEB(5), wasmULEB(5))),
		wasmSection(7, wasmVec(
			wasmExport("abi_version", 0x00, 0),
			wasmExport("run", 0x00, 3),
			wasmExport("overlap", 0x00, 4),
			wasmExport("bounds", 0x00, 5),
			wasmExport("malformed", 0x00, 6),
		)),
		wasmSection(10, wasmVec(code(roundTripBody), code(overlapBody), code(boundsBody), code(malformedBody))),
	)
}

func gcImmutableInputModule(stream []byte) []byte {
	immutableArray := []byte{0x5e, 0x78, 0x00}
	mutableArray := []byte{0x5e, 0x78, 0x01}
	refImmutable := []byte{0x63, 0x00}
	refMutable := []byte{0x63, 0x01}
	params := append(append([]byte{}, refImmutable...), wasmI32, wasmI32)
	params = append(params, refMutable...)
	params = append(params, wasmI32, wasmI32)
	decompressType := wasmFuncTypeLogical(params, 6, []byte{wasmI32, wasmI32}, 2)
	runType := wasmFuncType(nil, []byte{wasmI32})
	body := []byte{0x02, 0x01, 0x63, 0x00, 0x01, 0x63, 0x01} // one ref of each type
	for _, value := range stream {
		body = append(body, 0x41)
		body = append(body, wasmSLEB32(int32(value))...)
	}
	body = append(body, 0xfb, 0x08, 0x00)
	body = append(body, wasmULEB(uint64(len(stream)))...)
	body = append(body,
		0x21, 0x00,
		0x41, 0x03, 0xfb, 0x07, 0x01, 0x21, 0x01,
		0x20, 0x00, 0x41, 0x00,
	)
	body = append(body, 0x41)
	body = append(body, wasmSLEB32(int32(len(stream)))...)
	body = append(body,
		0x20, 0x01, 0x41, 0x00, 0x41, 0x03, 0x10, 0x00,
		0x1a, 0x1a,
		0x20, 0x01, 0x41, 0x00, 0xfb, 0x0d, 0x01, 0x41, 0x01, 0x46,
		0x20, 0x01, 0x41, 0x01, 0xfb, 0x0d, 0x01, 0x41, 0x02, 0x46, 0x71,
		0x20, 0x01, 0x41, 0x02, 0xfb, 0x0d, 0x01, 0x41, 0x03, 0x46, 0x71,
		0x0b,
	)
	code := append(wasmULEB(uint64(len(body))), body...)
	return wasmModule(
		wasmSection(1, wasmVec(immutableArray, mutableArray, decompressType, runType)),
		wasmSection(2, wasmVec(wasmImportFunc(ModuleGC, "decompress", 2))),
		wasmSection(3, wasmVec(wasmULEB(3))),
		wasmSection(7, wasmVec(wasmExport("run", 0x00, 1))),
		wasmSection(10, wasmVec(code)),
	)
}

func gcRejectedArrayModule(storage, mutability byte) []byte {
	arrayType := []byte{0x5e, storage, mutability}
	refArray := []byte{0x63, 0x00}
	decompressParams := append(append([]byte{}, refArray...), wasmI32, wasmI32)
	decompressParams = append(decompressParams, refArray...)
	decompressParams = append(decompressParams, wasmI32, wasmI32)
	decompressType := wasmFuncTypeLogical(decompressParams, 6, []byte{wasmI32, wasmI32}, 2)
	runType := wasmFuncType(nil, []byte{wasmI32})
	body := []byte{
		0x01, 0x01, 0x63, 0x00, // 1 array ref
		0x41, 0x04, 0xfb, 0x07, 0x00, 0x21, 0x00,
		0x20, 0x00, 0x41, 0x00, 0x41, 0x00,
		0x20, 0x00, 0x41, 0x00, 0x41, 0x00, 0x10, 0x00,
		0x1a, // drop written; return status
		0x0b,
	}
	code := append(wasmULEB(uint64(len(body))), body...)
	return wasmModule(
		wasmSection(1, wasmVec(arrayType, decompressType, runType)),
		wasmSection(2, wasmVec(wasmImportFunc(ModuleGC, "decompress", 1))),
		wasmSection(3, wasmVec(wasmULEB(2))),
		wasmSection(7, wasmVec(wasmExport("run", 0x00, 1))),
		wasmSection(10, wasmVec(code)),
	)
}

func gcOversizedImmutableInputModule() []byte {
	immutableArray := []byte{0x5e, 0x78, 0x00}
	mutableArray := []byte{0x5e, 0x78, 0x01}
	refImmutable := []byte{0x63, 0x00}
	refMutable := []byte{0x63, 0x01}
	params := append(append([]byte{}, refImmutable...), wasmI32, wasmI32)
	params = append(params, refMutable...)
	params = append(params, wasmI32, wasmI32)
	decompressType := wasmFuncTypeLogical(params, 6, []byte{wasmI32, wasmI32}, 2)
	runType := wasmFuncType(nil, []byte{wasmI32})
	body := []byte{
		0x02, 0x01, 0x63, 0x00, 0x01, 0x63, 0x01, // one ref of each array type
		0x41, 0x05, 0xfb, 0x07, 0x00, 0x21, 0x00,
		0x41, 0x00, 0xfb, 0x07, 0x01, 0x21, 0x01,
		0x20, 0x00, 0x41, 0x00, 0x41, 0x00,
		0x20, 0x01, 0x41, 0x00, 0x41, 0x00, 0x10, 0x00,
		0x1a, // drop written; return status
		0x0b,
	}
	code := append(wasmULEB(uint64(len(body))), body...)
	return wasmModule(
		wasmSection(1, wasmVec(immutableArray, mutableArray, decompressType, runType)),
		wasmSection(2, wasmVec(wasmImportFunc(ModuleGC, "decompress", 2))),
		wasmSection(3, wasmVec(wasmULEB(3))),
		wasmSection(7, wasmVec(wasmExport("run", 0x00, 1))),
		wasmSection(10, wasmVec(code)),
	)
}

func selectedPluginSet(t testing.TB, config json.RawMessage) wago.PluginSet {
	return selectedProviderSet(t, Provider(), config)
}

func selectedProviderSet(t testing.TB, provider wago.PluginProvider, config json.RawMessage) wago.PluginSet {
	t.Helper()
	digest, err := wago.DefinitionDigest(provider.Definition)
	if err != nil {
		t.Fatal(err)
	}
	grants := make([]wago.AuthorityGrant, len(provider.Definition.Authorities))
	for i, request := range provider.Definition.Authorities {
		grants[i] = wago.AuthorityGrant{Name: request.Name, Scope: request.Scope}
	}
	return wago.PluginSet{
		Providers: []wago.PluginProvider{provider},
		Selections: []wago.PluginSelection{{
			ID:               provider.Definition.ID,
			DefinitionDigest: digest,
			Direct:           true,
			Dependencies:     map[string]string{},
			Grants:           grants,
			Config:           config,
		}},
	}
}

func newTestRuntime(t testing.TB, config json.RawMessage) *wago.Runtime {
	t.Helper()
	runtimeConfig := wago.NewRuntimeConfig().WithCoreFeatures(wago.CoreFeaturesV3)
	runtime := wago.NewRuntime(wago.WithRuntimeConfig(runtimeConfig))
	if err := runtime.LoadPlugins(context.Background(), selectedPluginSet(t, config)); err != nil {
		runtime.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close() })
	return runtime
}

func TestDefinitionAndConfig(t *testing.T) {
	first, second := Definition(), Definition()
	if first.ID != ID || first.Provenance.License != "Apache-2.0" || len(first.Authorities) != 1 {
		t.Fatalf("definition = %+v", first)
	}
	first.ConfigSchema[0] ^= 1
	if bytes.Equal(first.ConfigSchema, second.ConfigSchema) {
		t.Fatal("Definition returned aliased config schema")
	}
	if err := validateConfig(nil); err != nil {
		t.Fatalf("default config: %v", err)
	}
	for _, raw := range []string{
		`null`,
		`{"unknown":1}`,
		`{"maxInputBytes":null}`,
		`{"maxInputBytes":0}`,
		`{"maxOutputBytes":0}`,
		`{"maxConcurrentOperations":0}`,
		`{"maxInputBytes":67108865}`,
		`{"maxOutputBytes":67108865}`,
		`{"maxConcurrentOperations":9}`,
		`{} {}`,
	} {
		if err := validateConfig(json.RawMessage(raw)); err == nil {
			t.Fatalf("config %s accepted", raw)
		}
	}
}

func TestOperationSlotsAreNonBlockingAndPerPlugin(t *testing.T) {
	first := &Plugin{slots: make(chan struct{}, 1)}
	second := &Plugin{slots: make(chan struct{}, 1)}
	if !first.acquire() || first.acquire() {
		t.Fatal("first plugin did not enforce its one-operation bound")
	}
	if !second.acquire() {
		t.Fatal("separate plugin shared the first plugin's operation bound")
	}
	first.release()
	second.release()
}

func TestWagoLinearMemoryEndToEnd(t *testing.T) {
	for _, test := range []struct {
		name     string
		module   string
		memory64 bool
	}{
		{"wasm32", ModuleWasm32, false},
		{"wasm64", ModuleWasm64, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			runtime := newTestRuntime(t, nil)
			compiled, err := runtime.Compile(linearMemoryGuestModule(test.module, test.memory64))
			if err != nil {
				t.Fatal(err)
			}
			defer compiled.Close()
			instance, err := runtime.Instantiate(context.Background(), compiled)
			if err != nil {
				t.Fatal(err)
			}
			defer instance.Close()
			version, err := instance.Invoke("abi_version")
			if err != nil || len(version) != 1 || wago.AsI32(version[0]) != ABIVersion {
				t.Fatalf("abi_version = %v, %v", version, err)
			}

			payload := bytes.Repeat([]byte("guest-zlib"), 128)
			memory := instance.Memory().UnsafeBytes()
			copy(memory[256:], payload)
			compressed, err := instance.Invoke("compress", 256, uint64(len(payload)), 2048, 4096, wago.I32(flate.DefaultCompression))
			if err != nil || Status(wago.AsI32(compressed[0])) != StatusOK {
				t.Fatalf("compress = %v, %v", compressed, err)
			}
			compressedLen := compressed[1]
			decoded, err := instance.Invoke("decompress", 2048, compressedLen, 3072, uint64(len(payload)))
			if err != nil || Status(wago.AsI32(decoded[0])) != StatusOK || decoded[1] != uint64(len(payload)) {
				t.Fatalf("decompress = %v, %v", decoded, err)
			}
			if !bytes.Equal(memory[3072:3072+len(payload)], payload) {
				t.Fatal("guest round trip mismatch")
			}

			copy(memory[8000:], payload)
			overlapCompressed, err := instance.Invoke("compress", 8000, uint64(len(payload)), 8064, 4096, wago.I32(flate.DefaultCompression))
			if err != nil || Status(wago.AsI32(overlapCompressed[0])) != StatusOK {
				t.Fatalf("overlapping compress = %v, %v", overlapCompressed, err)
			}
			overlapDecoded, err := instance.Invoke("decompress", 8064, overlapCompressed[1], 8000, uint64(len(payload)))
			if err != nil || Status(wago.AsI32(overlapDecoded[0])) != StatusOK || overlapDecoded[1] != uint64(len(payload)) {
				t.Fatalf("overlapping decompress = %v, %v", overlapDecoded, err)
			}
			if !bytes.Equal(memory[8000:8000+len(payload)], payload) {
				t.Fatal("overlapping guest round trip mismatch")
			}

			copy(memory[12000:], memory[2048:2048+compressedLen])
			memory[12000+compressedLen] = 0xee
			copy(memory[14000:14000+len(payload)], bytes.Repeat([]byte{0xa7}, len(payload)))
			trailingBefore := append([]byte(nil), memory[14000:14000+len(payload)]...)
			trailing, err := instance.Invoke("decompress", 12000, compressedLen+1, 14000, uint64(len(payload)))
			if err != nil || Status(wago.AsI32(trailing[0])) != StatusTrailingData || trailing[1] != 0 {
				t.Fatalf("trailing-data decompress = %v, %v", trailing, err)
			}
			if !bytes.Equal(memory[14000:14000+len(payload)], trailingBefore) {
				t.Fatal("trailing-data failure changed destination")
			}
			if test.memory64 {
				wideLength, err := instance.Invoke("compress", 0, uint64(1)<<32|1, 16000, 32, wago.I32(flate.DefaultCompression))
				if err != nil || Status(wago.AsI32(wideLength[0])) != StatusInputTooLarge {
					t.Fatalf("64-bit length narrowed = %v, %v", wideLength, err)
				}
				widePointer, err := instance.Invoke("compress", uint64(1)<<32, 1, 16000, 32, wago.I32(flate.DefaultCompression))
				if err != nil || Status(wago.AsI32(widePointer[0])) != StatusInvalidArgument {
					t.Fatalf("64-bit pointer narrowed = %v, %v", widePointer, err)
				}
			}

			before := append([]byte(nil), memory[5000:5016]...)
			bad, err := instance.Invoke("decompress", uint64(len(memory)-1), 8, 5000, 16)
			if err != nil || Status(wago.AsI32(bad[0])) != StatusInvalidArgument || bad[1] != 0 {
				t.Fatalf("out-of-bounds decompress = %v, %v", bad, err)
			}
			if !bytes.Equal(memory[5000:5016], before) {
				t.Fatal("failed guest call changed destination")
			}
		})
	}
}

func TestWagoConfiguredLimits(t *testing.T) {
	runtime := newTestRuntime(t, json.RawMessage(`{"maxInputBytes":4,"maxOutputBytes":8,"maxConcurrentOperations":1}`))
	compiled, err := runtime.Compile(linearMemoryGuestModule(ModuleWasm32, false))
	if err != nil {
		t.Fatal(err)
	}
	instance, err := runtime.Instantiate(context.Background(), compiled)
	if err != nil {
		t.Fatal(err)
	}
	defer instance.Close()
	copy(instance.Memory().UnsafeBytes(), "12345")
	if result, err := instance.Invoke("compress", 0, 5, 64, 8, wago.I32(flate.DefaultCompression)); err != nil || Status(wago.AsI32(result[0])) != StatusInputTooLarge {
		t.Fatalf("input limit = %v, %v", result, err)
	}
	if result, err := instance.Invoke("compress", 0, 4, 64, 9, wago.I32(flate.DefaultCompression)); err != nil || Status(wago.AsI32(result[0])) != StatusOutputTooLarge {
		t.Fatalf("output limit = %v, %v", result, err)
	}
}

func TestWagoBusyStatus(t *testing.T) {
	plugin := &Plugin{}
	provider := Provider()
	provider.New = func() wago.Plugin { return plugin }
	runtimeConfig := wago.NewRuntimeConfig().WithCoreFeatures(wago.CoreFeaturesV3)
	runtime := wago.NewRuntime(wago.WithRuntimeConfig(runtimeConfig))
	if err := runtime.LoadPlugins(context.Background(), selectedProviderSet(t, provider, json.RawMessage(`{"maxConcurrentOperations":1}`))); err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	compiled, err := runtime.Compile(linearMemoryGuestModule(ModuleWasm32, false))
	if err != nil {
		t.Fatal(err)
	}
	instance, err := runtime.Instantiate(context.Background(), compiled)
	if err != nil {
		t.Fatal(err)
	}
	defer instance.Close()
	if !plugin.acquire() {
		t.Fatal("failed to reserve the only operation slot")
	}
	defer plugin.release()
	result, err := instance.Invoke("compress", 0, 0, 64, 64, wago.I32(flate.DefaultCompression))
	if err != nil || Status(wago.AsI32(result[0])) != StatusBusy || result[1] != 0 {
		t.Fatalf("busy result = %v, %v", result, err)
	}
}

func TestWagoGCByteArrayEndToEnd(t *testing.T) {
	runtime := newTestRuntime(t, nil)
	compiled, err := runtime.Compile(gcGuestModule())
	if err != nil {
		t.Fatal(err)
	}
	defer compiled.Close()
	instance, err := runtime.Instantiate(context.Background(), compiled)
	if err != nil {
		t.Fatal(err)
	}
	defer instance.Close()
	version, err := instance.Invoke("abi_version")
	if err != nil || len(version) != 1 || wago.AsI32(version[0]) != ABIVersion {
		t.Fatalf("GC abi_version = %v, %v", version, err)
	}
	result, err := instance.Invoke("run")
	if err != nil || len(result) != 1 || wago.AsI32(result[0]) != 1 {
		t.Fatalf("GC round trip = %v, %v", result, err)
	}
	result, err = instance.Invoke("overlap")
	if err != nil || len(result) != 1 || wago.AsI32(result[0]) != 1 {
		t.Fatalf("GC overlap round trip = %v, %v", result, err)
	}
	result, err = instance.Invoke("bounds")
	if err != nil || len(result) != 2 || Status(wago.AsI32(result[0])) != StatusInvalidArgument || wago.AsI32(result[1]) != 1 {
		t.Fatalf("GC bounds result = %v, %v", result, err)
	}
	result, err = instance.Invoke("malformed")
	if err != nil || len(result) != 2 || Status(wago.AsI32(result[0])) != StatusInvalidData || wago.AsI32(result[1]) != 1 {
		t.Fatalf("GC malformed result = %v, %v", result, err)
	}
}

func TestWagoGCImmutableInputSuccess(t *testing.T) {
	runtime := newTestRuntime(t, nil)
	compiled, err := runtime.Compile(gcImmutableInputModule(zlibStream(t, []byte{1, 2, 3})))
	if err != nil {
		t.Fatal(err)
	}
	defer compiled.Close()
	instance, err := runtime.Instantiate(context.Background(), compiled)
	if err != nil {
		t.Fatal(err)
	}
	defer instance.Close()
	result, err := instance.Invoke("run")
	if err != nil || len(result) != 1 || wago.AsI32(result[0]) != 1 {
		t.Fatalf("immutable GC input = %v, %v", result, err)
	}
}

func TestWagoGCRejectsWrongAndImmutableArrays(t *testing.T) {
	runtime := newTestRuntime(t, nil)
	for _, test := range []struct {
		name       string
		storage    byte
		mutability byte
	}{
		{"i32-array", 0x7f, 0x01},
		{"immutable-i8-output", 0x78, 0x00},
	} {
		t.Run(test.name, func(t *testing.T) {
			compiled, err := runtime.Compile(gcRejectedArrayModule(test.storage, test.mutability))
			if err != nil {
				t.Fatal(err)
			}
			defer compiled.Close()
			instance, err := runtime.Instantiate(context.Background(), compiled)
			if err != nil {
				t.Fatal(err)
			}
			defer instance.Close()
			result, err := instance.Invoke("run")
			if err != nil || len(result) != 1 || Status(wago.AsI32(result[0])) != StatusInvalidArgument {
				t.Fatalf("rejected array = %v, %v", result, err)
			}
		})
	}
}

func TestWagoGCBoundsImmutableInputCopy(t *testing.T) {
	runtime := newTestRuntime(t, json.RawMessage(`{"maxInputBytes":4}`))
	compiled, err := runtime.Compile(gcOversizedImmutableInputModule())
	if err != nil {
		t.Fatal(err)
	}
	defer compiled.Close()
	instance, err := runtime.Instantiate(context.Background(), compiled)
	if err != nil {
		t.Fatal(err)
	}
	defer instance.Close()
	result, err := instance.Invoke("run")
	if err != nil || len(result) != 1 || Status(wago.AsI32(result[0])) != StatusInputTooLarge {
		t.Fatalf("oversized immutable GC input = %v, %v", result, err)
	}
}

func benchmarkWagoInstance(b *testing.B) *wago.Instance {
	b.Helper()
	runtime := newTestRuntime(b, nil)
	compiled, err := runtime.Compile(linearMemoryGuestModule(ModuleWasm32, false))
	if err != nil {
		b.Fatal(err)
	}
	instance, err := runtime.Instantiate(context.Background(), compiled)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() {
		_ = instance.Close()
		_ = compiled.Close()
	})
	return instance
}

func BenchmarkWagoCompress1KiB(b *testing.B) {
	instance := benchmarkWagoInstance(b)
	src := bytes.Repeat([]byte("abcdefgh"), 128)
	copy(instance.Memory().UnsafeBytes(), src)
	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result, err := instance.Invoke("compress", 0, uint64(len(src)), 2048, 2048, wago.I32(flate.DefaultCompression))
		if err != nil || Status(wago.AsI32(result[0])) != StatusOK {
			b.Fatalf("compress = %v, %v", result, err)
		}
	}
}

func BenchmarkWagoDecompress1KiB(b *testing.B) {
	instance := benchmarkWagoInstance(b)
	payload := bytes.Repeat([]byte("abcdefgh"), 128)
	stream := zlibStream(b, payload)
	copy(instance.Memory().UnsafeBytes(), stream)
	b.ReportAllocs()
	b.SetBytes(int64(len(payload)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result, err := instance.Invoke("decompress", 0, uint64(len(stream)), 2048, uint64(len(payload)))
		if err != nil || Status(wago.AsI32(result[0])) != StatusOK {
			b.Fatalf("decompress = %v, %v", result, err)
		}
	}
}
