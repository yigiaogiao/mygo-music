package player

import (
	"sync"
	"testing"
	"time"

	"mygo-music/internal/music"
)

// fakeElement is the page's media element in a test: the test makes it end,
// fail or wait, as the browser would.
type fakeElement struct {
	mu        sync.Mutex
	playing   bool
	pos, dur  time.Duration
	buffering bool
	closed    bool
	end       func()
	fail      func(error)
}

func (f *fakeElement) Play() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.playing = true
}

func (f *fakeElement) Pause() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.playing = false
}

func (f *fakeElement) Playing() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.playing
}

func (f *fakeElement) Seek(t time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pos = t
}

// SeekInstant is what the page's element does: it jumps at once, playing.
func (f *fakeElement) SeekInstant(t time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pos = t
}

func (f *fakeElement) Position() time.Duration {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.pos
}

func (f *fakeElement) Duration() time.Duration { return f.dur }

func (f *fakeElement) SetVolume(float64) {}

func (f *fakeElement) OnEnd(fn func()) { f.end = fn }

func (f *fakeElement) OnError(fn func(error)) { f.fail = fn }

func (f *fakeElement) Buffering() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.buffering
}

func (f *fakeElement) setBuffering(on bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.buffering = on
}

func (f *fakeElement) Close() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
}

// elements makes the player play with fake elements, and returns them.
func elements(t *testing.T) *[]*fakeElement {
	t.Helper()
	var made []*fakeElement
	old := elementSinkFor
	elementSinkFor = func(src music.Source, tr music.Track) (sink, bool) {
		f := &fakeElement{dur: tr.Length(), buffering: true}
		made = append(made, f)
		return f, true
	}
	t.Cleanup(func() { elementSinkFor = old })
	return &made
}

func twoSongs() []music.Track {
	return []music.Track{
		{ID: "1", Name: "one", Duration: 60_000},
		{ID: "2", Name: "two", Duration: 60_000},
	}
}

// until runs what the player posts until cond holds: a load posts more than
// the change it's waited for, such as its lyrics.
func until(t *testing.T, ch chan func(), cond func() bool) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for !cond() {
		select {
		case fn := <-ch:
			fn()
		case <-deadline:
			t.Fatal("the player didn't get there")
		}
	}
}

// The element plays what the player gives it, says when it waits, and the
// next song plays when one ends.
func TestElementPlaysAndGoesOn(t *testing.T) {
	ch := make(chan func(), 16)
	p := New(func(fn func()) { ch <- fn })
	made := elements(t)
	p.PlayQueue(twoSongs(), 0)

	if got := *made; len(got) != 1 {
		t.Fatalf("%d elements for the first song", len(got))
	}
	if got := (*made)[0]; !got.Playing() || !p.Playing() {
		t.Error("the first song doesn't play")
	}
	if got := (*made)[0]; !got.Buffering() || !p.Loading() {
		t.Error("a song loading in the element doesn't show as loading")
	}
	(*made)[0].setBuffering(false)
	if p.Loading() {
		t.Error("still loading once the element can play it")
	}

	// It ends: the next plays, by itself, as with the screen off.
	(*made)[0].end()
	until(t, ch, func() bool { return len(*made) == 2 })
	if p.Current().ID != "2" || !(*made)[1].Playing() {
		t.Errorf("after the first song ended: %q playing %v", p.Current().ID, (*made)[1].Playing())
	}
}

// What the browser can't play is asked for again rather than skipped: its
// error says the same of a file it couldn't fetch as of one it can't decode.
func TestElementRetriesWhatItCantPlay(t *testing.T) {
	ch := make(chan func(), 16)
	p := New(func(fn func()) { ch <- fn })
	p.after = func(time.Duration, func()) {}
	made := elements(t)
	p.PlayQueue(twoSongs(), 0)

	(*made)[0].fail(errUnplayable)
	until(t, ch, func() bool { return p.Retrying() })
	if got := *made; len(got) != 1 {
		t.Fatalf("%d elements while the failure is waited out", len(got))
	}
	if p.Current().ID != "1" {
		t.Errorf("after the failure, current is %q", p.Current().ID)
	}
	if p.Err() == "" {
		t.Error("the failure isn't said")
	}
}

// Picking up a session plays its song from where it was left, paused.
func TestElementRestoresAtPosition(t *testing.T) {
	ch := make(chan func(), 16)
	p := New(func(fn func()) { ch <- fn })
	made := elements(t)
	queue := twoSongs()[:1]
	p.Restore(Session{Queue: queue, Order: []int{0}, At: 0, Position: 90_000})

	if got := *made; len(got) != 1 {
		t.Fatalf("%d elements", len(got))
	}
	if got := (*made)[0].Position(); got != 90*time.Second {
		t.Errorf("restored at %v, want 1m30s", got)
	}
	if (*made)[0].Playing() || p.Playing() {
		t.Error("a restored session plays before the listener asks")
	}
	if got := p.Position(); got != 90*time.Second {
		t.Errorf("position %v, want 1m30s", got)
	}
}

// A seek on a phone's element jumps at once, playing: the lock screen's
// seek may come while the page is out of sight, where a pause and a timer
// around it could wait.
func TestElementSeekJumpsAtOnce(t *testing.T) {
	ch := make(chan func(), 16)
	p := New(func(fn func()) { ch <- fn })
	made := elements(t)
	p.PlayQueue(twoSongs(), 0)
	e := (*made)[0]
	if !e.Playing() {
		t.Fatal("the song doesn't play")
	}

	p.Seek(30 * time.Second)
	if !e.Playing() {
		t.Error("paused for the seek")
	}
	if got := e.Position(); got != 30*time.Second {
		t.Errorf("the element is at %v, want 30s", got)
	}
	if got := p.Position(); got != 30*time.Second {
		t.Errorf("the player says %v, want 30s", got)
	}
}
