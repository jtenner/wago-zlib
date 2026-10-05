// Package register exposes this module's explicit Wago provider catalog.
package register

import (
	wagozlib "github.com/jtenner/wago-zlib"
	wago "github.com/wago-org/wago"
)

// Providers returns a fresh, side-effect-free provider catalog.
func Providers() []wago.PluginProvider {
	return []wago.PluginProvider{wagozlib.Provider()}
}
