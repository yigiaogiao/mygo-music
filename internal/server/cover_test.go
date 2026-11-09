package server

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"mygo-music/internal/netease"
	"mygo-music/internal/qqmusic"
)

// coverServer is a server whose NetEase answers with one song of the album
// asked for, its art at art.
func coverServer(t *testing.T, art []byte) (*Server, *atomic.Int32, func()) {
	t.Helper()
	var imageCalls atomic.Int32
	images := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		imageCalls.Add(1)
		w.Header().Set("Content-Type", "image/png")
		w.Write(art)
	}))
	var music *httptest.Server
	music = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"result": map[string]any{"songs": []any{map[string]any{
				"id": 1, "name": "Song", "duration": 180000,
				"artists": []any{map[string]any{"name": "Band"}},
				"album":   map[string]any{"name": "Record", "picUrl": images.URL + "/art.png"},
			}}},
		})
	}))
	t.Cleanup(music.Close)
	var once sync.Once
	closeImages := func() { once.Do(images.Close) }
	t.Cleanup(closeImages)
	n := netease.New()
	n.BaseURL, n.HTTP = music.URL, music.Client()
	q := qqmusic.New()
	q.BaseURL, q.HTTP = music.URL, music.Client()
	s := &Server{
		cfg:     Config{DataDir: t.TempDir(), OnlineLyrics: true},
		netease: n,
		qqmusic: q,
		covers:  map[string]*coverLoad{},
		secret:  []byte("0123456789abcdef0123456789abcdef"),
	}
	return s, &imageCalls, closeImages
}

func testPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestCoverBytesFindsAndKeepsArt(t *testing.T) {
	art := testPNG(t)
	s, imageCalls, closeImages := coverServer(t, art)
	data, ctype, err := s.coverBytes(context.Background(), "album\x00Record\x00Band")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, art) || ctype != "image/png" {
		t.Fatalf("art %d bytes of %q", len(data), ctype)
	}
	if n := imageCalls.Load(); n != 1 {
		t.Errorf("fetched the art %d times", n)
	}
	// The next time, it's on disk, and the image server isn't needed.
	closeImages()
	data, _, err = s.coverBytes(context.Background(), "album\x00Record\x00Band")
	if err != nil || !bytes.Equal(data, art) {
		t.Fatalf("from disk: %d bytes, %v", len(data), err)
	}
}

func TestCoverServesWhatItFinds(t *testing.T) {
	art := testPNG(t)
	s, _, _ := coverServer(t, art)
	token := s.token("cover", "album\x00Record\x00Band")
	req := httptest.NewRequest(http.MethodGet, "/api/cover/"+token, nil)
	rec := httptest.NewRecorder()
	s.cover(rec, req)
	if rec.Code != http.StatusOK || !bytes.Equal(rec.Body.Bytes(), art) {
		t.Fatalf("answered %d: %d bytes", rec.Code, rec.Body.Len())
	}
	if got := rec.Header().Get("Cache-Control"); !strings.Contains(got, "immutable") {
		t.Errorf("Cache-Control %q", got)
	}
}

func TestCoverRejectsAChangedToken(t *testing.T) {
	s, _, _ := coverServer(t, testPNG(t))
	req := httptest.NewRequest(http.MethodGet, "/api/cover/album.xxxx", nil)
	rec := httptest.NewRecorder()
	s.cover(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("answered %d", rec.Code)
	}
}

func TestOnlineCoverFillsOnlyMissingArt(t *testing.T) {
	s, _, _ := coverServer(t, testPNG(t))
	r := httptest.NewRequest(http.MethodPost, "http://app.example/api/call", nil)
	track := map[string]any{
		"name":    "Song",
		"artists": []any{map[string]any{"name": "Band"}},
		"album":   map[string]any{"name": "Record"},
	}
	u := s.onlineCover(r, track)
	if !strings.HasPrefix(u, "//app.example/api/cover/") {
		t.Errorf("cover URL %q", u)
	}
	track["album"].(map[string]any)["imageUrl"] = "file:///covers/record.jpg"
	if u := s.onlineCover(r, track); u != "" {
		t.Errorf("covered a track that has art: %q", u)
	}

	album := map[string]any{
		"name":    "Record",
		"artists": []any{map[string]any{"name": "Band"}},
	}
	if u := s.onlineAlbumCover(r, album); !strings.HasPrefix(u, "//app.example/api/cover/") {
		t.Errorf("album cover URL %q", u)
	}
	album["imageUrl"] = "file:///covers/record.jpg"
	if u := s.onlineAlbumCover(r, album); u != "" {
		t.Errorf("covered an album that has art: %q", u)
	}
	if u := s.onlineAlbumCover(r, track); u != "" {
		t.Errorf("covered a track as an album: %q", u)
	}
}

func TestCoverFallsBackToTheAlbum(t *testing.T) {
	art := testPNG(t)
	images := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write(art)
	}))
	defer images.Close()
	music := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var songs []any
		if strings.Contains(r.URL.Query().Get("s"), "Record") {
			songs = []any{map[string]any{
				"id": 1, "name": "Unknown", "duration": 180000,
				"artists": []any{map[string]any{"name": "Band"}},
				"album":   map[string]any{"name": "Record", "picUrl": images.URL + "/art.png"},
			}}
		}
		json.NewEncoder(w).Encode(map[string]any{"result": map[string]any{"songs": songs}})
	}))
	defer music.Close()
	n := netease.New()
	n.BaseURL, n.HTTP = music.URL, music.Client()
	q := qqmusic.New()
	q.BaseURL, q.HTTP = music.URL, music.Client()
	s := &Server{
		cfg:     Config{DataDir: t.TempDir(), OnlineLyrics: true},
		netease: n,
		qqmusic: q,
		covers:  map[string]*coverLoad{},
		secret:  []byte("0123456789abcdef0123456789abcdef"),
	}
	data, _, err := s.coverBytes(context.Background(), "song\x00Unknown\x00Band\x00Record")
	if err != nil || !bytes.Equal(data, art) {
		t.Fatalf("album art: %d bytes, %v", len(data), err)
	}
}
