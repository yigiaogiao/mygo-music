package qqmusic

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"mygo-music/internal/songmatch"
)

func fakeQQ(t *testing.T, songs []song, lyric func(mid string) any) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/soso/fcgi-bin/client_search_cp":
			json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"song": map[string]any{"list": songs},
				},
			})
		case "/lyric/fcgi-bin/fcg_query_lyric_new.fcg":
			json.NewEncoder(w).Encode(lyric(r.URL.Query().Get("songmid")))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	c := New()
	c.BaseURL = srv.URL
	c.HTTP = srv.Client()
	return c
}

func mid(m string) song {
	return song{
		SongMid: m, SongName: "Song", Interval: 180,
		AlbumName: "Record",
		Singer:    []struct{ Name string }{{Name: "Band"}},
	}
}

func lyricJSON(lrc string) map[string]any {
	return map[string]any{"lyric": lrc, "retcode": 0}
}

func TestLookupFindsTheSong(t *testing.T) {
	c := fakeQQ(t,
		[]song{mid("001")},
		func(m string) any {
			if m != "001" {
				t.Errorf("asked for song %s", m)
			}
			return lyricJSON("[00:01.00]words")
		})
	lrc, err := c.Lookup(context.Background(), songmatch.Track{Title: "Song", Artist: "Band", Duration: 3 * time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if lrc != "[00:01.00]words" {
		t.Errorf("lyrics %q", lrc)
	}
}

func TestLookupSkipsTheInstrumentalFirst(t *testing.T) {
	second := mid("002")
	second.SongName = "Song (Live)"
	c := fakeQQ(t,
		[]song{mid("001"), second},
		func(m string) any {
			if m == "001" {
				return lyricJSON("")
			}
			return lyricJSON("[00:02.00]right")
		})
	lrc, err := c.Lookup(context.Background(), songmatch.Track{Title: "Song", Artist: "Band", Duration: 3 * time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if lrc != "[00:02.00]right" {
		t.Errorf("lyrics %q", lrc)
	}
}

func TestLookupRejectsPlainWords(t *testing.T) {
	c := fakeQQ(t,
		[]song{mid("001")},
		func(m string) any { return lyricJSON("plain words\nwithout time") })
	lrc, err := c.Lookup(context.Background(), songmatch.Track{Title: "Song", Artist: "Band"})
	if err != nil {
		t.Fatal(err)
	}
	if lrc != "" {
		t.Errorf("lyrics %q", lrc)
	}
}

func TestLookupIgnoresAnotherSong(t *testing.T) {
	other := mid("001")
	other.SongName = "Another Song"
	other.Singer[0].Name = "Nobody"
	other.Interval = 240
	c := fakeQQ(t,
		[]song{other},
		func(m string) any {
			t.Error("asked for the lyrics of another song")
			return nil
		})
	lrc, err := c.Lookup(context.Background(), songmatch.Track{Title: "Song", Artist: "Band", Duration: 3 * time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if lrc != "" {
		t.Errorf("lyrics %q", lrc)
	}
}

func TestLookupAsksInSimplified(t *testing.T) {
	c := fakeQQ(t,
		[]song{{SongMid: "007", SongName: "梵高先生", Interval: 250,
			AlbumName: "梵高先生2005",
			Singer:    []struct{ Name string }{{Name: "李志"}}}},
		func(m string) any { return lyricJSON("[00:01.00]words") })
	lrc, err := c.Lookup(context.Background(), songmatch.Track{Title: "08梵高先生", Artist: "李誌", Duration: 250 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if lrc != "[00:01.00]words" {
		t.Errorf("lyrics %q", lrc)
	}
}
