package player

import (
	"errors"
	"time"

	"github.com/egoist/mygo/audio"

	"mygo-music/internal/music"
)

// A sink is what plays the current track's sound: on a computer, the audio
// player that decodes what comes in; on a phone, the page's own media element,
// which the browser fetches and plays, keeping it going with the screen off.
type sink interface {
	Play()
	Pause()
	Playing() bool
	Seek(t time.Duration)
	Position() time.Duration
	Duration() time.Duration
	SetVolume(v float64)
	// OnEnd hears each time it plays to the end.
	OnEnd(fn func())
	// OnError hears that it couldn't play what it was given.
	OnError(fn func(error))
	// Buffering reports whether it waits for more of the track.
	Buffering() bool
	Close()
}

// instantSeeker is a sink that jumps at once, as the page's media element
// does, without the pause decoded audio takes to keep the jump from clicking:
// a phone's seek comes from the lock screen, where a timer may wait.
type instantSeeker interface {
	SeekInstant(t time.Duration)
}

// decoded is the audio player as a sink: the sound is decoded here, from what
// the app downloaded, and it fails by ending short rather than saying so.
type decoded struct{ *audio.Player }

func (d decoded) SetVolume(v float64) { d.Player.SetVolume(v) }
func (d decoded) OnEnd(fn func())     { d.Player.OnEnd(fn) }
func (d decoded) OnError(func(error)) {}
func (d decoded) Buffering() bool     { return false }

// errUnplayable is a track the browser's own player couldn't decode, or has no
// support for: it's said in the listener's terms, since it's about the phone.
// It's asked for again, though, as any other failure is: the browser says the
// same of a file it couldn't fetch, as of one it can't decode.
var errUnplayable = errors.New("the browser can't play this file")

// elementSinkFor returns a sink playing t with the page's media element, if
// the page has one: a phone plays its music that way, as any page plays its
// audio, so that the system keeps it going with the screen off. It's nil where
// the app decodes the sound itself.
var elementSinkFor func(src music.Source, t music.Track) (sink, bool)
