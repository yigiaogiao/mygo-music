package netease

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"mygo-music/internal/songmatch"
)

func fakeNetease(t *testing.T, songs []song, lyric func(id string) any) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/search/get/web":
			json.NewEncoder(w).Encode(map[string]any{
				"result": map[string]any{"songs": songs},
			})
		case "/api/song/lyric":
			json.NewEncoder(w).Encode(lyric(r.URL.Query().Get("id")))
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

func known(id int64) song {
	return song{
		ID: id, Name: "Song", Duration: 180000,
		Artists: []credit{{Name: "Band"}},
		Album:   albumInfo{Name: "Record"},
	}
}

func lyricJSON(lrc string) map[string]any {
	return map[string]any{
		"lrc": map[string]any{"lyric": lrc},
	}
}

func TestLookupFindsTheSong(t *testing.T) {
	c := fakeNetease(t,
		[]song{known(1)},
		func(id string) any {
			if id != "1" {
				t.Errorf("asked for song %s", id)
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
	second := known(2)
	second.Name = "Song (Live)"
	c := fakeNetease(t,
		[]song{known(1), second},
		func(id string) any {
			if id == "1" {
				return map[string]any{"nolyric": true}
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
	c := fakeNetease(t,
		[]song{known(1)},
		func(id string) any { return lyricJSON("plain words\nwithout time") })
	lrc, err := c.Lookup(context.Background(), songmatch.Track{Title: "Song", Artist: "Band"})
	if err != nil {
		t.Fatal(err)
	}
	if lrc != "" {
		t.Errorf("lyrics %q", lrc)
	}
}

func TestLookupIgnoresAnotherSong(t *testing.T) {
	other := known(1)
	other.Name = "Another Song"
	other.Artists[0].Name = "Nobody"
	other.Duration = 240000
	c := fakeNetease(t,
		[]song{other},
		func(id string) any {
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
	c := fakeNetease(t,
		[]song{{ID: 7, Name: "梵高先生", Duration: 250000,
			Artists: []credit{{Name: "李志"}},
			Album:   albumInfo{Name: "梵高先生2005"}}},
		func(id string) any { return lyricJSON("[00:01.00]words") })
	lrc, err := c.Lookup(context.Background(), songmatch.Track{Title: "08梵高先生", Artist: "李誌", Duration: 250 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if lrc != "[00:01.00]words" {
		t.Errorf("lyrics %q", lrc)
	}
}

func TestCoverFindsTheClosestAlbumArt(t *testing.T) {
	far := known(1)
	far.Album = albumInfo{Name: "Other", PicURL: "http://p1.example/other.jpg"}
	near := known(2)
	near.Album = albumInfo{Name: "Record", PicURL: "http://p2.example/record.jpg"}
	c := fakeNetease(t, []song{far, near}, func(string) any { return nil })
	art, err := c.Cover(context.Background(), songmatch.Track{Album: "Record", Artist: "Band"})
	if err != nil {
		t.Fatal(err)
	}
	if art != "http://p2.example/record.jpg" {
		t.Errorf("art %q", art)
	}
}

func TestCoverIgnoresAnotherAlbum(t *testing.T) {
	s := known(1)
	s.Album = albumInfo{Name: "Other", PicURL: "http://p1.example/other.jpg"}
	c := fakeNetease(t, []song{s}, func(string) any { return nil })
	art, err := c.Cover(context.Background(), songmatch.Track{Album: "Record", Artist: "Band"})
	if err != nil {
		t.Fatal(err)
	}
	if art != "" {
		t.Errorf("art %q", art)
	}
}
