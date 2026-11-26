//go:build js

package app

import (
	"encoding/json"
	"syscall/js"
)

// settingsKey is where the browser keeps the listener's settings: it has no
// user data directory for settings.json, and its storage outlives updates,
// so an experiment turned on stays on the next time the app opens.
const settingsKey = "mygo-music.settings"

// loadBrowserSettings reads what the browser kept over the defaults.
func loadBrowserSettings(s *settings) {
	v := js.Global().Get("localStorage").Call("getItem", settingsKey)
	if v.Type() != js.TypeString {
		return
	}
	json.Unmarshal([]byte(v.String()), s)
}

// saveBrowserSettings keeps the settings for the next time the app opens.
func saveBrowserSettings(s settings) {
	b, err := json.Marshal(s)
	if err != nil {
		return
	}
	js.Global().Get("localStorage").Call("setItem", settingsKey, string(b))
}
