package server

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"mygo-music/internal/music"
	"mygo-music/internal/playback"
)

const (
	// playbackOnlineFor is how long after a device's last request it still
	// counts as open.
	playbackOnlineFor = 15 * time.Second
	// playbackPendingFor is how long a device asked to pick the session up
	// waits for it to open the app.
	playbackPendingFor = 10 * time.Minute
	// playbackDeviceFor is how long a device stays in the list without any
	// request.
	playbackDeviceFor = 180 * 24 * time.Hour
	// playbackCookie names the device of a browser that the gateway does
	// not name.
	playbackCookie = "mygo_music_device"
	// playbackDevices is how many devices the list keeps, the last seen.
	playbackDevices = 32
)

// An entry is one device in the list, and when it was last heard from.
type playbackDevice struct {
	name string
	kind string
	seen time.Time
}

// playbackHub keeps what one listener's devices play, and which of them are
// around to play it.
type playbackHub struct {
	mu  sync.Mutex
	now func() time.Time
	// path is where the devices are kept, to know a phone that has not been
	// opened since the server started.
	path    string
	saved   []byte
	devices map[string]*playbackDevice

	owner    string
	track    *playback.Track
	position int64
	duration int64
	playing  bool
	updated  time.Time
	queue    []music.Track
	order    []int
	at       int
	shuffle  bool
	repeat   int

	pending      string
	pendingFrom  string
	pendingSince time.Time
}

type savedPlayback struct {
	Devices []savedPlaybackDevice `json:"devices"`
}

type savedPlaybackDevice struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind,omitempty"`
}

// newPlaybackHub keeps what the listener's devices play, under dataDir.
func newPlaybackHub(dataDir string) *playbackHub {
	h := &playbackHub{
		now:     time.Now,
		path:    filepath.Join(dataDir, "playback.json"),
		devices: map[string]*playbackDevice{},
	}
	h.load()
	return h
}

func (h *playbackHub) load() {
	b, err := os.ReadFile(h.path)
	if err != nil {
		return
	}
	var saved savedPlayback
	if json.Unmarshal(b, &saved) != nil {
		return
	}
	for _, d := range saved.Devices {
		if d.ID == "" {
			continue
		}
		h.devices[d.ID] = &playbackDevice{name: d.Name, kind: d.Kind}
	}
	h.saved = b
}

// saveLocked keeps the devices, which outlive a restart, rather than the
// session, which the devices tell the server again as they play.
func (h *playbackHub) saveLocked() {
	saved := savedPlayback{Devices: make([]savedPlaybackDevice, 0, len(h.devices))}
	for _, id := range h.sortedDevicesLocked() {
		d := h.devices[id]
		saved.Devices = append(saved.Devices, savedPlaybackDevice{ID: id, Name: d.name, Kind: d.kind})
	}
	b, err := json.Marshal(saved)
	if err != nil || bytes.Equal(b, h.saved) {
		return
	}
	tmp := h.path + ".part"
	if os.WriteFile(tmp, b, 0o600) == nil && os.Rename(tmp, h.path) == nil {
		h.saved = b
	}
}

func (h *playbackHub) sortedDevicesLocked() []string {
	ids := make([]string, 0, len(h.devices))
	for id := range h.devices {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		a, b := h.devices[ids[i]], h.devices[ids[j]]
		if !a.seen.Equal(b.seen) {
			return a.seen.After(b.seen)
		}
		return ids[i] < ids[j]
	})
	return ids
}

// touchLocked notes a device's request, adding it to the list the first time.
func (h *playbackHub) touchLocked(id, name, kind string, now time.Time) {
	if id == "" {
		return
	}
	d := h.devices[id]
	if d == nil {
		if len(h.devices) >= playbackDevices {
			ids := h.sortedDevicesLocked()
			delete(h.devices, ids[len(ids)-1])
		}
		h.devices[id] = &playbackDevice{name: name, kind: kind, seen: now}
		h.saveLocked()
		return
	}
	if name != "" && d.name != name {
		d.name = name
		h.saveLocked()
	}
	if kind != "" && d.kind != kind {
		d.kind = kind
		h.saveLocked()
	}
	d.seen = now
}

// expireLocked drops what has waited too long: a takeover no one answered,
// and devices not used for months.
func (h *playbackHub) expireLocked(now time.Time) {
	if h.pending != "" && now.Sub(h.pendingSince) > playbackPendingFor {
		h.pending, h.pendingFrom, h.pendingSince = "", "", time.Time{}
	}
	for id, d := range h.devices {
		if !d.seen.IsZero() && now.Sub(d.seen) > playbackDeviceFor {
			delete(h.devices, id)
		}
	}
}

