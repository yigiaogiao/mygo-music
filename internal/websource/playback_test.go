package websource

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"mygo-music/internal/playback"
)

func TestPlaybackKeepsTheDeviceTheServerNamed(t *testing.T) {
	var mu sync.Mutex
	var seen, reports []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/status" {
			json.NewEncoder(w).Encode(Status{})
			return
		}
		if r.URL.Path != "/api/playback" {
			http.NotFound(w, r)
			return
		}
		mu.Lock()
		seen = append(seen, r.Header.Get("X-MyGo-Device"))
		mu.Unlock()
		device := r.Header.Get("X-MyGo-Device")
		if device == "" {
			device = "named-by-the-server"
		}
		rep := playback.Report{}
		if r.Method == http.MethodPost {
			if err := json.NewDecoder(r.Body).Decode(&rep); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			mu.Lock()
			reports = append(reports, rep.Action)
			mu.Unlock()
		}
		json.NewEncoder(w).Encode(playback.State{
			DeviceID: device,
			Owner:    device,
			Playing:  rep.Playing,
			Position: rep.Position,
		})
	}))
	defer ts.Close()

	s := New(ts.URL)
	defer s.Close()

	st, err := s.Playback()
	if err != nil {
		t.Fatal(err)
	}
	if st.DeviceID != "named-by-the-server" {
		t.Fatalf("device: %q", st.DeviceID)
	}
	st, err = s.ReportPlayback(playback.Report{Action: playback.ActionClaim, Position: 1234, Playing: true})
	if err != nil {
		t.Fatal(err)
	}
	if st.DeviceID != "named-by-the-server" || st.Position != 1234 {
		t.Fatalf("state: %+v", st)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 2 || seen[0] != "" || seen[1] != "named-by-the-server" {
		t.Errorf("device headers: %q", seen)
	}
	if len(reports) != 1 || reports[0] != playback.ActionClaim {
		t.Errorf("reports: %q", reports)
	}
}
