package app

import (
	"testing"

	"mygo-music/internal/music"
	"mygo-music/internal/playback"
	"mygo-music/internal/player"
	"mygo-music/internal/websource"
)

// The queue another device played comes over as it was, from where it is.
func TestSessionFromStateCarriesTheQueue(t *testing.T) {
	queue := []music.Track{
		{ID: "a", Source: "lazycat", Name: "One", Duration: 1000},
		{ID: "b", Source: "lazycat", Name: "Two", Duration: 2000},
		{ID: "c", Source: "lazycat", Name: "Three", Duration: 3000},
	}
	st := playback.State{
		Track: &playback.Track{ID: "b", Name: "Two", Duration: 2000},
		Queue: queue, Order: []int{2, 0, 1}, At: 1, Shuffle: true, Repeat: 2,
		Position: 900, Duration: 2000,
	}
	s, ok := sessionFromState(st, 1234)
	if !ok {
		t.Fatal("no session")
	}
	if len(s.Queue) != 3 || s.Queue[1].ID != "b" {
		t.Errorf("queue: %+v", s.Queue)
	}
	if len(s.Order) != 3 || s.Order[0] != 2 || s.At != 1 {
		t.Errorf("order: %v at %d", s.Order, s.At)
	}
	if !s.Shuffle || s.Repeat != player.RepeatMode(2) {
		t.Errorf("shuffle %v repeat %v", s.Shuffle, s.Repeat)
	}
	if s.Position != 1234 {
		t.Errorf("position: %d", s.Position)
	}
}

// A queue that can't be picked up, as one from another version would be,
// leaves the song alone to play.
func TestSessionFromStateFallsBackToTheTrack(t *testing.T) {
	st := playback.State{
		Track:    &playback.Track{ID: "a", Name: "One", Artist: "Someone", Album: "Somewhere", Duration: 1000},
		Queue:    []music.Track{{ID: "a"}, {ID: "b"}},
		Order:    []int{0},
		Position: 200,
	}
	s, ok := sessionFromState(st, 250)
	if !ok {
		t.Fatal("no session")
	}
	if len(s.Queue) != 1 || len(s.Order) != 1 || s.Order[0] != 0 || s.At != 0 {
		t.Fatalf("queue: %+v order %v at %d", s.Queue, s.Order, s.At)
	}
	tr := s.Queue[0]
	if tr.ID != "a" || tr.Name != "One" || tr.Source != websource.ID || tr.Duration != 1000 {
		t.Errorf("track: %+v", tr)
	}
	if len(tr.Artists) != 1 || tr.Artists[0].Name != "Someone" || tr.Album.Name != "Somewhere" {
		t.Errorf("artists %+v album %+v", tr.Artists, tr.Album)
	}
	if s.Position != 250 {
		t.Errorf("position: %d", s.Position)
	}
}

// A queue is told apart by its songs, their order and how it plays, not by
// where in the song it is, which moves every second.
func TestSessionSigIgnoresThePosition(t *testing.T) {
	base := player.Session{
		Queue: []music.Track{{ID: "a"}, {ID: "b"}}, Order: []int{0, 1},
		At: 1, Shuffle: true, Repeat: player.RepeatOne, Position: 100,
	}
	moved := base
	moved.Position = 9000
	if sessionSig(base) != sessionSig(moved) {
		t.Error("position changed the signature")
	}
	reordered := base
	reordered.Order = []int{1, 0}
	if sessionSig(base) == sessionSig(reordered) {
		t.Error("the order did not change the signature")
	}
}

func TestDeviceLabel(t *testing.T) {
	if got := deviceLabel("iPhone"); got != "iPhone" {
		t.Errorf("iPhone: %q", got)
	}
	if got := deviceLabel(""); got != "Device" {
		t.Errorf("empty: %q", got)
	}
	if got := deviceLabel("Device"); got != "Device" {
		t.Errorf("Device: %q", got)
	}
}