// liveLocked is where the session is at now, the device playing having run
// on by the clock since it last reported.
func (h *playbackHub) liveLocked(now time.Time) int64 {
	online := false
	if d := h.devices[h.owner]; d != nil && !d.seen.IsZero() && now.Sub(d.seen) <= playbackOnlineFor {
		online = true
	}
	return playback.State{
		Playing:     h.playing,
		OwnerOnline: online,
		Position:    h.position,
		Duration:    h.duration,
		UpdatedAt:   unixMilli(h.updated),
	}.LivePosition(now)
}

func (h *playbackHub) nameLocked(id string) string {
	if d := h.devices[id]; d != nil && d.name != "" {
		return d.name
	}
	return "Device"
}

// stateLocked is what the server answers a device: the session, and the
// devices that may pick it up.
func (h *playbackHub) stateLocked(device string, now time.Time) playback.State {
	h.expireLocked(now)
	st := playback.State{
		DeviceID:    device,
		Owner:       h.owner,
		Playing:     h.playing,
		Track:       h.track,
		Position:    h.liveLocked(now),
		Duration:    h.duration,
		UpdatedAt:   unixMilli(h.updated),
		Queue:       h.queue,
		Order:       h.order,
		At:          h.at,
		Shuffle:     h.shuffle,
		Repeat:      h.repeat,
		Pending:     h.pending,
		PendingFrom: h.pendingFrom,
		Now:         now.UnixMilli(),
	}
	if h.owner != "" {
		st.OwnerName = h.nameLocked(h.owner)
	}
	if d := h.devices[h.owner]; d != nil && !d.seen.IsZero() && now.Sub(d.seen) <= playbackOnlineFor {
		st.OwnerOnline = true
	}
	if h.pending != "" {
		st.PendingName = h.nameLocked(h.pending)
		st.PendingFromName = h.nameLocked(h.pendingFrom)
	}

	labels := map[string]int{}
	for _, d := range h.devices {
		labels[deviceLabel(d.name)]++
	}
	for _, id := range h.sortedDevicesLocked() {
		d := h.devices[id]
		name := deviceLabel(d.name)
		if labels[name] > 1 {
			name += " · " + shortDeviceID(id)
		}
		st.Devices = append(st.Devices, playback.Device{
			ID:     id,
			Name:   name,
			Kind:   d.kind,
			Online: !d.seen.IsZero() && now.Sub(d.seen) <= playbackOnlineFor,
			Active: id == h.owner,
		})
	}
	sort.SliceStable(st.Devices, func(i, j int) bool {
		a, b := st.Devices[i], st.Devices[j]
		if a.Active != b.Active {
			return a.Active
		}
		if a.Online != b.Online {
			return a.Online
		}
		return a.Name < b.Name
	})
	return st
}

func deviceLabel(name string) string {
	if name == "" {
		return "Device"
	}
	return name
}

func shortDeviceID(id string) string {
	if len(id) <= 4 {
		return id
	}
	return id[len(id)-4:]
}

func unixMilli(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixMilli()
}

func clampPosition(pos, duration int64) int64 {
	if pos < 0 {
		return 0
	}
	if duration > 0 && pos > duration {
		return duration
	}
	return pos
}

// apply runs what a device reports or asks, and answers it the state.
func (h *playbackHub) apply(device, name, kind string, rep playback.Report) (playback.State, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	now := h.now()
	h.touchLocked(device, name, kind, now)

	var err error
	switch rep.Action {
	case playback.ActionReport:
		h.reportLocked(device, rep, now)
	case playback.ActionClaim:
		err = h.claimLocked(device, rep, now)
	case playback.ActionTakeover:
		err = h.takeoverLocked(device, rep.Target, now)
	case playback.ActionCancel:
		h.cancelLocked(device)
	default:
		err = fmt.Errorf("unknown action %q", rep.Action)
	}
	return h.stateLocked(device, now), err
}

// reportLocked takes where the device playing is. A device that is not the
// one playing tells the server nothing about the session, but learns from the
// answer that it should stop.
func (h *playbackHub) reportLocked(device string, rep playback.Report, now time.Time) {
	if h.owner == "" && rep.Playing && rep.Track != nil {
		h.owner = device
	}
	if h.owner != device {
		return
	}
	if rep.Track != nil {
		h.track = rep.Track
		if rep.Duration > 0 {
			h.duration = rep.Duration
		} else if rep.Track.Duration > 0 {
			h.duration = rep.Track.Duration
		}
		if rep.Queue != nil && playback.ValidQueue(rep.Queue, rep.Order, rep.At) {
			h.queue, h.order, h.at = rep.Queue, rep.Order, rep.At
			h.shuffle, h.repeat = rep.Shuffle, rep.Repeat
		}
	}
	h.playing = rep.Playing
	h.position = clampPosition(rep.Position, h.duration)
	h.updated = now
}

