package register

import (
	"bytes"
	"os"
	"testing"

	wagozlib "github.com/jtenner/wago-zlib"
	wago "github.com/wago-org/wago"
)

func TestProviderCatalog(t *testing.T) {
	providers := Providers()
	if len(providers) != 1 || providers[0].Definition.ID != wagozlib.ID || providers[0].New == nil || providers[0].New() == nil {
		t.Fatalf("Providers() = %#v", providers)
	}
	want, err := wago.EncodeProviderCatalog("github.com/jtenner/wago-zlib/register", providers)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile("../wago.providers.json")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("wago.providers.json is stale; run: wago plugin catalog")
	}
}
