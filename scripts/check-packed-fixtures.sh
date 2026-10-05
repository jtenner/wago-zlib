#!/usr/bin/env bash
set -euo pipefail

wasm_tools_cmd="${WASM_TOOLS:-wasm-tools}"
tmp_dir="$(mktemp -d "${TMPDIR:-/tmp}/wago-zlib-wat.XXXXXX")"
trap 'rm -r -- "$tmp_dir"' EXIT

for abi in wasm32 wasm64 gc; do
  "$wasm_tools_cmd" parse "testdata/packed-$abi.wat" -o "$tmp_dir/packed-$abi.wasm"
  if ! cmp -s "testdata/packed-$abi.wasm" "$tmp_dir/packed-$abi.wasm"; then
    echo "testdata/packed-$abi.wasm is stale; regenerate it with wasm-tools 1.251.0 parse" >&2
    exit 1
  fi
done
