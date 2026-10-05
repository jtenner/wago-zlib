package wagozlib

import (
	_ "embed"
	"encoding/json"

	wago "github.com/wago-org/wago"
)

const (
	ID         = "github.com/jtenner/wago-zlib"
	ABIVersion = 1

	Capability = wago.Capability("compression.zlib")

	ModuleWasm32 = "wago_zlib.wasm32"
	ModuleWasm64 = "wago_zlib.wasm64"
	ModuleGC     = "wago_zlib.gc"
)

//go:embed config.schema.json
var configSchema json.RawMessage

// Definition returns fresh immutable metadata for the zlib provider.
func Definition() wago.PluginDefinition {
	return wago.PluginDefinition{
		ID:          ID,
		Name:        "Zlib",
		Version:     "0.0.1",
		Description: "Bounded zlib-wrapped DEFLATE compression and decompression.",
		Stability:   wago.Experimental,
		Compatibility: wago.Compatibility{
			Engines: map[string]string{"wago": ">=0.1.0-beta.11", "go": ">=1.22"},
		},
		Provenance: wago.PluginProvenance{
			Homepage:   "https://github.com/jtenner/wago-zlib",
			Repository: "https://github.com/jtenner/wago-zlib",
			License:    "Apache-2.0",
			Authors:    []string{"Josh Tenner"},
		},
		Authorities: []wago.AuthorityRequest{{
			Name:   wago.AuthorityHostImportDefine,
			Mode:   wago.AuthorityRequired,
			Reason: "define the bounded wago_zlib guest APIs",
			Scope: wago.AuthorityScope{Modules: []string{
				ModuleWasm32,
				ModuleWasm64,
				ModuleGC,
			}},
		}},
		ConfigSchema: append(json.RawMessage(nil), configSchema...),
	}
}

// Provider returns a side-effect-free catalog entry. Each Wago runtime gets a
// separate Plugin and therefore a separate concurrency budget.
func Provider() wago.PluginProvider {
	return wago.PluginProvider{
		Definition:     Definition(),
		New:            func() wago.Plugin { return &Plugin{} },
		ValidateConfig: validateConfig,
	}
}

// Plugin owns only a fixed-capacity operation semaphore and immutable limits.
// It retains no guest pointers or per-instance resources. Wago revokes its host
// imports during normal runtime teardown, so no additional lifecycle hook is
// required.
type Plugin struct {
	cfg   Config
	slots chan struct{}
}

