//go:build !js

package app

// savedVolume is where the volume was left, which elsewhere is in the
// settings, loaded with them.
func savedVolume() (float64, bool) { return 0, false }

// saveVolume keeps the volume for the next time the app opens, which
// elsewhere the settings do as they quit.
func saveVolume(float64) {}
