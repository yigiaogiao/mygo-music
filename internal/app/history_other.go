//go:build !(js && wasm)

package app

// installHistory and syncHistory keep the page's history as deep as the app
// is in it, which only a browser needs: a window has the app's own back.
func (a *app) installHistory() {}

func (a *app) syncHistory(bool) {}