func (p *Plugin) Register(reg *wago.Registrar) error {
	var configured Config
	if err := reg.Config(&configured); err != nil {
		return err
	}
	resolved, err := configured.resolved()
	if err != nil {
		return err
	}
	p.cfg = resolved
	p.slots = make(chan struct{}, resolved.MaxConcurrentOperations)

	if err := reg.GuestCapability(Capability, wago.CapabilityDocs("compress and decompress RFC 1950 zlib streams")); err != nil {
		return err
	}
	imports, err := reg.HostImports()
	if err != nil {
		return err
	}
	for _, module := range []string{ModuleWasm32, ModuleWasm64, ModuleGC} {
		imports.HostFunc(module, "abi_version", func(call wago.HostCall) {
			call.SetI32(0, ABIVersion)
		}).Results(wago.ValI32).Docs("return the wago_zlib ABI version")
	}
	imports.HostFunc(ModuleWasm32, "compress", p.compress32).
		Params(wago.ValI32, wago.ValI32, wago.ValI32, wago.ValI32, wago.ValI32).
		Results(wago.ValI32, wago.ValI32).
		Capability(Capability).
		Docs("compress one first-memory memory32 slice into one zlib stream")
	imports.HostFunc(ModuleWasm32, "decompress", p.decompress32).
		Params(wago.ValI32, wago.ValI32, wago.ValI32, wago.ValI32).
		Results(wago.ValI32, wago.ValI32).
		Capability(Capability).
		Docs("decompress exactly one checksum-verified zlib stream in first-memory memory32")
	imports.HostFunc(ModuleWasm32, "compress_packed", p.compressPacked32).
		Params(wago.ValI32, wago.ValI32, wago.ValI32, wago.ValI32, wago.ValI32).
		Results(wago.ValI64).
		Capability(Capability).
		Docs("compress one first-memory memory32 slice and return status:written packed into one i64")
	imports.HostFunc(ModuleWasm32, "decompress_packed", p.decompressPacked32).
		Params(wago.ValI32, wago.ValI32, wago.ValI32, wago.ValI32).
		Results(wago.ValI64).
		Capability(Capability).
		Docs("decompress exactly one checksum-verified zlib stream and return status:written packed into one i64")
	imports.HostFunc(ModuleWasm64, "compress", p.compress64).
		Params(wago.ValI64, wago.ValI64, wago.ValI64, wago.ValI64, wago.ValI32).
		Results(wago.ValI32, wago.ValI64).
		Capability(Capability).
		Docs("compress one first-memory memory64 slice into one zlib stream")
	imports.HostFunc(ModuleWasm64, "decompress", p.decompress64).
		Params(wago.ValI64, wago.ValI64, wago.ValI64, wago.ValI64).
		Results(wago.ValI32, wago.ValI64).
		Capability(Capability).
		Docs("decompress exactly one checksum-verified zlib stream in first-memory memory64")
	imports.HostFunc(ModuleWasm64, "compress_packed", p.compressPacked64).
		Params(wago.ValI64, wago.ValI64, wago.ValI64, wago.ValI64, wago.ValI32).
		Results(wago.ValI64).
		Capability(Capability).
		Docs("compress one first-memory memory64 slice and return status:written packed into one i64")
	imports.HostFunc(ModuleWasm64, "decompress_packed", p.decompressPacked64).
		Params(wago.ValI64, wago.ValI64, wago.ValI64, wago.ValI64).
		Results(wago.ValI64).
		Capability(Capability).
		Docs("decompress exactly one checksum-verified zlib stream and return status:written packed into one i64")
	imports.HostFunc(ModuleGC, "compress", p.compressGC).
		Params(wago.ValAnyRef, wago.ValI32, wago.ValI32, wago.ValAnyRef, wago.ValI32, wago.ValI32, wago.ValI32).
		Results(wago.ValI32, wago.ValI32).
		Capability(Capability).
		Docs("compress one byte-array slice into one zlib stream")
	imports.HostFunc(ModuleGC, "decompress", p.decompressGC).
		Params(wago.ValAnyRef, wago.ValI32, wago.ValI32, wago.ValAnyRef, wago.ValI32, wago.ValI32).
		Results(wago.ValI32, wago.ValI32).
		Capability(Capability).
		Docs("decompress exactly one checksum-verified zlib stream between byte arrays")
	imports.HostFunc(ModuleGC, "compress_packed", p.compressPackedGC).
		Params(wago.ValAnyRef, wago.ValI32, wago.ValI32, wago.ValAnyRef, wago.ValI32, wago.ValI32, wago.ValI32).
		Results(wago.ValI64).
		Capability(Capability).
		Docs("compress one byte-array slice and return status:written packed into one i64")
	imports.HostFunc(ModuleGC, "decompress_packed", p.decompressPackedGC).
		Params(wago.ValAnyRef, wago.ValI32, wago.ValI32, wago.ValAnyRef, wago.ValI32, wago.ValI32).
		Results(wago.ValI64).
		Capability(Capability).
		Docs("decompress exactly one checksum-verified zlib stream and return status:written packed into one i64")
	return nil
}

func (p *Plugin) compress32(caller wago.Caller, call wago.HostCall) {
	p.transformMemory(caller, call, true, false, false)
}

func (p *Plugin) decompress32(caller wago.Caller, call wago.HostCall) {
	p.transformMemory(caller, call, false, false, false)
}

func (p *Plugin) compressPacked32(caller wago.Caller, call wago.HostCall) {
	p.transformMemory(caller, call, true, false, true)
}

func (p *Plugin) decompressPacked32(caller wago.Caller, call wago.HostCall) {
	p.transformMemory(caller, call, false, false, true)
}

