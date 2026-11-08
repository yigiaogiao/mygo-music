//go:build !js

package app

import "mygo-music/internal/music"

// platformSource is nil on the desktop, whose sources are the files and the
// plugins.
func platformSource() music.Source { return nil }

// webBuild reports whether this is the browser app.
func webBuild() bool { return false }
