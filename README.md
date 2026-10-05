# Wago Zlib

`wago-zlib` is a capability-gated [Wago](https://github.com/wago-org/wago)
plugin for bounded RFC 1950 zlib compression and decompression. It delegates the
compression algorithm and framing to Go's maintained standard-library
`compress/zlib` package; this repository does not implement a new codec.

The first release is deliberately narrow:

- zlib-wrapped DEFLATE only—never raw DEFLATE or gzip;
- exactly one complete stream per decompression call;
- verified Adler-32 completion is required;
- trailing bytes and preset dictionaries are rejected;
- output is staged and committed only on success;
- input/output bytes, staging memory, and concurrent calls have finite limits.

## Wago provider

The package exports a side-effect-free catalog entry:

```go
provider := wagozlib.Provider()
```

Hosts select the provider through the normal Wago `PluginSet` lock graph and
grant its exact `host.import.define` scope. The plugin exposes guest capability
`compression.zlib` and these import modules:

| Module | Storage | Offset/length width |
|---|---|---|
| `wago_zlib.wasm32` | first linear memory, memory32 only | `i32` |
| `wago_zlib.wasm64` | first linear memory, memory64 only | `i64` |
| `wago_zlib.gc` | mutable/immutable `array<i8>` input; mutable `array<i8>` output | `i32` array offsets |

Linear-memory modules intentionally address memory index 0. Multi-memory guests
can use them when their selected first memory has the corresponding address
width. The GC surface accepts caller-defined packed byte-array types and rejects
null references, non-byte arrays, and immutable output arrays.

See [the ABI v1 specification](docs/abi-v1.md) for exact signatures and status
behavior.

## Configuration

Configuration is strict JSON in the reviewed Wago plugin selection:

```json
{
  "maxInputBytes": 8388608,
  "maxOutputBytes": 33554432,
  "maxConcurrentOperations": 2
}
```

Omitted fields use those finite defaults. Hard maxima are 64 MiB input, 64 MiB
output, and eight concurrent operations. A call reserves one non-blocking slot;
when all slots are in use it returns `BUSY` instead of creating another worker
or waiting indefinitely.

Each operation allocates a bounded transaction buffer no larger than the guest's
declared destination capacity, plus the bounded internal state owned by Go's
zlib implementation. At the defaults, transaction staging is therefore bounded
by 64 MiB per runtime across two simultaneous calls. Go runtime and codec
bookkeeping are additional; this is not a zero-allocation claim.

## Native Go API

The same strict one-shot behavior is available without Wago:

```go
n, status := wagozlib.Compress(dst, src, flate.DefaultCompression)
if status != wagozlib.StatusOK {
    // dst is unchanged and n == 0.
}

n, status = wagozlib.Decompress(dst, compressed)
```

The Go API is bounded by the caller-provided slices. The Wago layer adds the
configured input, output, and concurrency ceilings.

## Failure and lifecycle contract

Only `StatusOK` commits bytes to the destination; `written` is the complete
output length and is zero for successful empty decompression. Every failure
also returns `written == 0`, but leaves the destination byte-for-byte unchanged.
This staging makes partially or completely overlapping input and output ranges
safe.

The plugin retains no guest slice, pointer, array reference, or `HostCall` after
the callback. It owns no goroutines, files, handles, or background workers.
State consists only of immutable limits and a fixed-capacity operation semaphore
per Wago runtime. Normal Wago runtime teardown revokes the registered imports;
there is no additional plugin resource lifecycle.

## Development

```sh
go test ./...
go test -race ./...
go vet ./...
go test -run '^$' -bench 'Benchmark(Compress|Decompress)1KiB' -benchmem
```

Benchmarks are smoke measurements for regressions, not claims that this wrapper
is allocation-free or the fastest zlib implementation.

## License

Apache-2.0. Go standard-library attribution is recorded in
[THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).
