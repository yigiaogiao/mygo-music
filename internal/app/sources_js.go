//go:build js

package app

import (
	"syscall/js"

	"mygo-music/internal/music"
	"mygo-music/internal/websource"
)

// platformSource is the source of the browser app: a server that reads the
// files. It answers from the page's own origin.
func platformSource() music.Source {
	base := js.Global().Get("location").Get("origin").String()
	if base == "" || base == "null" {
		base = "http://127.0.0.1:8090"
	}
	return websource.New(base)
}

// webBuild reports whether this is the browser app.
func webBuild() bool { return true }

// openFolderSetup takes the browser to the server's folder page in the same
// window. window.open is blocked in the LazyCat desktop and mobile webviews,
// so a popup would leave the button looking dead.
func openFolderSetup(url string) {
	js.Global().Get("location").Call("assign", url)
}
