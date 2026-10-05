//go:build tinygo_guest_qualification

package wagozlib

import (
	"context"
	"os"
	"testing"
)

// TestTinyGoProducedPackedWasm32Guest executes real codec operations from a
// Wasm32 guest compiled by the pinned TinyGo toolchain. It is run once with an
// ordinary Go Wago host and again with a TinyGo-built Wago host.
func TestTinyGoProducedPackedWasm32Guest(t *testing.T) {
	guest, err := os.ReadFile("testdata/tinygo-wasm32-packed.generated.wasm")
	if err != nil {
		t.Fatal(err)
	}
	runtime := newTestRuntime(t, nil)
	compiled, err := runtime.Compile(guest)
	if err != nil {
		t.Fatal(err)
	}
	defer compiled.Close()
	instance, err := runtime.Instantiate(context.Background(), compiled)
	if err != nil {
		t.Fatal(err)
	}
	defer instance.Close()

	for _, export := range []string{
		"roundtrip",
		"empty",
		"overlap_case",
		"output_small",
		"bounds",
		"checksum",
		"truncated",
		"invalid_level",
	} {
		t.Run(export, func(t *testing.T) {
			result, err := instance.Invoke(export)
			if err != nil || len(result) != 1 || result[0] != 0 {
				t.Fatalf("%s = %v, %v", export, result, err)
			}
		})
	}
}