func (p *Plugin) compress64(caller wago.Caller, call wago.HostCall) {
	p.transformMemory(caller, call, true, true, false)
}

func (p *Plugin) decompress64(caller wago.Caller, call wago.HostCall) {
	p.transformMemory(caller, call, false, true, false)
}

func (p *Plugin) compressPacked64(caller wago.Caller, call wago.HostCall) {
	p.transformMemory(caller, call, true, true, true)
}

func (p *Plugin) decompressPacked64(caller wago.Caller, call wago.HostCall) {
	p.transformMemory(caller, call, false, true, true)
}

func (p *Plugin) compressGC(caller wago.Caller, call wago.HostCall) {
	p.transformGC(caller, call, true, false)
}

func (p *Plugin) decompressGC(caller wago.Caller, call wago.HostCall) {
	p.transformGC(caller, call, false, false)
}

func (p *Plugin) compressPackedGC(caller wago.Caller, call wago.HostCall) {
	p.transformGC(caller, call, true, true)
}

func (p *Plugin) decompressPackedGC(caller wago.Caller, call wago.HostCall) {
	p.transformGC(caller, call, false, true)
}

func packedResult(status Status, written uint64) uint64 {
	if status != StatusOK {
		written = 0
	}
	return uint64(uint32(status)) | uint64(uint32(written))<<32
}

func setMemoryResult(call wago.HostCall, memory64, packed bool, status Status, written uint64) {
	if packed {
		call.SetI64(0, int64(packedResult(status, written)))
		return
	}
	call.SetI32(0, int32(status))
	if memory64 {
		call.SetI64(1, int64(written))
	} else {
		call.SetI32(1, int32(uint32(written)))
	}
}

func (p *Plugin) transformMemory(caller wago.Caller, call wago.HostCall, compress, memory64, packed bool) {
	setMemoryResult(call, memory64, packed, StatusInternalError, 0)

	var srcPtr, srcLen, dstPtr, dstLen uint64
	if memory64 {
		srcPtr = uint64(call.I64(0))
		srcLen = uint64(call.I64(1))
		dstPtr = uint64(call.I64(2))
		dstLen = uint64(call.I64(3))
	} else {
		srcPtr = uint64(uint32(call.I32(0)))
		srcLen = uint64(uint32(call.I32(1)))
		dstPtr = uint64(uint32(call.I32(2)))
		dstLen = uint64(uint32(call.I32(3)))
	}
	if srcLen > uint64(p.cfg.MaxInputBytes) {
		setMemoryResult(call, memory64, packed, StatusInputTooLarge, 0)
		return
	}
	if dstLen > uint64(p.cfg.MaxOutputBytes) {
		setMemoryResult(call, memory64, packed, StatusOutputTooLarge, 0)
		return
	}
	if !p.acquire() {
		setMemoryResult(call, memory64, packed, StatusBusy, 0)
		return
	}
	defer p.release()

	host, ok := any(caller).(wago.GuestStorageHostModule)
	if !ok {
		setMemoryResult(call, memory64, packed, StatusUnsupported, 0)
		return
	}
	status := StatusUnsupported
	written := uint64(0)
	err := host.WithGuestStorage(func(storage wago.GuestStorage) error {
		info, infoErr := storage.MemoryInfo(0)
		wantAddressType := wago.GuestMemory32
		if memory64 {
			wantAddressType = wago.GuestMemory64
		}
		if infoErr != nil || info.AddressType != wantAddressType {
			status = StatusUnsupported
			return nil
		}
		src, rangeErr := storage.MemoryRange(0, srcPtr, srcLen, wago.GuestStorageRead)
		if rangeErr != nil {
			status = StatusInvalidArgument
			return nil
		}
		dst, rangeErr := storage.MemoryRange(0, dstPtr, dstLen, wago.GuestStorageWrite)
		if rangeErr != nil {
			status = StatusInvalidArgument
			return nil
		}
		var n int
		if compress {
			n, status = Compress(dst, src, int(call.I32(4)))
		} else {
			n, status = Decompress(dst, src)
		}
		written = uint64(n)
		return nil
	})
	if err != nil {
		status = StatusUnsupported
		written = 0
	}
	setMemoryResult(call, memory64, packed, status, written)
}

