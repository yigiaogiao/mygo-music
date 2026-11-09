// Package server serves the browser app and the music it reads: the app
// compiled to WebAssembly, a small JSON API over internal/local, the files
// themselves with Range support, and a page for choosing the folders.
package server

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"mygo-music/internal/i18n"
	"mygo-music/internal/local"
	"mygo-music/internal/lrclib"
	"mygo-music/internal/netease"
	"mygo-music/internal/qqmusic"
)

// Config is what the server needs to start.
type Config struct {
	// DataDir keeps the library, covers and lyric cache. It must persist.
	DataDir string
	// MusicDir is the root the setup page browses, usually the netdisk's
	// mount, /lzcapp/media/RemoteFS.
	MusicDir string
	// WebDir holds index.html, app.wasm and wasm_exec.js.
	WebDir string
	// Version is shown in the setup page and the status.
	Version string
	// OnlineLyrics looks up songs without timed lyrics at LRCLib, NetEase
	// and QQ Music.
	OnlineLyrics bool
}

// A Server is the app and its music.
type Server struct {
	cfg     Config
	local   *local.Source
	lyrics  *lrclib.Cache
	lrclib  *lrclib.Client
	netease *netease.Client
	qqmusic *qqmusic.Client
	library string // where local keeps its own files

	secret []byte
	mux    *http.ServeMux

	// langMu serializes calls, as the language of what a source says is
	// global.
	langMu sync.Mutex

	// coverMu guards covers, the art being looked up, by its key.
	coverMu sync.Mutex
	covers  map[string]*coverLoad
}

// New opens the library in cfg.DataDir and starts scanning what it holds.
func New(cfg Config) (*Server, error) {
	if cfg.DataDir == "" {
		cfg.DataDir = "data"
	}
	if cfg.MusicDir == "" {
		cfg.MusicDir = "/lzcapp/media/RemoteFS"
	}
	if cfg.WebDir == "" {
		cfg.WebDir = "web"
	}
	if cfg.Version == "" {
		cfg.Version = "dev"
	}
	cfg.DataDir = abs(cfg.DataDir)
	cfg.MusicDir = abs(cfg.MusicDir)
	cfg.WebDir = abs(cfg.WebDir)
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return nil, fmt.Errorf("data directory: %w", err)
	}
	s := &Server{cfg: cfg, library: filepath.Join(cfg.DataDir, "library"), covers: map[string]*coverLoad{}}
	if err := os.MkdirAll(s.library, 0o755); err != nil {
		return nil, fmt.Errorf("library directory: %w", err)
	}
	secret, err := loadSecret(filepath.Join(cfg.DataDir, "secret"))
	if err != nil {
		return nil, err
	}
	s.secret = secret
	s.local = local.New(s.library)
	s.lyrics = lrclib.NewCache(filepath.Join(cfg.DataDir, "lyrics"))
	s.lrclib = lrclib.New()
	s.netease = netease.New()
	s.qqmusic = qqmusic.New()
	s.mux = http.NewServeMux()
	s.routes()
	if len(s.local.Folders()) > 0 {
		s.local.Rescan()
	}
	return s, nil
}

// Handler is the server's HTTP handler.
func (s *Server) Handler() http.Handler { return s.mux }

// Local is the library, for a test or a command to look at.
func (s *Server) Local() *local.Source { return s.local }

func (s *Server) routes() {
	s.mux.HandleFunc("/api/health", s.health)
	s.mux.HandleFunc("/api/status", s.status)
	s.mux.HandleFunc("/api/call", s.call)
	s.mux.HandleFunc("/api/audio/", func(w http.ResponseWriter, r *http.Request) { s.media(w, r, "audio") })
	s.mux.HandleFunc("/api/asset/", func(w http.ResponseWriter, r *http.Request) { s.media(w, r, "asset") })
	s.mux.HandleFunc("/api/cover/", s.cover)
	s.mux.HandleFunc("/setup", s.setup)
	s.mux.HandleFunc("/", s.static)
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	fmt.Fprint(w, `{"ok":true}`)
}

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	st := struct {
		Scanning bool     `json:"scanning"`
		Tracks   int      `json:"tracks"`
		Folders  []string `json:"folders"`
		Root     string   `json:"root"`
		Version  string   `json:"version"`
	}{
		Scanning: s.local.Scanning(),
		Tracks:   s.local.TrackCount(),
		Folders:  s.local.Folders(),
		Root:     s.cfg.MusicDir,
		Version:  s.cfg.Version,
	}
	writeJSON(w, http.StatusOK, st)
}

// language picks what the source speaks for a request.
func language(r *http.Request) string {
	if l := r.Header.Get("X-MyGo-Language"); l != "" {
		return i18n.Resolve(l, nil)
	}
	return i18n.Resolve(i18n.System, strings.Split(r.Header.Get("Accept-Language"), ","))
}

func loadSecret(path string) ([]byte, error) {
	b, err := os.ReadFile(path)
	if err == nil && len(b) >= 32 {
		return b[:32], nil
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	b = make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		return nil, err
	}
	return b, nil
}

func abs(path string) string {
	if p, err := filepath.Abs(path); err == nil {
		return p
	}
	return filepath.Clean(path)
}

// allowed reports whether path is one of the files the server serves: a track
// in the folders, a cover, or a picture beside one.
func (s *Server) allowed(path string) bool {
	path = filepath.Clean(path)
	if !filepath.IsAbs(path) {
		return false
	}
	if under(path, filepath.Join(s.library, "covers")) {
		return true
	}
	for _, root := range s.local.Folders() {
		if under(path, root) {
			return true
		}
	}
	return false
}

func under(path, root string) bool {
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(path))
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	if err := writeJSONBody(w, v); err != nil {
		log.Printf("server: writing JSON: %v", err)
	}
}

func writeJSONBody(w http.ResponseWriter, v any) error {
	return json.NewEncoder(w).Encode(v)
}
