//go:build js

package speech

import (
	"context"
	"errors"
	"time"
)

// Browsers have no sherpa-onnx: speech recognition is a desktop feature.

// SampleRate is the rate of the audio the models hear.
const SampleRate = 16000

// A Segment is what is said between two pauses, and when.
type Segment struct {
	Start, End time.Duration
	Text       string
}

// A Source is audio to transcribe: interleaved samples of its channels, at
// its rate.
type Source interface {
	SampleRate() int
	Channels() int
	Read(dst []float32) (int, error)
}

// A Recognizer would transcribe audio; in a browser there is none.
type Recognizer struct{}

// ErrUnavailable says there is no speech recognition for this platform.
var ErrUnavailable = errors.New("speech: no sherpa-onnx for this platform")

// Available reports whether there is speech recognition for this
// platform, to download.
func Available() bool { return false }

// NewRecognizer loads the model and the libraries Download put in root.
func NewRecognizer(root string) (*Recognizer, error) { return nil, ErrUnavailable }

// Close lets the model go.
func (r *Recognizer) Close() {}

// Transcribe transcribes src, which is from into the episode, up to until
// into it, or to its end if until is 0, or until ctx ends.
func (r *Recognizer) Transcribe(ctx context.Context, src Source, from, until time.Duration, said func(Segment), heard func(time.Duration)) error {
	return ErrUnavailable
}

func platform() string { return "js-wasm" }

// libDir returns the directory holding the libraries and the VAD.
func libDir(root string) string { return root }
