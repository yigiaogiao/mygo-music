//go:build js

package app

import "syscall/js"

// pageColor is the color last given to the page around the app, and the
// browser's own bar, if it may take one.
var pageColor string

// setPageColor paints the page around the app in the appearance's color. The
// app draws inside the safe area, so that a notch or a home indicator never
// covers its controls; the room it leaves would otherwise show the dark page
// of a light app, as a black edge at the top.
func setPageColor(dark bool) {
	color := "#ffffff"
	if dark {
		color = "#141416"
	}
	if color == pageColor {
		return
	}
	pageColor = color
	doc := js.Global().Get("document")
	doc.Call("querySelector", "html").Get("style").Set("backgroundColor", color)
	doc.Get("body").Get("style").Set("backgroundColor", color)
	doc.Call("getElementById", "mygo").Get("style").Set("backgroundColor", color)
	metas := doc.Call("querySelectorAll", `meta[name="theme-color"]`)
	for i := 0; i < metas.Length(); i++ {
		metas.Index(i).Call("setAttribute", "content", color)
	}
}
