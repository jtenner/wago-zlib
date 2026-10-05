# `wago_zlib` ABI v1

## Scope

ABI v1 performs one-shot RFC 1950 zlib operations. It does not auto-detect or
accept RFC 1951 raw DEFLATE, RFC 1952 gzip, concatenated zlib members, preset
dictionaries, or unrelated codecs.

Every pointer, offset, length, and capacity is unsigned at the ABI boundary.
The implementation validates complete ranges before processing and never retains
a borrowed guest view after the host callback.

## Imports

Every module exports:

```text
abi_version() -> i32                         // always 1
```

`wago_zlib.wasm32` uses first-memory memory32:

```text
compress(src_ptr:i32, src_len:i32,
         dst_ptr:i32, dst_cap:i32,
         level:i32) -> (status:i32, written:i32)

decompress(src_ptr:i32, src_len:i32,
           dst_ptr:i32, dst_cap:i32)
        -> (status:i32, written:i32)
```

`wago_zlib.wasm64` uses first-memory memory64:

```text
compress(src_ptr:i64, src_len:i64,
         dst_ptr:i64, dst_cap:i64,
         level:i32) -> (status:i32, written:i64)

decompress(src_ptr:i64, src_len:i64,
           dst_ptr:i64, dst_cap:i64)
        -> (status:i32, written:i64)
```

`wago_zlib.gc` uses caller-defined packed byte arrays. `src` may be mutable or
immutable; `dst` must be mutable:

```text
compress(src:(ref array<i8>), src_offset:i32, src_len:i32,
         dst:(ref (mut array<i8>)), dst_offset:i32, dst_cap:i32,
         level:i32) -> (status:i32, written:i32)

decompress(src:(ref array<i8>), src_offset:i32, src_len:i32,
           dst:(ref (mut array<i8>)), dst_offset:i32, dst_cap:i32)
        -> (status:i32, written:i32)
```

Go zlib levels are accepted: `-2` (`HuffmanOnly`), `-1` (default), and `0`
through `9`. Other values return `INVALID_ARGUMENT`.

## Status values

| Value | Name | Meaning |
|---:|---|---|
| 0 | `OK` | Complete output committed; `written` is its length. |
| 1 | `INVALID_ARGUMENT` | Invalid level, reference, guest range, or array shape/mutability. |
| 2 | `INPUT_TOO_LARGE` | `src_len` exceeds the configured input limit. |
| 3 | `OUTPUT_TOO_LARGE` | `dst_cap` exceeds the configured output limit. |
| 4 | `OUTPUT_TOO_SMALL` | Valid completion could not be established within `dst_cap`. |
| 5 | `INVALID_DATA` | Not a valid zlib stream, including raw DEFLATE and gzip. |
| 6 | `TRUNCATED` | Input ended before a complete zlib stream and checksum. |
| 7 | `CHECKSUM_MISMATCH` | The zlib Adler-32 trailer did not match decoded content. |
| 8 | `DICTIONARY_REQUIRED` | The stream requests a preset dictionary; v1 has no dictionary API. |
| 9 | `TRAILING_DATA` | Bytes remain after the first complete zlib stream. |
| 10 | `BUSY` | The runtime's concurrent-operation slots are occupied. |
| 11 | `UNSUPPORTED` | The caller does not provide the selected storage ABI. |
| 12 | `INTERNAL_ERROR` | An unexpected host/codec invariant failed. |

Length-limit checks happen before storage access and slot acquisition. The
operation then acquires a slot, validates guest storage, and runs the codec. If
the decompressed output exceeds `dst_cap`, `OUTPUT_TOO_SMALL` takes precedence
because the implementation deliberately stops before consuming potentially
unbounded additional output; checksum or trailing-data status is available once
the guest supplies enough bounded destination capacity to reach stream EOF.

## Completion and buffers

`OK` requires the decoder to read through the checksum-verified end of the zlib
stream and confirm that no source bytes remain. A zero-byte payload is valid when
encoded as a complete zlib stream; a zero-byte compressed input is `TRUNCATED`.

Output is transactional. On every non-`OK` result:

- `written` is zero;
- the entire destination range is unchanged;
- no output prefix is externally committed.

The implementation stages output in a fixed-capacity host buffer, so source and
destination may overlap in any way. Staging is bounded by `dst_cap`, which itself
must not exceed the configured `maxOutputBytes`.

Wago exposes immutable GC arrays to hosts as detached copies. To keep that copy
finite, ABI v1 accepts an immutable input array only when the array's total byte
length is no greater than `maxInputBytes`; otherwise it returns
`INPUT_TOO_LARGE`, even if the selected slice is smaller. Mutable input arrays
are borrowed zero-copy and remain governed by the selected `src_len` limit.
