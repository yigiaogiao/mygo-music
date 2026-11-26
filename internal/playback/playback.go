// Package playback is what one listener's devices play together: the state
// the server keeps, the reports the device playing sends it, and what another
// device reads to pick the music up where it was.
package playback

import (
	"time"

	"mygo-music/internal/music"
)

// The actions a report asks for.
const (
	// ActionReport tells the server where this device is in what it plays.
	ActionReport = "report"
	// ActionClaim makes this device the one playing, from where the session
	// is, or from where the report says it now is.
	ActionClaim = "claim"
	// ActionTakeover asks another device to pick the session up.
	ActionTakeover = "takeover"
	// ActionCancel calls off a takeover asked for either side.
	ActionCancel = "cancel"
)

// freshFor is how long after a report from the device playing its position
// is still taken to run on by the clock. A device that has not reported for
// longer may have stalled, and its last position stands as it was.
const freshFor = 5 * time.Second

// A Track is what a device says plays, in the few words another device needs
// to show it before it has the queue.
type Track struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Artist   string `json:"artist,omitempty"`
	Album    string `json:"album,omitempty"`
	Duration int64  `json:"duration,omitempty"` // milliseconds
}

// A Device is one the listener has used this server from.
type Device struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Kind   string `json:"kind,omitempty"` // phone, tablet or computer
	Online bool   `json:"online,omitempty"`
	Active bool   `json:"active,omitempty"` // the device playing
}

// A State is what the server answers a device: the session, and the devices
// that may pick it up.
type State struct {
	// DeviceID is the device that asked, as the server knows it.
	DeviceID string `json:"deviceId,omitempty"`

	Owner       string `json:"owner,omitempty"`
	OwnerName   string `json:"ownerName,omitempty"`
	OwnerOnline bool   `json:"ownerOnline,omitempty"`
	Playing     bool   `json:"playing,omitempty"`

	Track    *Track `json:"track,omitempty"`
	Position int64  `json:"position"`           // milliseconds, at UpdatedAt
	Duration int64  `json:"duration,omitempty"` // milliseconds
	UpdatedAt int64 `json:"updatedAt,omitempty"` // Unix milliseconds

	// The queue, as the device playing last sent it, for another device to
	// go on with the same songs.
	Queue   []music.Track `json:"queue,omitempty"`
	Order   []int         `json:"order,omitempty"`
	At      int           `json:"at,omitempty"`
	Shuffle bool          `json:"shuffle,omitempty"`
	Repeat  int           `json:"repeat,omitempty"`

	// Pending is the device asked to pick the session up, and PendingFrom
	// the device that asked it to.
	Pending         string `json:"pending,omitempty"`
	PendingName     string `json:"pendingName,omitempty"`
	PendingFrom     string `json:"pendingFrom,omitempty"`
	PendingFromName string `json:"pendingFromName,omitempty"`

	Devices []Device `json:"devices,omitempty"`
	// Now is the server's clock, for the app to place what it reads in time.
	Now int64 `json:"now,omitempty"`
}

// A Report is what a device tells the server, and asks it.
type Report struct {
	Action string `json:"action"`
	Track  *Track `json:"track,omitempty"`

	Position int64 `json:"position"`
	Duration int64 `json:"duration,omitempty"`
	Playing  bool  `json:"playing,omitempty"`

	Queue   []music.Track `json:"queue,omitempty"`
	Order   []int         `json:"order,omitempty"`
	At      int           `json:"at,omitempty"`
	Shuffle bool          `json:"shuffle,omitempty"`
	Repeat  int           `json:"repeat,omitempty"`

	// Target is the device a takeover asks to pick the session up.
	Target string `json:"target,omitempty"`
}

// Mine reports whether this device is the one playing, or last played.
func (s State) Mine() bool {
	return s.DeviceID != "" && s.Owner == s.DeviceID
}

// PendingForMe reports whether another device asked this one to pick the
// session up.
func (s State) PendingForMe() bool {
	return s.DeviceID != "" && s.Pending == s.DeviceID && s.Track != nil
}

// LivePosition is where the session is at now: the position reported, run on
// by the clock while the device playing is still reporting.
func (s State) LivePosition(now time.Time) int64 {
	pos := s.Position
	if !s.Playing || !s.OwnerOnline || s.UpdatedAt == 0 {
		return pos
	}
	elapsed := now.UnixMilli() - s.UpdatedAt
	if elapsed <= 0 {
		return pos
	}
	if elapsed > freshFor.Milliseconds() {
		elapsed = freshFor.Milliseconds()
	}
	pos += elapsed
	if s.Duration > 0 && pos > s.Duration {
		pos = s.Duration
	}
	return pos
}

// Same reports whether two states would show the same, ignoring where in the
// song the first is, which moves every second.
func (s State) Same(o State) bool {
	if s.DeviceID != o.DeviceID || s.Owner != o.Owner || s.OwnerName != o.OwnerName ||
		s.Playing != o.Playing || s.Duration != o.Duration ||
		s.Pending != o.Pending || s.PendingName != o.PendingName ||
		s.PendingFrom != o.PendingFrom || s.PendingFromName != o.PendingFromName {
		return false
	}
	if !sameTrack(s.Track, o.Track) || s.At != o.At || s.Shuffle != o.Shuffle || s.Repeat != o.Repeat {
		return false
	}
	if len(s.Devices) != len(o.Devices) {
		return false
	}
	for i := range s.Devices {
		a, b := s.Devices[i], o.Devices[i]
		if a.ID != b.ID || a.Name != b.Name || a.Kind != b.Kind || a.Online != b.Online || a.Active != b.Active {
			return false
		}
	}
	return true
}

func sameTrack(a, b *Track) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.ID == b.ID && a.Name == b.Name && a.Artist == b.Artist && a.Album == b.Album && a.Duration == b.Duration
}

// ValidQueue reports whether q, ordered by order and playing order index at,
// is a queue another device can pick up.
func ValidQueue(q []music.Track, order []int, at int) bool {
	n := len(q)
	if n == 0 || len(order) != n || at < 0 || at >= n {
		return false
	}
	seen := make([]bool, n)
	for _, i := range order {
		if i < 0 || i >= n || seen[i] {
			return false
		}
		seen[i] = true
	}
	return true
}
