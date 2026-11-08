package lrclib

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func fakeLRCLib(t *testing.T, get func(track Track) (int, any), search func(track Track) []Result) (*Client, *int32) {
	t.Helper()
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		q := r.URL.Query()
		track := Track{
			Title:  q.Get("track_name"),
			Artist: q.Get("artist_name"),
			Album:  q.Get("album_name"),
		}
		if q.Get("duration") != "" {
			var secs int
			json.Unmarshal([]byte(q.Get("duration")), &secs)
			track.Duration = time.Duration(secs) * time.Second
		}
		switch r.URL.Path {
		case "/api/get":
			code, body := get(track)
			if code != http.StatusOK {
				w.WriteHeader(code)
				return
			}
			json.NewEncoder(w).Encode(body)
		case "/api/search":
			json.NewEncoder(w).Encode(search(track))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	c := New()
	c.BaseURL = srv.URL
	c.HTTP = srv.Client()
	return c, &calls
}

func TestLookupUsesGet(t *testing.T) {
	c, _ := fakeLRCLib(t,
		func(tr Track) (int, any) {
			if tr.Title != "Song" || tr.Artist != "Band" || tr.Duration != 3*time.Minute {
				t.Errorf("get asked with %+v", tr)
			}
			return http.StatusOK, Result{
				TrackName: "Song", ArtistName: "Band", Duration: 180,
				SyncedLyrics: "[00:01.00]words",
			}
		},
		func(tr Track) []Result {
			t.Error("search used after get answered")
			return nil
		})
	lrc, err := c.Lookup(context.Background(), Track{Title: "Song", Artist: "Band", Album: "Record", Duration: 3 * time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if lrc != "[00:01.00]words" {
		t.Errorf("lyrics %q", lrc)
	}
}

func TestLookupFallsBackToSearch(t *testing.T) {
	c, _ := fakeLRCLib(t,
		func(tr Track) (int, any) { return http.StatusNotFound, nil },
		func(tr Track) []Result {
			return []Result{
				{TrackName: "Song (Live)", ArtistName: "Other", Duration: 180, SyncedLyrics: "[00:01.00]wrong"},
				{TrackName: "Song", ArtistName: "Band", AlbumName: "Record", Duration: 181, SyncedLyrics: "[00:02.00]right"},
			}
		})
	lrc, err := c.Lookup(context.Background(), Track{Title: "Song", Artist: "Band", Album: "Record", Duration: 3 * time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if lrc != "[00:02.00]right" {
		t.Errorf("picked %q", lrc)
	}
}

func TestLookupNothing(t *testing.T) {
	c, _ := fakeLRCLib(t,
		func(tr Track) (int, any) { return http.StatusNotFound, nil },
		func(tr Track) []Result {
			return []Result{{TrackName: "Something Else", ArtistName: "Nobody", Duration: 300, SyncedLyrics: "[00:01.00]no"}}
		})
	lrc, err := c.Lookup(context.Background(), Track{Title: "Song", Artist: "Band", Duration: 3 * time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if lrc != "" {
		t.Errorf("should find nothing, got %q", lrc)
	}
}

func TestLookupSearchesWhenOnlyTheTitleIsKnown(t *testing.T) {
	// /api/get wants an artist and an album too, and answers 400 without
	// them; a WAV has only its name, and search finds it.
	c, calls := fakeLRCLib(t,
		func(tr Track) (int, any) {
			t.Error("get used without an artist and an album")
			return http.StatusBadRequest, nil
		},
		func(tr Track) []Result {
			if tr.Title != "七里香" {
				t.Errorf("search asked for %q", tr.Title)
			}
			return []Result{{TrackName: "七里香", ArtistName: "Jay Chou", Duration: 297, SyncedLyrics: "[00:01.00]words"}}
		})
	lrc, err := c.Lookup(context.Background(), Track{Title: "七里香", Duration: 297 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if lrc != "[00:01.00]words" {
		t.Errorf("lyrics %q", lrc)
	}
	if n := atomic.LoadInt32(calls); n != 1 {
		t.Errorf("asked %d times, want 1", n)
	}
}

func TestLookupReadsADecimalDuration(t *testing.T) {
	c, _ := fakeLRCLib(t,
		func(tr Track) (int, any) { return http.StatusNotFound, nil },
		func(tr Track) []Result {
			return []Result{{TrackName: "Song", ArtistName: "Band", Duration: 180.0, SyncedLyrics: "[00:01.00]words"}}
		})
	lrc, err := c.Lookup(context.Background(), Track{Title: "Song", Artist: "Band", Album: "Record", Duration: 3 * time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if lrc != "[00:01.00]words" {
		t.Errorf("lyrics %q", lrc)
	}
}

func TestPickSkipsInstrumentalAndUnsynced(t *testing.T) {
	rs := []Result{
		{TrackName: "Song", ArtistName: "Band", Duration: 180, Instrumental: true, SyncedLyrics: "[00:01.00]x"},
		{TrackName: "Song", ArtistName: "Band", Duration: 180, PlainLyrics: "x"},
	}
	if r := pick(rs, Track{Title: "Song", Artist: "Band", Duration: 3 * time.Minute}); r != nil {
		t.Errorf("picked %+v", r)
	}
}

func TestCacheKeepsFoundLyrics(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c, calls := fakeLRCLib(t,
		func(tr Track) (int, any) {
			return http.StatusOK, Result{TrackName: "Song", ArtistName: "Band", Duration: 180, SyncedLyrics: "[00:01.00]words"}
		},
		func(tr Track) []Result { return nil })
	cache := NewCache(t.TempDir())
	cache.Now = func() time.Time { return now }
	tr := Track{Title: "Song", Artist: "Band", Album: "Record", Duration: 3 * time.Minute}

	for i := 0; i < 2; i++ {
		lrc, err := cache.Lookup(context.Background(), c, tr)
		if err != nil || lrc != "[00:01.00]words" {
			t.Fatalf("lookup %d: %q, %v", i, lrc, err)
		}
	}
	if n := atomic.LoadInt32(calls); n != 1 {
		t.Errorf("asked %d times, want 1", n)
	}

	// Still good at 89 days, stale after 90.
	now = now.Add(89 * 24 * time.Hour)
	if lrc, err := cache.Lookup(context.Background(), c, tr); err != nil || lrc == "" {
		t.Fatalf("at 89 days: %q, %v", lrc, err)
	}
	now = now.Add(2 * 24 * time.Hour)
	cache.Lookup(context.Background(), c, tr)
	if n := atomic.LoadInt32(calls); n != 2 {
		t.Errorf("asked %d times after expiry, want 2", n)
	}
}

func TestCacheKeepsNotFoundBriefly(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c, calls := fakeLRCLib(t,
		func(tr Track) (int, any) { return http.StatusNotFound, nil },
		func(tr Track) []Result { return nil })
	cache := NewCache(t.TempDir())
	cache.Now = func() time.Time { return now }
	tr := Track{Title: "Unknown", Artist: "Nobody", Album: "Record", Duration: 2 * time.Minute}

	for i := 0; i < 2; i++ {
		lrc, err := cache.Lookup(context.Background(), c, tr)
		if err != nil || lrc != "" {
			t.Fatalf("lookup %d: %q, %v", i, lrc, err)
		}
	}
	// A miss asks get, then search.
	if n := atomic.LoadInt32(calls); n != 2 {
		t.Errorf("asked %d times, want 2", n)
	}
	now = now.Add(8 * 24 * time.Hour)
	cache.Lookup(context.Background(), c, tr)
	if n := atomic.LoadInt32(calls); n != 4 {
		t.Errorf("asked %d times after expiry, want 4", n)
	}
}

func TestCacheSurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	c, _ := fakeLRCLib(t,
		func(tr Track) (int, any) {
			return http.StatusOK, Result{TrackName: "Song", ArtistName: "Band", Duration: 180, SyncedLyrics: "[00:01.00]words"}
		},
		func(tr Track) []Result { return nil })
	tr := Track{Title: "Song", Artist: "Band", Album: "Record", Duration: 3 * time.Minute}
	first := NewCache(dir)
	if _, err := first.Lookup(context.Background(), c, tr); err != nil {
		t.Fatal(err)
	}
	// A second cache on the same directory, as after a restart, finds the file.
	second := NewCache(dir)
	lrc, err := second.Lookup(context.Background(), c, tr)
	if err != nil || lrc != "[00:01.00]words" {
		t.Fatalf("after restart: %q, %v", lrc, err)
	}
}