func setGCResult(call wago.HostCall, packed bool, status Status, written int) {
	if packed {
		call.SetI64(0, int64(packedResult(status, uint64(written))))
		return
	}
	call.SetI32(0, int32(status))
	call.SetI32(1, int32(uint32(written)))
}

func (p *Plugin) transformGC(caller wago.Caller, call wago.HostCall, compress, packed bool) {
	setGCResult(call, packed, StatusInternalError, 0)

	srcOffset := uint64(uint32(call.I32(1)))
	srcLen := uint64(uint32(call.I32(2)))
	dstOffset := uint64(uint32(call.I32(4)))
	dstLen := uint64(uint32(call.I32(5)))
	if srcLen > uint64(p.cfg.MaxInputBytes) {
		setGCResult(call, packed, StatusInputTooLarge, 0)
		return
	}
	if dstLen > uint64(p.cfg.MaxOutputBytes) {
		setGCResult(call, packed, StatusOutputTooLarge, 0)
		return
	}
	if !p.acquire() {
		setGCResult(call, packed, StatusBusy, 0)
		return
	}
	defer p.release()

	host, ok := any(caller).(wago.GuestStorageHostModule)
	if !ok {
		setGCResult(call, packed, StatusUnsupported, 0)
		return
	}
	status := StatusUnsupported
	written := 0
	err := host.WithGuestStorage(func(storage wago.GuestStorage) error {
		srcToken, _ := call.RawParam(0)
		dstToken, _ := call.RawParam(3)
		srcRef, refErr := storage.GCRef(srcToken)
		if refErr != nil {
			status = StatusInvalidArgument
			return nil
		}
		dstRef, refErr := storage.GCRef(dstToken)
		if refErr != nil {
			status = StatusInvalidArgument
			return nil
		}
		srcInfo, arrayErr := storage.GCArrayInfo(srcRef)
		if arrayErr != nil || srcInfo.Storage != wago.GuestGCArrayI8 {
			status = StatusInvalidArgument
			return nil
		}
		dstInfo, arrayErr := storage.GCArrayInfo(dstRef)
		if arrayErr != nil || dstInfo.Storage != wago.GuestGCArrayI8 || !dstInfo.Mutable {
			status = StatusInvalidArgument
			return nil
		}
		if srcOffset > uint64(srcInfo.Length) || srcLen > uint64(srcInfo.Length)-srcOffset ||
			dstOffset > uint64(dstInfo.Length) || dstLen > uint64(dstInfo.Length)-dstOffset {
			status = StatusInvalidArgument
			return nil
		}
		// Wago must detach immutable arrays to enforce read-only access. Bound
		// that copy by the same input limit even when the selected slice is
		// smaller than its containing array.
		if !srcInfo.Mutable && uint64(srcInfo.Length) > uint64(p.cfg.MaxInputBytes) {
			status = StatusInputTooLarge
			return nil
		}
		srcArray, _, arrayErr := storage.GCArrayBytes(srcRef, wago.GuestStorageRead)
		if arrayErr != nil {
			status = StatusInvalidArgument
			return nil
		}
		dstArray, _, arrayErr := storage.GCArrayBytes(dstRef, wago.GuestStorageWrite)
		if arrayErr != nil {
			status = StatusInvalidArgument
			return nil
		}
		src := srcArray[srcOffset : srcOffset+srcLen]
		dst := dstArray[dstOffset : dstOffset+dstLen]
		if compress {
			written, status = Compress(dst, src, int(call.I32(6)))
		} else {
			written, status = Decompress(dst, src)
		}
		return nil
	})
	if err != nil {
		status = StatusUnsupported
		written = 0
	}
	setGCResult(call, packed, status, written)
}

func (p *Plugin) acquire() bool {
	select {
	case p.slots <- struct{}{}:
		return true
	default:
		return false
	}
}

func (p *Plugin) release() {
	<-p.slots
}
