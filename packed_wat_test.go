package wagozlib

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	wago "github.com/wago-org/wago"
)

func TestWagoPackedWATParity(t *testing.T) {
	for _, abi := range []string{"wasm32", "wasm64", "gc"} {
		t.Run(abi, func(t *testing.T) {
			guest, err := os.ReadFile(filepath.Join("testdata", "packed-"+abi+".wasm"))
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
			for _, export := range []string{"success_parity", "failure_parity"} {
				result, err := instance.Invoke(export)
				if err != nil || len(result) != 1 || wago.AsI32(result[0]) != 1 {
					t.Fatalf("%s = %v, %v", export, result, err)
				}
			}
		})
	}
}

func TestPackedResultLayoutAndFailureInvariant(t *testing.T) {
	const written = uint64(0x12345678)
	if got := packedResult(StatusOK, written); uint32(got) != uint32(StatusOK) || uint32(got>>32) != uint32(written) {
		t.Fatalf("success packed result = %#x", got)
	}
	if got := packedResult(StatusInvalidArgument, written); uint32(got) != uint32(StatusInvalidArgument) || got>>32 != 0 {
		t.Fatalf("failure packed result = %#x", got)
	}
}
