//go:build js

package app

import (
	"strconv"
	"syscall/js"
)

// volumeKey is where the browser keeps the volume: the app has no files
// there, and settings.json is written only where there are.
const volumeKey = "mygo-music.volume"

// savedVolume is where the volume was left, if the browser kept it.
func savedVolume() (float64, bool) {
	v := js.Global().Get("localStorage").Call("getItem", volumeKey)
	if v.Type() != js.TypeString {
		return 0, false
	}
	f, err := strconv.ParseFloat(v.String(), 64)
	if err != nil || f < 0 || f > 1 {
		return 0, false
	}
	return f, true
}

// saveVolume keeps the volume for the next time the app opens.
func saveVolume(v float64) {
	js.Global().Get("localStorage").Call("setItem", volumeKey, strconv.FormatFloat(v, 'f', -1, 64))
}
