//go:build !js

package app

import (
	"github.com/egoist/mygo"

	"mygo-music/internal/music"
)

// platformSource is nil on the desktop, whose sources are the files and the
// plugins.
func platformSource() music.Source { return nil }

// webBuild reports whether this is the browser app.
func webBuild() bool { return false }

// openFolderSetup opens the server's folder page in the desktop's browser.
func openFolderSetup(url string) { mygo.Shell.OpenExternal(url) }
