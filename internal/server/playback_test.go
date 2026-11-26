package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mygo-music/internal/music"
	"mygo-music/internal/playback"
)

func testHub(t *testing.T) (*playbackHub, *time.Time) {
	t.Helper()
	h := newPlaybackHub(t.TempDir())
	now := time.Unix(1_700_000_000, 0)
	h.now = func() time.Time { return now }
	return h, &now
}

func testTrack() *playback.Track {
	return &playback.Track{ID: "t1", Name: "One", Artist: "The Band", Album: "First", Duration: 180_000}
}

func testQueue() []music.Track {
	return []music.Track{
		{ID: "t1", Source: "lazycat", Name: "One", Duration: 180_000},
		{ID: "t2", Source: "lazycat", Name: "Two", Duration: 200_000},
	}
}

func TestPlaybackTransfer(t *testing.T) {
	h, now := testHub(t)

	st, err := h.apply("a", "Mac", "computer", playback.Report{
		Action: playback.ActionClaim, Track: testTrack(), Position: 0, Playing: true,
		Queue: testQueue(), Order: []int{0, 1}, At: 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	if st.Owner != "a" || !st.Playing || st.Track == nil || st.Track.ID != "t1" {
		t.Fatalf("after the claim: %+v", st)
	}

	*now = now.Add(3 * time.Second)
	if got := h.snapshot("a").Position; got != 3000 {
		t.Errorf("position runs on by the clock: got %d, want 3000", got)
	}
	if _, err := h.apply("a", "Mac", "computer", playback.Report{
		Action: playback.ActionReport, Track: testTrack(), Position: 1234, Playing: true,
	}); err != nil {
		t.Fatal(err)
	}
	if got := h.snapshot("a").Position; got != 1234 {
		t.Errorf("position after the report: got %d, want 1234", got)
	}

	// The phone has been used before, and is offline now, but is still listed.
	st = h.snapshot("b")
	if len(st.Devices) != 2 {
		t.Fatalf("devices: %+v", st.Devices)
	}
	var phoneFound bool
	for _, d := range st.Devices {
		if d.ID == "b" {
			phoneFound = true
		}
		if d.ID == "a" && (d.Name != "Mac" || !d.Online || !d.Active) {
			t.Errorf("the Mac: %+v", d)
		}
	}
	if !phoneFound {
		t.Fatalf("the phone is not in the list: %+v", st.Devices)
	}

	st, err = h.apply("a", "Mac", "computer", playback.Report{Action: playback.ActionTakeover, Target: "b"})
	if err != nil {
		t.Fatal(err)
	}
	if st.Owner != "a" || !st.Playing || st.Pending != "b" || st.PendingName != "Device" {
		t.Fatalf("after the takeover: %+v", st)
	}

	st = h.snapshot("b")
	if !st.PendingForMe() || st.PendingFrom != "a" || st.PendingFromName != "Mac" {
		t.Fatalf("the phone's pending: %+v", st)
	}

	*now = now.Add(2 * time.Second)
	pos := st.LivePosition(*now)
	st, err = h.apply("b", "iPhone", "phone", playback.Report{
		Action: playback.ActionClaim, Track: testTrack(), Position: pos, Playing: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if st.Owner != "b" || !st.Playing || st.Pending != "" || st.Position != pos {
		t.Fatalf("after the phone claimed it: %+v", st)
	}
	if len(st.Queue) != 2 || len(st.Order) != 2 || st.At != 0 {
		t.Fatalf("the queue did not come along: %+v", st)
	}

	// The Mac reporting what it still played does not take the session back.
	st, err = h.apply("a", "Mac", "computer", playback.Report{
		Action: playback.ActionReport, Track: testTrack(), Position: 4000, Playing: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if st.Owner != "b" || st.Position != pos {
		t.Fatalf("a stale report took the session: %+v", st)
	}

	// Pausing on the phone leaves the session paused there.
	st, err = h.apply("b", "iPhone", "phone", playback.Report{
		Action: playback.ActionReport, Track: testTrack(), Position: 5000, Playing: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !st.Mine() || st.Playing || st.Position != 5000 {
		t.Fatalf("after the phone paused: %+v", st)
	}
}

func TestPlaybackClaimFromTheList(t *testing.T) {
	h, _ := testHub(t)
	if _, err := h.apply("a", "Mac", "computer", playback.Report{
		Action: playback.ActionClaim, Track: testTrack(), Position: 60_000, Playing: true,
	}); err != nil {
		t.Fatal(err)
	}
	// A device with no queue of its own picks the session up as it is.
	st, err := h.apply("b", "iPhone", "phone", playback.Report{Action: playback.ActionClaim})
	if err != nil {
		t.Fatal(err)
	}
	if st.Owner != "b" || !st.Playing || st.Track == nil || st.Track.ID != "t1" {
		t.Fatalf("claim from the list: %+v", st)
	}
	if st.Position < 60_000 {
		t.Errorf("position: got %d, want at least 60000", st.Position)
	}
}

func TestPlaybackTakeoverChecks(t *testing.T) {
	h, _ := testHub(t)
	if _, err := h.apply("a", "Mac", "computer", playback.Report{
		Action: playback.ActionClaim, Track: testTrack(), Playing: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.apply("a", "Mac", "computer", playback.Report{Action: playback.ActionTakeover, Target: "nobody"}); err == nil {
		t.Error("a takeover of a device not in the list was allowed")
	}
	if _, err := h.apply("b", "iPhone", "phone", playback.Report{Action: playback.ActionTakeover, Target: "a"}); err == nil {
		t.Error("a device not playing asked for a takeover")
	}
}

func TestPlaybackCancel(t *testing.T) {
	h, _ := testHub(t)
	if _, err := h.apply("a", "Mac", "computer", playback.Report{
		Action: playback.ActionClaim, Track: testTrack(), Playing: true,
	}); err != nil {
		t.Fatal(err)
	}
	h.snapshot("b")
	if _, err := h.apply("a", "Mac", "computer", playback.Report{Action: playback.ActionTakeover, Target: "b"}); err != nil {
		t.Fatal(err)
	}
	st, err := h.apply("b", "iPhone", "phone", playback.Report{Action: playback.ActionCancel})
	if err != nil {
		t.Fatal(err)
	}
	if st.Pending != "" {
		t.Fatalf("the phone's cancel did not call it off: %+v", st)
	}
	// And the Mac can ask again, then call it off itself.
	if _, err := h.apply("a", "Mac", "computer", playback.Report{Action: playback.ActionTakeover, Target: "b"}); err != nil {
		t.Fatal(err)
	}
	st, err = h.apply("a", "Mac", "computer", playback.Report{Action: playback.ActionCancel})
	if err != nil {
		t.Fatal(err)
	}
	if st.Pending != "" {
		t.Fatalf("the Mac's cancel did not call it off: %+v", st)
	}
}

func TestPlaybackDevicesOutliveTheServer(t *testing.T) {
	dir := t.TempDir()
	h := newPlaybackHub(dir)
	now := time.Unix(1_700_000_000, 0)
	h.now = func() time.Time { return now }
	if _, err := h.apply("a", "Mac", "computer", playback.Report{
		Action: playback.ActionClaim, Track: testTrack(), Playing: true,
	}); err != nil {
		t.Fatal(err)
	}

	again := newPlaybackHub(dir)
	again.now = func() time.Time { return now.Add(time.Hour) }
	st := again.snapshot("b")
	var found bool
	for _, d := range st.Devices {
		if d.ID == "a" && d.Name == "Mac" && !d.Online {
			found = true
		}
	}
	if !found {
		t.Fatalf("the Mac did not come back from the file: %+v", st.Devices)
	}
}

func TestPlaybackDeviceID(t *testing.T) {
	h := newPlaybackHub(t.TempDir())

	r := httptest.NewRequest(http.MethodGet, "/api/playback", nil)
	w := httptest.NewRecorder()
	first := h.deviceID(r, w)
	if first == "" {
		t.Fatal("no device ID")
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != playbackCookie || cookies[0].Value != first {
		t.Fatalf("cookies: %+v", cookies)
	}

	r = httptest.NewRequest(http.MethodGet, "/api/playback", nil)
	r.AddCookie(cookies[0])
	if again := h.deviceID(r, httptest.NewRecorder()); again != first {
		t.Errorf("the cookie named another device: %q, want %q", again, first)
	}

	r = httptest.NewRequest(http.MethodGet, "/api/playback", nil)
	r.Header.Set("X-HC-Device-ID", "gateway-device")
	if got := h.deviceID(r, httptest.NewRecorder()); got != "gateway-device" {
		t.Errorf("the gateway's device: %q", got)
	}
}

func TestPlaybackHTTP(t *testing.T) {
	s := &Server{pb: newPlaybackHub(t.TempDir())}
	body := `{"action":"claim","track":{"id":"t1","name":"One","duration":180000},` +
		`"position":1000,"playing":true,"queue":[{"id":"t1","name":"One","source":"lazycat"}],` +
		`"order":[0],"at":0}`
	r := httptest.NewRequest(http.MethodPost, "/api/playback", strings.NewReader(body))
	r.Host = "music.example"
	r.Header.Set("Origin", "http://music.example")
	r.Header.Set("X-MyGo-Device", "a")
	w := httptest.NewRecorder()
	s.playback(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	var st playback.State
	if err := json.Unmarshal(w.Body.Bytes(), &st); err != nil {
		t.Fatal(err)
	}
	if st.Owner != "a" || st.Track == nil || !st.Playing {
		t.Fatalf("state: %+v", st)
	}

	r = httptest.NewRequest(http.MethodPost, "/api/playback", strings.NewReader(`{"action":"claim"}`))
	r.Host = "music.example"
	r.Header.Set("Origin", "http://elsewhere.example")
	w = httptest.NewRecorder()
	s.playback(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("cross-origin status %d", w.Code)
	}
}
