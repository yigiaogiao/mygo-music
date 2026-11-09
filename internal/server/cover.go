package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"mygo-music/internal/music"
	"mygo-music/internal/songmatch"
)

// coverLoad is a cover being looked up, which others asking for the same
// one wait for rather than look up again.
type coverLoad struct {
	done  chan struct{}
	data  []byte
	ctype string
	err   error
}

// coverer finds the art of a track somewhere online.
type coverer interface {
	Cover(context.Context, songmatch.Track) (string, error)
}

// cover serves the art of a track, looked up online by its album or name
// and kept for next time.
func (s *Server) cover(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	token := strings.TrimPrefix(r.URL.Path, "/api/cover/")
	if token == "" || strings.Contains(token, "/") {
		http.NotFound(w, r)
		return
	}
	payload, ok := s.untoken("cover", token)
	if !ok {
		http.NotFound(w, r)
		return
	}
	data, ctype, err := s.coverBytes(r.Context(), payload)
	if err != nil || len(data) == 0 {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	// The art of a song doesn't change: the browser keeps it, as it does a
	// local cover.
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	if r.Method == http.MethodHead {
		return
	}
	w.Write(data)
}

// coverBytes answers the art a payload names, from the cache if it's there,
// or else from the services, whose answer it keeps.
func (s *Server) coverBytes(ctx context.Context, payload string) ([]byte, string, error) {
	key := coverKey(payload)
	if data, ok := s.readCover(key); ok {
		return data, http.DetectContentType(data), nil
	}

	s.coverMu.Lock()
	if l := s.covers[key]; l != nil {
		s.coverMu.Unlock()
		select {
		case <-l.done:
			return l.data, l.ctype, l.err
		case <-ctx.Done():
			return nil, "", ctx.Err()
		}
	}
	l := &coverLoad{done: make(chan struct{})}
	s.covers[key] = l
	s.coverMu.Unlock()

	l.data, l.ctype, l.err = s.findCover(ctx, payload)
	if l.err == nil && len(l.data) > 0 {
		s.writeCover(key, l.data)
	}
	close(l.done)
	s.coverMu.Lock()
	delete(s.covers, key)
	s.coverMu.Unlock()
	return l.data, l.ctype, l.err
}

// findCover asks each service in turn for the art of what payload names,
// then fetches it. It answers no bytes and no error when there is none.
func (s *Server) findCover(ctx context.Context, payload string) ([]byte, string, error) {
	t, ok := coverTrack(payload)
	if !ok {
		return nil, "", nil
	}
	art, firstErr := s.findArt(ctx, t)
	// A song the services don't know by name may still be on an album they
	// know, whose art is the song's.
	if art == "" && t.Title != "" && t.Album != "" {
		art, _ = s.findArt(ctx, songmatch.Track{Album: t.Album, Artist: t.Artist})
	}
	if art == "" {
		return nil, "", firstErr
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, art, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", music.Agent)
	res, err := coverHTTP.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("cover: %s", res.Status)
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, 16<<20))
	if err != nil {
		return nil, "", err
	}
	ctype := http.DetectContentType(data)
	if len(data) == 0 || !strings.HasPrefix(ctype, "image/") {
		return nil, "", fmt.Errorf("cover: %s is not an image", art)
	}
	return data, ctype, nil
}

// findArt asks each service in turn for the art of t, as it is named, and
// answers the first it finds, or else the error of the first that failed.
func (s *Server) findArt(ctx context.Context, t songmatch.Track) (string, error) {
	var firstErr error
	for _, c := range []coverer{s.netease, s.qqmusic} {
		u, err := c.Cover(ctx, t)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if u != "" {
			return u, nil
		}
	}
	return "", firstErr
}

// coverTrack reads what a cover payload names: an album, or a song, and
// whose.
func coverTrack(payload string) (songmatch.Track, bool) {
	parts := strings.Split(payload, "\x00")
	switch {
	case len(parts) == 3 && parts[0] == "album":
		return songmatch.Track{Album: parts[1], Artist: parts[2]}, true
	case len(parts) == 4 && parts[0] == "song":
		return songmatch.Track{Title: parts[1], Artist: parts[2], Album: parts[3]}, true
	}
	return songmatch.Track{}, false
}

func coverKey(payload string) string {
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:16])
}

func (s *Server) coverPath(key string) string {
	return filepath.Join(s.cfg.DataDir, "covers", key+".img")
}

func (s *Server) readCover(key string) ([]byte, bool) {
	data, err := os.ReadFile(s.coverPath(key))
	if err != nil || len(data) == 0 {
		return nil, false
	}
	return data, true
}

func (s *Server) writeCover(key string, data []byte) {
	path := s.coverPath(key)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
	}
}

// coverHTTP fetches art, and may take longer than a source's answer.
var coverHTTP = &http.Client{Timeout: 20 * time.Second}

// onlineCover gives a track without art the URL of the art of the song, to
// be found online as the browser asks for it, or of its album when the song
// is unknown.
func (s *Server) onlineCover(r *http.Request, track map[string]any) string {
	if !s.cfg.OnlineLyrics {
		return ""
	}
	album, ok := track["album"].(map[string]any)
	if !ok {
		return ""
	}
	if img, _ := album["imageUrl"].(string); img != "" {
		return ""
	}
	name, _ := track["name"].(string)
	albumName, _ := album["name"].(string)
	artist := artistNames(track["artists"])
	if artist == "" {
		artist = artistNames(album["artists"])
	}
	var payload string
	switch {
	case name != "":
		payload = "song\x00" + name + "\x00" + artist + "\x00" + albumName
	case albumName != "":
		payload = "album\x00" + albumName + "\x00" + artist
	default:
		return ""
	}
	return baseURL(r) + "/api/cover/" + s.token("cover", payload)
}

// onlineAlbumCover gives an album without art the URL of its art, to be
// found online as the browser asks for it: the albums page has album entries
// of their own, which are not tracks.
func (s *Server) onlineAlbumCover(r *http.Request, album map[string]any) string {
	if !s.cfg.OnlineLyrics {
		return ""
	}
	if img, _ := album["imageUrl"].(string); img != "" {
		return ""
	}
	// A track has an album of its own, filled above; this is an album.
	if _, track := album["album"]; track {
		return ""
	}
	name, _ := album["name"].(string)
	artist := artistNames(album["artists"])
	if name == "" || artist == "" {
		return ""
	}
	return baseURL(r) + "/api/cover/" + s.token("cover", "album\x00"+name+"\x00"+artist)
}

// artistNames joins the names of a track's or an album's artists, as the
// credits read.
func artistNames(v any) string {
	list, _ := v.([]any)
	names := make([]string, 0, len(list))
	for _, e := range list {
		m, _ := e.(map[string]any)
		if name, _ := m["name"].(string); name != "" {
			names = append(names, name)
		}
	}
	return strings.Join(names, ", ")
}
