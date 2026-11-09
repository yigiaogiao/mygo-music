package app

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"mygo-music/internal/plugin"
)

// The netdisk and the files of the listener's own play as they are: only a
// plugin may have a quality to choose, and the page keeps even loudness alone
// without one.
func TestQualityOnlyWithAPlugin(t *testing.T) {
	a := newTestApp()
	a.settingsPage = "playback"
	tt := ui.NewTester(a.settingsView, 760, 1000)
	tt.Frame()
	if _, ok := tt.Find("Quality"); ok {
		t.Fatal("a quality without a plugin to choose it")
	}
	if _, ok := tt.Find("Even Out Loudness"); !ok {
		t.Fatal("no Even Out Loudness")
	}
	a.libs = append(a.libs, &library{src: &plugin.Plugin{}})
	tt.Frame()
	if _, ok := tt.Find("Quality"); !ok {
		t.Fatal("no quality with a plugin")
	}
}

// A build that can't update itself, as the browser's, has no page for
// updates to show.
func TestNoUpdatesPageWithoutUpdater(t *testing.T) {
	a := newTestApp()
	a.settingsPage = "updates"
	tt := ui.NewTester(a.settingsView, 760, 1000)
	tt.Frame()
	if _, ok := tt.Find("Check for Updates Automatically"); ok {
		t.Fatal("update settings in a build without updates")
	}
}
