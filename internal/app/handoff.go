package app

import (
	"hash/fnv"
	"io"
	"strconv"
	"time"

	"github.com/egoist/mygo"

	"mygo-music/internal/i18n"
	"mygo-music/internal/music"
	"mygo-music/internal/playback"
	"mygo-music/internal/player"
	"mygo-music/internal/websource"
)

// handoff is what this device has of the playback the listener's devices
// share: the session as last heard, and what it is asking the server.
type handoff struct {
	web *websource.Source

	state playback.State
	have  bool
	err   string

	// last is when the server was last asked, and inFlight whether an answer
	// is on its way. asks wait for it, in order.
	last     time.Time
	inFlight bool
	asks     []playback.Report

	// lastSig is the song and whether it plays, as last told: a change is
	// this device starting something, which claims the session.
	lastSig string
	// queueSig is the queue as last sent, not to send it every second.
	queueSig uint64
	// claimUntil is while this device may still be playing after starting on
	// its own, before the server's answer may say another device plays.
	claimUntil time.Time

	pending bool // another device asked this one to go on
	open    bool // the takeover dialog shows
	picking bool // the devices popover shows
	// declined says this device said no to a takeover, which an answer on
	// its way may still tell of. It stays until an answer says no device is
	// asked, so the dialog does not come back for a moment.
	declined bool
}

// attachWebSource gives the browser's source the handoff, which the desktop
// app, with only its own music, has none of.
func (a *app) attachWebSource(src music.Source) {
	if w, ok := src.(*websource.Source); ok {
		a.pb.web = w
	}
}

// startHandoff asks the server about the shared playback, and tells it what
// this device plays, every second or two, as long as the app is open.
func (a *app) startHandoff() {
	if a.pb.web == nil {
		return
	}
	go func() {
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for range t.C {
			mygo.RunOnMain(func() { a.handoffTick() })
		}
	}()
}

// handoffTick reports where playback is, or asks where it is, when due. What
// plays is reported once a second; otherwise every other second, and as soon
// as the listener opens the devices.
func (a *app) handoffTick() {
	h := &a.pb
	if h.web == nil || a.pl == nil || h.inFlight || len(h.asks) > 0 {
		return
	}
	now := time.Now()
	every := 2 * time.Second
	if a.pl.Playing() || h.pending {
		every = time.Second
	}
	if !h.last.IsZero() && now.Sub(h.last) < every {
		return
	}
	h.last = now
	rep, ok := a.handoffReport(now)
	if ok {
		a.askHandoff(rep)
	}
}

// handoffReport makes what this device tells the server: the song, where it
// is, and the queue when it changed. A song started or resumed is a claim,
// which takes the session from whichever device had it.
func (a *app) handoffReport(now time.Time) (playback.Report, bool) {
	h := &a.pb
	playing := a.pl.Playing()
	rep := playback.Report{Action: playback.ActionReport, Playing: playing}
	s, ok := a.pl.Session()
	if !ok {
		h.lastSig = ""
		return rep, true
	}
	tr := a.pl.Current()
	trackID := ""
	if tr != nil {
		rep.Track = trackSummary(tr)
		rep.Duration = tr.Duration
		trackID = tr.ID
	}
	rep.Position = s.Position
	rep.At = s.At
	rep.Shuffle = s.Shuffle
	rep.Repeat = int(s.Repeat)

	sig := sessionSig(s)
	if trackID+"|"+strconv.FormatBool(playing) != h.lastSig {
		h.lastSig = trackID + "|" + strconv.FormatBool(playing)
		if playing && tr != nil {
			rep.Action = playback.ActionClaim
			h.claimUntil = now.Add(5 * time.Second)
		}
	}
	// A claim takes a queue of its own; a report sends it only when it moved.
	if rep.Action == playback.ActionClaim || sig != h.queueSig {
		rep.Queue = append([]music.Track(nil), s.Queue...)
		rep.Order = append([]int(nil), s.Order...)
		h.queueSig = sig
	}
	return rep, true
}

// askHandoff sends a report, one at a time, in the order they were made.
func (a *app) askHandoff(rep playback.Report) {
	h := &a.pb
	if h.web == nil {
		return
	}
	if h.inFlight || len(h.asks) > 0 {
		if len(h.asks) < 8 {
			h.asks = append(h.asks, rep)
		}
		return
	}
	h.inFlight = true
	go func() {
		st, err := h.web.ReportPlayback(rep)
		mygo.RunOnMain(func() { a.handoffDone(st, err) })
	}()
}

