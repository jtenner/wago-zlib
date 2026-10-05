#!/usr/bin/env bash
set -euo pipefail

tinygo_cmd="${TINYGO:-tinygo}"
jobs="${TINYGO_JOBS:-2}"

"$tinygo_cmd" version | grep -F "tinygo version 0.42.0 "
go version | grep -F "go1.27.1 "
"$tinygo_cmd" info wasm-unknown | grep -E '^LLVM triple:[[:space:]]+wasm32-'

if "$tinygo_cmd" targets | grep -Eq '^(wasm64|wasmgc)$'; then
	echo "TinyGo gained a Wasm64 or WasmGC target; add generated-guest coverage for it." >&2
	exit 1
fi

"$tinygo_cmd" build \
	-target=wasm-unknown -scheduler=none -gc=leaking -no-debug -p="$jobs" \
	-o testdata/tinygo-wasm32-packed.generated.wasm \
	./testdata/tinygo-wasm32-packed

echo "TinyGo guest scope: generated Wasm32 executes the packed codec ABI; Wago's Wasm64 and Wasm GC packed APIs are covered by WAT because TinyGo 0.42.0 has no corresponding output targets."
