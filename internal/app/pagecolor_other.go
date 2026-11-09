//go:build !js

package app

// setPageColor is the browser's alone: a window paints its own background.
func setPageColor(bool) {}