// handoffDone takes the server's answer: another device playing takes the
// sound from this one, and an ask for this one opens the dialog to go on.
func (a *app) handoffDone(st *playback.State, err error) {
	h := &a.pb
	h.inFlight = false
	if err != nil {
		h.err = err.Error()
		// The claim did not land: ask again next time, rather than report,
		// which a device that is not playing may not do.
		h.lastSig = ""
	} else {
		h.err = ""
		changed := !h.have || !st.Same(h.state)
		h.state, h.have = *st, true
		if changed {
			a.redraw()
		}
		now := time.Now()
		// Another device began playing: this one stops, as on a phone call.
		if a.pl != nil && a.pl.Playing() && st.Owner != "" && st.Owner != st.DeviceID && st.Playing && now.After(h.claimUntil) {
			a.pl.TogglePlay()
			a.toast = i18n.Tf("Continued on %s", deviceLabel(st.OwnerName))
			a.redraw()
		}
		// This device began playing and the server has not said so yet: its
		// next ask claims it.
		if a.pl != nil && a.pl.Playing() && st.DeviceID != "" && st.Owner != st.DeviceID && now.Before(h.claimUntil) {
			h.lastSig = ""
		}
		if !st.PendingForMe() {
			h.declined = false
		}
		pending := st.PendingForMe() && !h.declined
		if pending != h.pending {
			h.pending = pending
			h.open = pending
			if pending {
				h.picking = false
			}
			a.redraw()
		}
	}
	if len(h.asks) > 0 {
		next := h.asks[0]
		h.asks = h.asks[1:]
		a.askHandoff(next)
	}
}

// requestHandoff asks id to pick the session up. This device goes on until it
// does, so there is no silence in between.
func (a *app) requestHandoff(id, name string) {
	h := &a.pb
	if !h.have || h.state.Owner != h.state.DeviceID || h.state.Track == nil {
		return
	}
	a.askHandoff(playback.Report{Action: playback.ActionTakeover, Target: id})
	h.picking = false
	a.toast = i18n.Tf("Open MyGo Music on %s to continue", deviceLabel(name))
	a.redraw()
}

// cancelHandoff calls off a takeover, from either side, and closes the dialog.
func (a *app) cancelHandoff() {
	h := &a.pb
	if !h.have {
		return
	}
	if h.state.PendingForMe() {
		h.declined = true
	}
	a.askHandoff(playback.Report{Action: playback.ActionCancel})
	h.pending = false
	h.open = false
	a.redraw()
}

// continueHandoff picks the session up here: the same queue, from where the
// other device is, and tells the server this device has it.
func (a *app) continueHandoff() {
	h := &a.pb
	st := h.state
	if st.Track == nil || a.pl == nil {
		return
	}
	now := time.Now()
	pos := st.LivePosition(now)
	s, ok := sessionFromState(st, pos)
	if !ok {
		return
	}
	a.pl.Restore(s)
	a.pl.TogglePlay()

	h.lastSig = st.Track.ID + "|true"
	h.queueSig = sessionSig(s)
	h.claimUntil = now.Add(5 * time.Second)
	tr := *st.Track
	a.askHandoff(playback.Report{
		Action: playback.ActionClaim, Track: &tr, Position: pos, Duration: st.Duration,
		Playing: true, Queue: s.Queue, Order: s.Order, At: s.At, Shuffle: s.Shuffle, Repeat: int(s.Repeat),
	})
	if h.state.PendingForMe() {
		h.declined = true
	}
	h.pending = false
	h.open = false
	h.picking = false
	a.redraw()
}

// refreshHandoff asks the server again as soon as may be, as the devices open.
func (a *app) refreshHandoff() {
	a.pb.last = time.Time{}
}

// handoffReady reports whether this build shares playback with other devices.
func (a *app) handoffReady() bool { return a.pb.web != nil }

// sessionFromState makes the queue another device played into one this app
// can go on with, from pos. Without the queue, the song alone plays.
func sessionFromState(st playback.State, pos int64) (player.Session, bool) {
	if st.Track == nil {
		return player.Session{}, false
	}
	if playback.ValidQueue(st.Queue, st.Order, st.At) {
		return player.Session{
			Queue: st.Queue, Order: st.Order, At: st.At, Shuffle: st.Shuffle,
			Repeat: player.RepeatMode(st.Repeat), Position: pos,
		}, true
	}
	tr := music.Track{ID: st.Track.ID, Source: websource.ID, Name: st.Track.Name, Duration: st.Track.Duration}
	tr.Album.Name = st.Track.Album
	if st.Track.Artist != "" {
		tr.Artists = []music.Artist{{Name: st.Track.Artist}}
	}
	return player.Session{Queue: []music.Track{tr}, Order: []int{0}, At: 0, Repeat: player.RepeatAll, Position: pos}, true
}

// trackSummary is a song in the few words another device needs before it has
// the queue.
func trackSummary(t *music.Track) *playback.Track {
	if t == nil {
		return nil
	}
	return &playback.Track{
		ID: t.ID, Name: t.Name, Artist: t.ArtistNames(), Album: t.Album.Name, Duration: t.Duration,
	}
}

// sessionSig says whether a queue and its order are the same as another, for
// a report to tell the server of a new one only.
func sessionSig(s player.Session) uint64 {
	h := fnv.New64a()
	for _, t := range s.Queue {
		io.WriteString(h, t.ID)
		h.Write([]byte{0})
	}
	for _, i := range s.Order {
		io.WriteString(h, strconv.Itoa(i))
		h.Write([]byte{','})
	}
	io.WriteString(h, "|"+strconv.Itoa(s.At)+"|"+strconv.FormatBool(s.Shuffle)+"|"+strconv.Itoa(int(s.Repeat)))
	return h.Sum64()
}

// deviceLabel names a device in the listener's language: the server knows
// phones and computers by their kinds, and a browser only as a device.
func deviceLabel(name string) string {
	if name == "" || name == "Device" {
		return i18n.T("Device")
	}
	return name
}
