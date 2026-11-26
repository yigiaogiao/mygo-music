//go:build js && wasm

package app

import (
	"syscall/js"

	"github.com/egoist/mygo"
)

// A phone goes back with a swipe from the screen's edge, and Android with its
// back button: both go back in the page's history, and leave the page when
// there is none. A page of the app is not a page of the browser, so without
// history of its own the app would be left from any level: each level over
// the root adds an entry instead, and a gesture takes one off again, as the
// back button of the app does. The app's own back takes the entry off in
// turn, so the page's history stays as deep as the app is.

var (
	hist          js.Value // the page's history
	histDepth     int      // the entries added over the page's own
	histPopping   int      // entries being taken off by the app, not the listener
	histFunc      js.Func
	histInstalled bool
)

// installHistory hears the page's back and forward gestures.
func (a *app) installHistory() {
	if histInstalled {
		return
	}
	histInstalled = true
	hist = js.Global().Get("history")
	histFunc = js.FuncOf(func(_ js.Value, args []js.Value) any {
		if histPopping > 0 {
			histPopping--
			return nil
		}
		to := 0
		if len(args) > 0 && args[0].Type() == js.TypeObject {
			if s := args[0].Get("state"); s.Type() == js.TypeObject {
				to = s.Get("mygo").Int()
			}
		}
		mygo.RunOnMain(func() {
			for histDepth > to {
				a.back()
				histDepth--
			}
			for histDepth < to {
				a.goForward()
				histDepth++
			}
			a.redraw()
		})
		return nil
	})
	js.Global().Call("addEventListener", "popstate", histFunc)
}

// syncHistory makes the page's history as deep as the app is: an entry for
// each page gone back from, and one for each cover over it.
func (a *app) syncHistory(mobile bool) {
	if !histInstalled {
		return
	}
	want := a.historyDepth(mobile)
	for histDepth < want {
		histDepth++
		hist.Call("pushState", map[string]any{"mygo": histDepth}, "")
	}
	for histDepth > want {
		histDepth--
		histPopping++
		hist.Call("back")
	}
}
