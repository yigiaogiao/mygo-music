package player

import (
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/audio"

	"mygo-music/internal/music"
)

func TestPlayEmptyQueue(t *testing.T) {
	p := &Player{at: -1}
	p.PlayQueue(nil, 0) // an empty playlist's play button
	if p.Current() != nil {
		t.Error("playing from an empty queue")
	}
}

// silence is a long song of nothing.
type silence struct{}

func (silence) SampleRate() int               { return 48000 }
func (silence) Channels() int                 { return 2 }
func (silence) Len() int64                    { return 48000 * 600 }
func (silence) SetPosition(int64) error       { return nil }
func (silence) Read(b []float32) (int, error) { clear(b); return len(b), nil }

// posted collects what the player posts to the main thread, for the test
// to run there.
func posted() (chan func(), *Player) {
	ch := make(chan func(), 16)
	p := New(func(fn func()) { ch <- fn })
	p.Cue([]music.Track{{ID: "1", Name: "one", Duration: 600000}})
	out := audio.NewPlayer(silence{})
	out.SetVolume(0)
	out.Play()
	p.Attach(out)
	p.fade.attach(p.out, 1)
	return ch, p
}

func next(t *testing.T, ch chan func()) {
	t.Helper()
	select {
	case fn := <-ch:
		fn()
	case <-time.After(2 * time.Second):
		t.Fatal("nothing posted")
	}
}

// Pausing fades out before the music stops; playing again fades it in.
func TestPauseFades(t *testing.T) {
	ch, p := posted()
	defer p.out.Close()
	p.TogglePlay()
	if p.Playing() || !p.out.Playing() {
		t.Fatal("paused at once, or not at all")
	}
	next(t, ch)
	if p.out.Playing() {
		t.Fatal("still playing after fading out")
	}
	if !p.fade.silent() {
		t.Error("faded out to something")
	}
	p.TogglePlay()
	if !p.Playing() || !p.out.Playing() {
		t.Fatal("did not play again")
	}
	time.Sleep(fadeIn + 50*time.Millisecond)
	p.fade.mu.Lock()
	level := p.fade.level(time.Now())
	p.fade.mu.Unlock()
	if level != 1 {
		t.Errorf("faded in to %v", level)
	}
}

// A seek playing pauses, which fades out over a few milliseconds, and
// plays again once there, its position the one asked for meanwhile.
func TestSeekWithoutClick(t *testing.T) {
	ch, p := posted()
	defer p.out.Close()
	p.Seek(90 * time.Second)
	if p.out.Playing() {
		t.Error("jumped while playing")
	}
	if p.Position() != 90*time.Second {
		t.Errorf("position %v while seeking", p.Position())
	}
	next(t, ch)
	if !p.out.Playing() || p.out.Position() < 90*time.Second {
		t.Errorf("after the seek: playing %v at %v", p.out.Playing(), p.out.Position())
	}
}

// Seeks one after the other, as the progress bar asks when let go, play
// on from the last.
func TestSeekTwicePlaysOn(t *testing.T) {
	ch, p := posted()
	defer p.out.Close()
	p.Seek(90 * time.Second)
	p.Seek(95 * time.Second)
	next(t, ch)
	for len(ch) > 0 {
		next(t, ch)
	}
	time.Sleep(seekGap * 2)
	for len(ch) > 0 {
		next(t, ch)
	}
	if !p.out.Playing() || p.out.Position() < 95*time.Second {
		t.Errorf("after two seeks: playing %v at %v", p.out.Playing(), p.out.Position())
	}
}

// A radio asks for more as its queue nears its end, and adds what is not
// in it already.
func TestRadioRefills(t *testing.T) {
	p := &Player{at: -1}
	p.Cue([]music.Track{{ID: "1"}, {ID: "2"}})
	asked := 0
	p.AttachRadio("station", func(got func([]music.Track)) {
		asked++
		got([]music.Track{{ID: "2"}, {ID: "3"}})
	})
	if asked != 1 || len(p.Queue()) != 3 || p.Queue()[2].ID != "3" {
		t.Fatalf("asked %d, queue %v", asked, p.Queue())
	}
	if p.Radio() != "station" {
		t.Errorf("radio %q", p.Radio())
	}
	// Playing something else ends the radio.
	p.endRadio()
	if p.Radio() != "" || p.feed != nil {
		t.Error("radio still on")
	}
}

func TestNormGain(t *testing.T) {
	for _, c := range []struct {
		st   *music.Stream
		want float64
	}{
		{nil, 1},
		{&music.Stream{}, 1},
		{&music.Stream{Gain: -6.6, Peak: 1}, math.Pow(10, -2.6/20)},
		// Quiet, it is lifted, but not past its peak.
		{&music.Stream{Gain: 3, Peak: 0.8}, 1.25},
		{&music.Stream{Gain: -4, Peak: 0}, 1},
	} {
		if g := normGain(c.st); math.Abs(g-c.want) > 1e-9 {
			t.Errorf("%+v: %v, want %v", c.st, g, c.want)
		}
	}
}

