package player

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// shorterStalls makes a body that stops coming give up sooner, for a test.
func shorterStalls(t *testing.T) {
	t.Helper()
	old := stallAfter
	stallAfter = 100 * time.Millisecond
	t.Cleanup(func() { stallAfter = old })
}

// A body that stops coming is asked for again from where it stopped, and the
// song is whole once the rest has come: a phone's network may go for a
// moment, and the song used to hang there, buffering, until the app was
// opened again.
func TestDownloadResumesAfterStall(t *testing.T) {
	shorterStalls(t)
	whole := make([]byte, 8<<10)
	for i := range whole {
		whole[i] = byte(i)
	}
	half := len(whole) / 2
	var mu sync.Mutex
	var ranges []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		ranges = append(ranges, r.Header.Get("Range"))
		mu.Unlock()
		from := 0
		if rng := r.Header.Get("Range"); rng != "" {
			n, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(rng, "bytes="), "-"))
			if err != nil {
				t.Errorf("range %q: %v", rng, err)
			}
			from = n
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", from, len(whole)-1, len(whole)))
			w.Header().Set("Content-Length", strconv.Itoa(len(whole)-from))
			w.WriteHeader(http.StatusPartialContent)
		} else {
			w.Header().Set("Content-Length", strconv.Itoa(len(whole)))
		}
		if from == 0 {
			w.Write(whole[:half])
			w.(http.Flusher).Flush()
			// Nothing more comes: the phone's network went.
			<-r.Context().Done()
			return
		}
		w.Write(whole[from:])
	}))
	defer srv.Close()

	d := startDownload(srv.URL, int64(len(whole)), nil, nil, nil)
	defer d.Close()
	eventually(t, "the song didn't come in whole", func() bool { return d.Progress() == 1 })
	if got := d.peek(0, len(whole)); string(got) != string(whole) {
		t.Fatalf("got %d bytes, of %d", len(got), len(whole))
	}
	mu.Lock()
	defer mu.Unlock()
	if len(ranges) < 2 || ranges[0] != "" || ranges[1] != fmt.Sprintf("bytes=%d-", half) {
		t.Fatalf("asked for ranges %v", ranges)
	}
}

// One whose server says nothing at all — no head, no body — fails rather than
// waits for good: a phone's network may go behind a connection that stays
// open, and the song used to buffer on it until the app was opened again.
func TestDownloadGivesUpWhenNoHeadComes(t *testing.T) {
	shorterStalls(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	d := startDownload(srv.URL, 1024, nil, nil, nil)
	defer d.Close()
	if n := d.waitFor(1); n != 0 {
		t.Fatalf("%d bytes came in", n)
	}
	d.mu.Lock()
	err := d.err
	d.mu.Unlock()
	if err == nil {
		t.Fatal("ended with nothing, saying nothing")
	}
}

// One whose body sends nothing at all fails rather than waits for good: the
// player asks for the song again on its own.
func TestDownloadGivesUpWithNothingIn(t *testing.T) {
	shorterStalls(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer srv.Close()

	d := startDownload(srv.URL, 1024, nil, nil, nil)
	defer d.Close()
	if n := d.waitFor(1); n != 0 {
		t.Fatalf("%d bytes came in", n)
	}
	d.mu.Lock()
	err := d.err
	d.mu.Unlock()
	if err == nil {
		t.Fatal("ended with nothing, saying nothing")
	}
}
