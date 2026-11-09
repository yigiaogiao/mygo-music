package app

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

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

// Loudness is always evened out, and a source's quality is the app's to ask
// of it: the settings have no page for playback.
func TestNoPlaybackPage(t *testing.T) {
	a := newTestApp()
	tt := ui.NewTester(a.settingsView, 760, 1000)
	tt.Frame()
	if _, ok := tt.Find("Playback"); ok {
		t.Fatal("a Playback page in the sidebar")
	}
	if _, ok := tt.Find("Even Out Loudness"); ok {
		t.Fatal("an Even Out Loudness row")
	}
}

// The settings shown over the page go back the way they came: no cross
// beside the back button, which did the same.
func TestSettingsSheetHasOnlyBack(t *testing.T) {
	a := newTestApp()
	a.settingsOpen = true
	tt := ui.NewTester(a.settingsSheet, 760, 1000)
	tt.Frame()
	if _, ok := tt.Find("Back"); !ok {
		t.Fatal("no way back out of the settings")
	}
	if _, ok := tt.Find("Close"); ok {
		t.Fatal("a Close button beside the back button")
	}
}
