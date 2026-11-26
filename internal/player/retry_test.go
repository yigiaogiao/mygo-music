package player

import (
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"mygo-music/internal/music"
)

// flaky says the songs can't play for as many asks as it's told, as a phone
// whose network went does, then plays them from disk.
type flaky struct {
	nothing
	fails int
	asks  int
}

func (s *flaky) Stream(string, string) (*music.Stream, error) {
	s.asks++
	if s.asks <= s.fails {
		return nil, errors.New("dial tcp: no route to host")
	}
	path, err := filepath.Abs("testdata/seek.mp3")
	return &music.Stream{URL: "file://" + filepath.ToSlash(path), Type: "mp3"}, err
}

// atOnce is p.after for a test that would rather not wait.
func atOnce(_ time.Duration, fn func()) { fn() }

// retryQueue plays a, of one song, on a player that waits for what failed
// as after says.
func retryQueue(t *testing.T, src *flaky, after func(time.Duration, func())) (*Player, chan func()) {
	t.Helper()
	ch := make(chan func(), 64)
	p := New(func(fn func()) { ch <- fn })
	p.SetVolume(0)
	p.Source = func(string) music.Source { return src }
	p.after = after
	t.Cleanup(p.stop)
	p.PlayQueue([]music.Track{{ID: "a", Source: "s"}}, 0)
	return p, ch
}

// runPosts runs what the player posts, up to n times, until done.
func runPosts(t *testing.T, p *Player, ch chan func(), n int, done func() bool) {
	t.Helper()
	for i := 0; i < n && !done(); i++ {
		select {
		case fn := <-ch:
			fn()
		case <-time.After(2 * time.Second):
			t.Fatalf("nothing posted, %d run, %d failed, waiting %v, playing %v, err %q",
				i, p.failN, p.waiting, p.playing, p.err)
		}
	}
	if !done() {
		t.Fatalf("not there after %d posts", n)
	}
}

// A song that couldn't load for a moment is asked for again, in place: it
// used to give way to the next one, and the listener heard that instead, or
// nothing at all, as the queue went by.
func TestFailedTrackIsAskedForAgain(t *testing.T) {
	p, ch := retryQueue(t, &flaky{fails: 2}, atOnce)
	runPosts(t, p, ch, 30, func() bool { return p.out != nil })
	if id := p.Current().ID; id != "a" || !p.Playing() || p.Err() != "" {
		t.Fatalf("on %s, playing %v, err %q", id, p.Playing(), p.Err())
	}
}

// One that goes on failing in sight is let go of after a few asks, so that a
// song that's gone doesn't hold the queue up for good.
func TestFailedTrackIsLetGoInSight(t *testing.T) {
	src := &flaky{fails: 1 << 30}
	p, ch := retryQueue(t, src, atOnce)
	runPosts(t, p, ch, 40, func() bool { return !p.Playing() })
	if id := p.Current().ID; id != "a" || p.Playing() || p.Err() == "" {
		t.Fatalf("on %s, playing %v, err %q", id, p.Playing(), p.Err())
	}
	if src.asks <= len(retryDelays) {
		t.Errorf("asked for %d times, fewer than it waited", src.asks)
	}
}

// Out of sight, a song that can't load is waited for rather than skipped: the
// listener can't see the queue go by, and a pocket loses the network for a
// while, not for good.
func TestHiddenWaitsThroughFailures(t *testing.T) {
	p, ch := retryQueue(t, &flaky{fails: 1 << 30}, atOnce)
	p.Hidden = func() bool { return true }
	deadline := time.After(2 * time.Second)
	for p.failN <= len(retryDelays)+2 {
		select {
		case fn := <-ch:
			fn()
		case <-deadline:
			t.Fatalf("nothing posted, %d asked for", p.failN)
		}
	}
	if id := p.Current().ID; id != "a" || !p.Playing() || !p.waiting {
		t.Fatalf("on %s, playing %v, waiting %v", id, p.Playing(), p.waiting)
	}
}

// stuck never answers its first ask for a song's stream, as a request that
// went with a phone's screen off does; the next asks play from disk.
type stuck struct {
	nothing
	began   atomic.Bool
	asks    atomic.Int32
	release chan struct{}
}

func (s *stuck) Stream(string, string) (*music.Stream, error) {
	if s.asks.Add(1) == 1 {
		s.began.Store(true)
		<-s.release
		return nil, errors.New("the ask went with the page")
	}
	path, err := filepath.Abs("testdata/seek.mp3")
	return &music.Stream{URL: "file://" + filepath.ToSlash(path), Type: "mp3"}, err
}

// Back in sight, a load that's still waiting for its source is started over:
// its ask may have gone with the page, never to answer, and the song would
// have stayed buffering with nothing asking for it again.
func TestRetryNowStartsAStuckLoadOver(t *testing.T) {
	src := &stuck{release: make(chan struct{})}
	t.Cleanup(func() { close(src.release) })
	ch := make(chan func(), 64)
	p := New(func(fn func()) { ch <- fn })
	p.SetVolume(0)
	p.Source = func(string) music.Source { return src }
	p.after = atOnce
	t.Cleanup(p.stop)
	p.PlayQueue([]music.Track{{ID: "a", Source: "s"}}, 0)
	eventually(t, "the load didn't begin", src.began.Load)
	if !p.Loading() || p.out != nil {
		t.Fatalf("loading %v, out %v", p.Loading(), p.out != nil)
	}
	p.RetryNow()
	runPosts(t, p, ch, 10, func() bool { return p.out != nil })
	if !p.Playing() {
		t.Fatal("not playing")
	}
}

// Back in sight, what waits for its wait is asked for at once: the network
// may be back with the app.
func TestRetryNowAsksAtOnce(t *testing.T) {
	var waits []func()
	p, ch := retryQueue(t, &flaky{fails: 1}, func(_ time.Duration, fn func()) { waits = append(waits, fn) })
	runPosts(t, p, ch, 10, func() bool { return p.waiting })
	if len(waits) != 1 {
		t.Fatalf("waits %d times", len(waits))
	}
	p.RetryNow()
	runPosts(t, p, ch, 10, func() bool { return p.out != nil })
	if !p.Playing() || p.Err() != "" {
		t.Fatalf("playing %v, err %q", p.Playing(), p.Err())
	}
}
