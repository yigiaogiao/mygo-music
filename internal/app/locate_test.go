package app

import (
	"testing"

	"mygo-music/internal/music"
)

// The button that locates the track playing finds it by what makes it that
// track, not by its name: a netdisk may hold the same song in two folders,
// which are two tracks, and only one of them plays.
func TestPlayingAtTellsTracksOfOneNameApart(t *testing.T) {
	a := song("a", "西口之夜")
	a.Source = "netdisk"
	b := song("b", "西口之夜")
	b.Source = "netdisk"
	tracks := []music.Track{a, b}
	if at := playingAt(&tracks[1], tracks); at != 1 {
		t.Errorf("playing the second of two, at %d", at)
	}
	if at := playingAt(&music.Track{ID: "a", Source: "elsewhere"}, tracks); at != -1 {
		t.Errorf("a track of another source, at %d", at)
	}
	if at := playingAt(nil, tracks); at != -1 {
		t.Errorf("nothing playing, at %d", at)
	}
}

// The button that locates the track playing on a page of sections goes to the
// row that lists it; the card that leads Home shows it among others, which is
// no row to go to, and a page that doesn't list it has none.
func TestBrowsePlayingRowGoesToListedTracks(t *testing.T) {
	cur := song("t2", "二")
	cur.Source = "netdisk"
	listed := song("t2", "二")
	listed.Source = "netdisk"
	page := &music.Browse{Sections: []music.Section{
		{Title: "Recently Played", Tracks: []music.Track{listed}},
		{Title: "Songs", Tracks: []music.Track{{ID: "t1", Source: "netdisk"}}},
	}}
	// The first section of Home is its feature card; the rows start after it.
	layout := browseLayout(page, "", 4)
	if at := browsePlayingRow(&cur, page, layout); at >= 0 {
		t.Errorf("on the card of Home, at %d", at)
	}
	page.Title = "Songs"
	layout = browseLayout(page, "songs", 4)
	want := -1
	for i, r := range layout {
		if r.kind == rowTrack && page.Sections[r.section].Tracks[r.from].ID == "t2" {
			want = i
		}
	}
	if at := browsePlayingRow(&cur, page, layout); at != want || at < 0 {
		t.Errorf("at %d, want the row of the track, %d", at, want)
	}
	other := song("t3", "三")
	other.Source = "netdisk"
	if at := browsePlayingRow(&other, page, layout); at != -1 {
		t.Errorf("a track the page doesn't list, at %d", at)
	}
}
