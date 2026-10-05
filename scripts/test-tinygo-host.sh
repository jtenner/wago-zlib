#!/usr/bin/env bash
set -euo pipefail

tinygo_cmd="${TINYGO:-tinygo}"
jobs="${TINYGO_JOBS:-4}"

go_version="$(go version)"
if [[ "$go_version" != "go version go1.22.12 linux/amd64" ]]; then
  echo "TinyGo host qualification requires go1.22.12 linux/amd64; got: $go_version" >&2
  exit 1
fi

tinygo_version="$($tinygo_cmd version)"
case "$tinygo_version" in
  "tinygo version 0.41.1 linux/amd64 (using go version go1.22.12 and LLVM version "*")") ;;
  *)
    echo "TinyGo host qualification requires TinyGo 0.41.1 linux/amd64 with Go 1.22.12; got: $tinygo_version" >&2
    exit 1
    ;;
esac

export GOMAXPROCS="${GOMAXPROCS:-$jobs}"

log_dir="$(mktemp -d "${TMPDIR:-/tmp}/wago-zlib-tinygo.XXXXXX")"
trap 'rm -r -- "$log_dir"' EXIT

require_passes() {
  local log="$1"
  shift
  local test_name
  for test_name in "$@"; do
    if ! grep -Fq -- "--- PASS: $test_name " "$log"; then
      echo "required TinyGo test did not execute successfully: $test_name" >&2
      exit 1
    fi
  done
}

echo "==> TinyGo codec and error-policy execution"
"$tinygo_cmd" test -p="$jobs" -scheduler=tasks -count=1 -v \
  -run '^(TestRoundTripLevels|TestEmptyPayloadAndEmptyInputAreDistinct|TestCompressionRejectsInvalidLevelTransactionally|TestOutputOverflowIsTransactional|TestExactFitForcesChecksumCompletion|TestMalformedInputStatusesAndNoCommit|TestEveryProperStreamPrefixIsTruncated|TestPresetDictionaryRejected|TestOverlappingBuffers|TestStatusStringsCoverABI)$' \
  . | tee "$log_dir/codec.log"
require_passes "$log_dir/codec.log" \
  TestRoundTripLevels \
  TestEmptyPayloadAndEmptyInputAreDistinct \
  TestCompressionRejectsInvalidLevelTransactionally \
  TestOutputOverflowIsTransactional \
  TestExactFitForcesChecksumCompletion \
  TestMalformedInputStatusesAndNoCommit \
  TestEveryProperStreamPrefixIsTruncated \
  TestPresetDictionaryRejected \
  TestOverlappingBuffers \
  TestStatusStringsCoverABI

echo "==> TinyGo-hosted Wago ABI execution (release compiler settings)"
"$tinygo_cmd" test -p="$jobs" -scheduler=tasks -no-debug -opt=z -gc=conservative -count=1 -v \
  -run '^(TestDefinitionAndConfig|TestOperationSlotsAreNonBlockingAndPerPlugin|TestWagoLinearMemoryEndToEnd|TestWagoConfiguredLimits|TestWagoBusyStatus|TestWagoGCByteArrayEndToEnd|TestWagoGCImmutableInputSuccess|TestWagoGCRejectsWrongAndImmutableArrays|TestWagoGCBoundsImmutableInputCopy)$' \
  . | tee "$log_dir/abi.log"
require_passes "$log_dir/abi.log" \
  TestDefinitionAndConfig \
  TestOperationSlotsAreNonBlockingAndPerPlugin \
  TestWagoLinearMemoryEndToEnd \
  TestWagoLinearMemoryEndToEnd/wasm32 \
  TestWagoLinearMemoryEndToEnd/wasm64 \
  TestWagoConfiguredLimits \
  TestWagoBusyStatus \
  TestWagoGCByteArrayEndToEnd \
  TestWagoGCImmutableInputSuccess \
  TestWagoGCRejectsWrongAndImmutableArrays \
  TestWagoGCRejectsWrongAndImmutableArrays/i32-array \
  TestWagoGCRejectsWrongAndImmutableArrays/immutable-i8-output \
  TestWagoGCBoundsImmutableInputCopy

echo "==> TinyGo provider registration execution"
"$tinygo_cmd" test -p="$jobs" -scheduler=tasks -count=1 -v ./register | tee "$log_dir/register.log"
require_passes "$log_dir/register.log" TestProviderCatalog