// claimLocked makes the device the one playing: from the session as it is, if
// it picked it up from another device, or from what it reports, if it began a
// song of its own.
func (h *playbackHub) claimLocked(device string, rep playback.Report, now time.Time) error {
	if rep.Track == nil && h.track == nil {
		return errors.New("nothing to play")
	}
	if rep.Track != nil {
		h.track = rep.Track
		if rep.Duration > 0 {
			h.duration = rep.Duration
		} else if rep.Track.Duration > 0 {
			h.duration = rep.Track.Duration
		}
		h.position = clampPosition(rep.Position, h.duration)
	} else {
		h.position = h.liveLocked(now)
	}
	if rep.Queue != nil && playback.ValidQueue(rep.Queue, rep.Order, rep.At) {
		h.queue, h.order, h.at = rep.Queue, rep.Order, rep.At
		h.shuffle, h.repeat = rep.Shuffle, rep.Repeat
	}
	h.owner = device
	h.playing = true
	h.updated = now
	h.pending, h.pendingFrom, h.pendingSince = "", "", time.Time{}
	return nil
}

// takeoverLocked asks target to pick the session up. The device playing goes
// on until target says it has it.
func (h *playbackHub) takeoverLocked(device, target string, now time.Time) error {
	if target == "" || target == device {
		return errors.New("no other device")
	}
	if h.owner != device {
		return errors.New("this device is not playing")
	}
	if h.track == nil {
		return errors.New("nothing is playing")
	}
	if h.devices[target] == nil {
		return errors.New("unknown device")
	}
	h.pending, h.pendingFrom, h.pendingSince = target, device, now
	return nil
}

func (h *playbackHub) cancelLocked(device string) {
	if device == h.pending || device == h.pendingFrom || device == h.owner {
		h.pending, h.pendingFrom, h.pendingSince = "", "", time.Time{}
	}
}

// snapshot is the state for the device that asked, by its ID alone.
func (h *playbackHub) snapshot(device string) playback.State {
	h.mu.Lock()
	defer h.mu.Unlock()
	now := h.now()
	h.touchLocked(device, "", "", now)
	return h.stateLocked(device, now)
}

// playbackName makes out what a browser says it is, for the device list.
func playbackName(userAgent string) (name, kind string) {
	ua := strings.ToLower(userAgent)
	switch {
	case strings.Contains(ua, "iphone"):
		return "iPhone", "phone"
	case strings.Contains(ua, "ipad"):
		return "iPad", "tablet"
	case strings.Contains(ua, "android"):
		if strings.Contains(ua, "mobile") {
			return "Android", "phone"
		}
		return "Android", "tablet"
	case strings.Contains(ua, "cros"):
		return "Chromebook", "computer"
	case strings.Contains(ua, "macintosh") || strings.Contains(ua, "mac os x"):
		return "Mac", "computer"
	case strings.Contains(ua, "windows"):
		return "Windows", "computer"
	case strings.Contains(ua, "linux"):
		return "Linux", "computer"
	}
	return "Device", ""
}

// deviceID is the device a request comes from: the gateway names it, and a
// browser it does not name gets a cookie of its own.
func (h *playbackHub) deviceID(r *http.Request, w http.ResponseWriter) string {
	id := strings.TrimSpace(r.Header.Get("X-HC-Device-ID"))
	if id == "" {
		id = strings.TrimSpace(r.Header.Get("X-MyGo-Device"))
	}
	if id == "" {
		id = strings.TrimSpace(r.URL.Query().Get("device"))
	}
	if id == "" {
		if c, err := r.Cookie(playbackCookie); err == nil {
			id = strings.TrimSpace(c.Value)
		}
	}
	if id == "" {
		id = newDeviceID()
		http.SetCookie(w, &http.Cookie{
			Name:     playbackCookie,
			Value:    id,
			Path:     "/",
			MaxAge:   365 * 24 * 60 * 60,
			SameSite: http.SameSiteLaxMode,
		})
	}
	if len(id) > 128 {
		id = id[:128]
	}
	return id
}

func newDeviceID() string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("device-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

// playback serves what the listener's devices play together: a GET for the
// state, a POST for a report or an ask.
func (s *Server) playback(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		device := s.pb.deviceID(r, w)
		writeJSON(w, http.StatusOK, s.pb.snapshot(device))
	case http.MethodPost:
		if !sameOrigin(r) {
			http.Error(w, "cross-origin request refused", http.StatusForbidden)
			return
		}
		body := http.MaxBytesReader(w, r.Body, 8<<20)
		defer body.Close()
		var rep playback.Report
		if err := json.NewDecoder(body).Decode(&rep); err != nil {
			http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
			return
		}
		device := s.pb.deviceID(r, w)
		name, kind := playbackName(r.UserAgent())
		st, err := s.pb.apply(device, name, kind, rep)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, st)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
