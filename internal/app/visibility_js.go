//go:build js

package app

import (
	"syscall/js"

	"github.com/egoist/mygo"
)

// sightFunc is kept for the life of the page: a function JavaScript holds
// goes once Go's garbage collector does, if nothing here does.
var sightFunc js.Func

// watchSight calls fn when the page comes back into sight, as when the
// listener unlocks the phone: the network may be back with it.
func watchSight(fn func()) {
	doc := js.Global().Get("document")
	sightFunc = js.FuncOf(func(js.Value, []js.Value) any {
		if !doc.Get("hidden").Bool() {
			mygo.RunOnMain(fn)
		}
		return nil
	})
	doc.Call("addEventListener", "visibilitychange", sightFunc)
}
