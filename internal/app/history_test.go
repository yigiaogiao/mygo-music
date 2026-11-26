package app

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"mygo-music/internal/music"
)

// A phone's back gesture goes back one level at a time, so the page's
// history is kept as deep as the app is: a page behind, Now Playing, the
// settings and the drawer each count for one.
func TestHistoryDepth(t *testing.T) {
	a := newTestApp()
	if got := a.historyDepth(true); got != 0 {
		t.Errorf("at the root, depth is %d", got)
	}
	a.history = []route{{page: pageStations}}
	if got := a.historyDepth(true); got != 1 {
		t.Errorf("a page behind, depth is %d", got)
	}
	a.nowPlaying = true
	a.settingsOpen = true
	a.navOpen = true
	if got := a.historyDepth(false); got != 3 {
		t.Errorf("page, Now Playing and settings, depth is %d", got)
	}
	if got := a.historyDepth(true); got != 4 {
		t.Errorf("with the drawer open on a phone, depth is %d", got)
	}
}

// The side buttons of a mouse go back and forward between pages.
func TestBackAndForward(t *testing.T) {
	a := newTestApp()
	a.playlists["42"] = &music.Playlist{ID: "42", Name: "飙升榜", Tracks: []music.Track{{ID: "1", Name: "一张相片"}}}
	a.stationsOnce = true
	tt := ui.NewTester(a.view, 1200, 800)
	stations, list := route{page: pageStations, src: "fake"}, route{page: pagePlaylist, src: "fake", id: "42"}
	a.open(stations)
	a.open(list)
	tt.Frame()

	tt.Key(0, ui.KeyBack)
	tt.Frame()
	if a.route != stations {
		t.Fatalf("back went to %+v", a.route)
	}
	tt.Key(0, ui.KeyForward)
	tt.Frame()
	if a.route != list {
		t.Fatalf("forward went to %+v", a.route)
	}

	// Back closes Now Playing before it leaves the page under it.
	a.pl.Cue([]music.Track{{ID: "1", Name: "一张相片"}})
	a.nowPlaying = true
	tt.Frame()
	tt.Key(0, ui.KeyBack)
	tt.Frame()
	if a.nowPlaying || a.route != list {
		t.Fatalf("back with Now Playing open: open %v, page %+v", a.nowPlaying, a.route)
	}

	// A new page leaves nothing to go forward to.
	tt.Key(0, ui.KeyBack)
	tt.Frame()
	home := route{page: pageHome, src: "fake"}
	a.open(home)
	tt.Key(0, ui.KeyForward)
	tt.Frame()
	if a.route != home {
		t.Fatalf("forward after a new page went to %+v", a.route)
	}
}
