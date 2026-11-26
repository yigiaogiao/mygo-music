package websource

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"mygo-music/internal/music"
)

// A folder removed in the settings has to go from the list at once, not at
// the watch's next look a few seconds later.
func TestRemoveFolderShowsAtOnce(t *testing.T) {
	var mu sync.Mutex
	folders := []string{"/music/a"}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/status":
			mu.Lock()
			st := Status{Folders: append([]string(nil), folders...)}
			mu.Unlock()
			json.NewEncoder(w).Encode(st)
		case "/api/call":
			var call struct {
				Method string
			}
			if err := json.NewDecoder(r.Body).Decode(&call); err != nil {
				t.Errorf("call: %v", err)
			}
			if call.Method == "RemoveFolder" {
				mu.Lock()
				folders = nil
				mu.Unlock()
			}
			w.Write([]byte(`{"result":null}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	s := New(srv.URL)
	defer s.Close()
	changed := make(chan struct{}, 1)
	s.OnChange(func() {
		select {
		case changed <- struct{}{}:
		default:
		}
	})

	// Let the first look land, which tells the source what the library is.
	deadline := time.Now().Add(2 * time.Second)
	for len(s.Status().Folders) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := s.Status().Folders; len(got) != 1 {
		t.Fatalf("folders before removing: %v", got)
	}

	if err := s.RemoveFolder("/music/a"); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if got := s.Status().Folders; len(got) != 0 {
		t.Fatalf("folders after removing: %v", got)
	}
	select {
	case <-changed:
	case <-time.After(time.Second):
		t.Fatal("no change told after removing a folder")
	}
}

// A track's URL at the server is plain: the page's media element fetches it
// itself, so a track change is no call.
func TestStreamURL(t *testing.T) {
	s := New("http://127.0.0.1:8090/")
	defer s.Close()
	if got, want := s.StreamURL(music.Track{ID: "a1b/c"}), "http://127.0.0.1:8090/api/track/a1b%2Fc"; got != want {
		t.Errorf("StreamURL = %q, want %q", got, want)
	}
	if got := s.StreamURL(music.Track{}); got != "" {
		t.Errorf("StreamURL of nothing = %q", got)
	}
}