// The cache gives back what it kept, at the quality asked for or a better
// one, and drops what was played longest ago once over its limit.
func TestCache(t *testing.T) {
	dir := t.TempDir()
	c := NewCache(dir, 400)
	store := func(id string, st *music.Stream, n int) {
		f := c.part(st)
		f.Write(make([]byte, n))
		f.Close()
		c.keep("src", id, st.Quality, st, f.Name(), int64(n))
	}
	store("1", &music.Stream{Quality: "lossless", Type: "flac", Gain: -3}, 100)
	dl, st, ok := c.lookup("src", "1", "high")
	if !ok || dl.size != 100 || st.Type != "flac" || st.Gain != -3 {
		t.Fatalf("lookup: %v %+v", ok, st)
	}
	dl.Close()
	if _, _, ok := c.lookup("other", "1", "high"); ok {
		t.Error("found another source's song of the same ID")
	}
	old := time.Now().Add(-time.Hour)
	os.Chtimes(filepath.Join(dir, cacheKey("src", "1", "lossless")+".flac"), old, old)
	store("2", &music.Stream{Quality: "high", Type: "mp3"}, 100)
	store("3", &music.Stream{Quality: "high", Type: "mp3"}, 100)
	store("4", &music.Stream{Quality: "high", Type: "mp3"}, 100)
	store("5", &music.Stream{Quality: "high", Type: "mp3"}, 100)
	if _, _, ok := c.lookup("src", "1", "standard"); ok {
		t.Error("kept the song played longest ago")
	}
	if _, _, ok := c.lookup("src", "2", "lossless"); ok {
		t.Error("found a high quality song for lossless")
	}
	if dl, _, ok := c.lookup("src", "5", "standard"); !ok {
		t.Error("dropped the song just kept")
	} else {
		dl.Close()
	}
	if n := c.Size(); n > 400 {
		t.Errorf("takes %d of 400", n)
	}
	c.Clear()
	if n := c.Size(); n != 0 {
		t.Errorf("cleared to %d", n)
	}
}

// Leaving a radio keeps the song playing and those before it, drops those
// it queued after, and asks it for no more.
func TestEndRadio(t *testing.T) {
	p := &Player{at: -1}
	p.Cue([]music.Track{{ID: "1"}, {ID: "2"}, {ID: "3"}, {ID: "4"}})
	p.at, p.track = 1, &p.queue[1]
	asked := 0
	p.AttachRadio("station", func(got func([]music.Track)) { asked++ })
	p.EndRadio()
	if p.Radio() != "" || p.Current().ID != "2" {
		t.Fatalf("radio %q, current %v", p.Radio(), p.Current())
	}
	if q := p.Queue(); len(q) != 2 || q[0].ID != "1" || q[1].ID != "2" {
		t.Fatalf("queue %v", q)
	}
	p.refill()
	if asked != 0 {
		t.Error("asked the radio after leaving it")
	}
}

// picky plays the songs in ok, from one on disk, and says the rest aren't
// available.
type picky struct {
	nothing
	ok map[string]bool
}

func (s picky) Stream(id, _ string) (*music.Stream, error) {
	if !s.ok[id] {
		return nil, music.ErrUnavailable
	}
	path, err := filepath.Abs("testdata/seek.mp3")
	return &music.Stream{URL: "file://" + filepath.ToSlash(path), Type: "mp3"}, err
}

// pickyQueue is a, b and c, of which b can't play.
func pickyQueue() (*Player, chan func(), []music.Track) {
	ch := make(chan func(), 64)
	p := New(func(fn func()) { ch <- fn })
	p.SetVolume(0)
	src := picky{ok: map[string]bool{"a": true, "c": true}}
	p.Source = func(string) music.Source { return src }
	return p, ch, []music.Track{{ID: "a", Source: "s"}, {ID: "b", Source: "s"}, {ID: "c", Source: "s"}}
}

// settle runs what the player posts until it loaded what it went on to.
func settle(t *testing.T, ch chan func(), p *Player) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for p.Loading() {
		select {
		case fn := <-ch:
			fn()
		case <-deadline:
			t.Fatal("still loading")
		}
	}
}

// A song the listener picks that can't play stays, saying why. Clicking one
// the source refused for a moment used to play the one after it instead.
func TestPickedTrackStays(t *testing.T) {
	p, ch, q := pickyQueue()
	defer p.stop()
	p.PlayTrack(q, 1)
	settle(t, ch, p)
	if p.Current().ID != "b" || p.Playing() || p.Err() == "" {
		t.Fatalf("on %s, playing %v, err %q", p.Current().ID, p.Playing(), p.Err())
	}
	p.Jump(0)
	settle(t, ch, p)
	p.Jump(1)
	settle(t, ch, p)
	if p.Current().ID != "b" || p.Playing() {
		t.Errorf("jumped to %s, playing %v", p.Current().ID, p.Playing())
	}
}

// Previous passes a song that can't play going back, as Next does going
// forward. It used to skip forward, back onto the song playing, so Previous
// seemed to do nothing.
func TestPrevSkipsBack(t *testing.T) {
	p, ch, q := pickyQueue()
	defer p.stop()
	p.PlayTrack(q, 2)
	settle(t, ch, p)
	p.Prev()
	settle(t, ch, p)
	if p.Current().ID != "a" || !p.Playing() {
		t.Fatalf("previous went to %s, playing %v", p.Current().ID, p.Playing())
	}
	p.Next()
	settle(t, ch, p)
	if p.Current().ID != "c" || !p.Playing() {
		t.Errorf("next went to %s, playing %v", p.Current().ID, p.Playing())
	}
}

// Shuffling a whole list may start on any song, not always its first; a
// song picked to start stays first.
func TestShuffleOrderHead(t *testing.T) {
	p := &Player{order: make([]int, 10)}
	heads := map[int]bool{}
	for range 200 {
		for i := range p.order {
			p.order[i] = i
		}
		p.shuffleOrder(-1)
		heads[p.order[0]] = true
	}
	if len(heads) < 5 {
		t.Errorf("shuffled lists start on only %v", heads)
	}
	p.shuffleOrder(3)
	if p.order[0] != 3 {
		t.Errorf("picked song is at %v", p.order)
	}
}
