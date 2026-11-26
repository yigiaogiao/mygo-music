//go:build js && wasm

package player

import (
	"errors"
	"math"
	"strings"
	"syscall/js"
	"time"

	"mygo-music/internal/music"
)

// On a phone the page's own media element plays the music. The browser fetches
// and decodes the file and keeps it playing with the screen off, as it does for
// any audio on a page, and the system's media controls follow it. Decoding the
// sound in Go instead, as a computer does, needs a request per track, which the
// system stops once the screen goes off.
func init() {
	ua := strings.ToLower(js.Global().Get("navigator").Get("userAgent").String())
	// The LazyCat client on a phone gives pages its media session; a browser
	// on Android has none, but keeps a media element going with the screen
	// off just the same. Elsewhere, as on a computer, the app decodes the
	// sound itself, which shows its spectrum as it plays.
	bridge := js.Global().Get("lzc_media_session")
	if !strings.Contains(ua, "android") && (bridge.IsNull() || bridge.IsUndefined()) {
		return
	}
	elementSinkFor = newElementSink
}

// newElementSink returns the page's element playing t.
func newElementSink(src music.Source, t music.Track) (sink, bool) {
	u, ok := src.(interface{ StreamURL(music.Track) string })
	if !ok {
		return nil, false
	}
	url := u.StreamURL(t)
	if url == "" {
		return nil, false
	}
	return playInElement(url), true
}

// playingElement is the page's one media element, which plays one song after
// another: changing its source keeps it the page's audio, which the system
// follows, where a new element each time would start over.
var playingElement *element

// element is the page's media element.
type element struct {
	a         js.Value
	cur       *elementPlaying // the song it plays, nil while stopped
	buffering bool
	playCatch js.Func
	funcs     []js.Func // kept for the page's life: JavaScript holds them
}

// elementPlaying is a song played by the page's element.
type elementPlaying struct {
	el   *element
	end  func()
	fail func(error)
}

// playInElement makes the page's element play url, and returns the sink
// playing it.
func playInElement(url string) *elementPlaying {
	el := playingElement
	if el == nil {
		el = newElement()
		playingElement = el
	}
	h := &elementPlaying{el: el}
	el.cur = h
	el.buffering = true
	el.a.Set("src", url)
	el.a.Call("load")
	return h
}

// newElement makes the page's media element, and hears what it does.
func newElement() *element {
	a := js.Global().Get("Audio").New()
	a.Set("preload", "auto")
	el := &element{a: a}
	el.playCatch = keep(el, js.FuncOf(func(_ js.Value, args []js.Value) any {
		if h := el.cur; h != nil && h.fail != nil {
			msg := "playback failed"
			if len(args) > 0 && args[0].Type() == js.TypeObject {
				if m := args[0].Get("message").String(); m != "" {
					msg = m
				}
			}
			h.fail(errors.New(msg))
		}
		return nil
	}))
	hear(el, "ended", func([]js.Value) {
		if h := el.cur; h != nil && h.end != nil {
			h.end()
		}
	})
	hear(el, "error", func([]js.Value) {
		if h := el.cur; h != nil && h.fail != nil {
			h.fail(elementError(el.a))
		}
	})
	hear(el, "loadstart", func([]js.Value) { el.buffering = true })
	hear(el, "waiting", func([]js.Value) { el.buffering = true })
	hear(el, "canplay", func([]js.Value) { el.buffering = false })
	hear(el, "playing", func([]js.Value) { el.buffering = false })
	hear(el, "seeked", func([]js.Value) { el.buffering = false })
	return el
}

// hear calls fn when the element says event.
func hear(el *element, event string, fn func(args []js.Value)) {
	f := keep(el, js.FuncOf(func(_ js.Value, args []js.Value) any {
		fn(args)
		return nil
	}))
	el.a.Call("addEventListener", event, f)
}

// keep holds on to f for the element's life, which is the page's: a function
// JavaScript holds goes once Go's garbage collector does, if nothing here does.
func keep(el *element, f js.Func) js.Func {
	el.funcs = append(el.funcs, f)
	return f
}

// elementError says why the element couldn't play the song.
func elementError(a js.Value) error {
	e := a.Get("error")
	if e.IsNull() || e.IsUndefined() {
		return errors.New("the browser couldn't play the song")
	}
	switch e.Get("code").Int() {
	case 3, 4: // it can't decode the file, or doesn't know its format
		return errUnplayable
	}
	if msg := e.Get("message").String(); msg != "" {
		return errors.New(msg)
	}
	return errors.New("the browser couldn't play the song")
}

func (h *elementPlaying) Play() {
	p := h.el.a.Call("play")
	// A rejected play says so as a failed promise, not as an event.
	if p.Type() == js.TypeObject && p.Get("catch").Type() == js.TypeFunction {
		p.Call("catch", h.el.playCatch)
	}
}

func (h *elementPlaying) Pause() { h.el.a.Call("pause") }

func (h *elementPlaying) Playing() bool {
	a := h.el.a
	return !a.Get("paused").Bool() && !a.Get("ended").Bool()
}

func (h *elementPlaying) Seek(t time.Duration) {
	a := h.el.a
	// With no source, as a closed element has, setting currentTime throws:
	// a throw from here would end the whole player.
	if a.Get("networkState").Int() == 0 {
		return
	}
	a.Set("currentTime", math.Max(0, t.Seconds()))
}

// SeekInstant jumps at once, as the element can while it plays: decoded
// audio needs a pause around the jump to keep it from clicking, but the
// pause waits on a timer a page out of sight may not run soon.
func (h *elementPlaying) SeekInstant(t time.Duration) { h.Seek(t) }

func (h *elementPlaying) Position() time.Duration { return seconds(h.el.a.Get("currentTime")) }

func (h *elementPlaying) Duration() time.Duration { return seconds(h.el.a.Get("duration")) }

func (h *elementPlaying) SetVolume(v float64) { h.el.a.Set("volume", math.Max(0, math.Min(1, v))) }

func (h *elementPlaying) OnEnd(fn func()) { h.end = fn }

func (h *elementPlaying) OnError(fn func(error)) { h.fail = fn }

func (h *elementPlaying) Buffering() bool { return h.el.buffering }

// Close lets the element go, unless it plays another song by now: the player
// closes one a moment after it stopped it, by which time the next may play.
func (h *elementPlaying) Close() {
	el := h.el
	if el.cur != h {
		return
	}
	el.cur = nil
	el.buffering = false
	el.a.Call("pause")
	el.a.Call("removeAttribute", "src")
	el.a.Call("load")
}

// seconds reads a time the element gives, in seconds, as a duration.
func seconds(v js.Value) time.Duration {
	f := v.Float()
	if math.IsNaN(f) || math.IsInf(f, 0) || f < 0 {
		return 0
	}
	return time.Duration(f * float64(time.Second))
}
