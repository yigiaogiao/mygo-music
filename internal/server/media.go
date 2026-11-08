package server

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// token signs a file path for a kind of URL, so that the browser can ask for
// it without the server taking a path from the browser.
func (s *Server) token(kind, path string) string {
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(kind))
	mac.Write([]byte{0})
	mac.Write([]byte(path))
	return base64.RawURLEncoding.EncodeToString([]byte(path)) + "." +
		base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (s *Server) untoken(kind, token string) (string, bool) {
	enc, sig, ok := strings.Cut(token, ".")
	if !ok || enc == "" || sig == "" {
		return "", false
	}
	b, err := base64.RawURLEncoding.DecodeString(enc)
	if err != nil {
		return "", false
	}
	got, err := base64.RawURLEncoding.DecodeString(sig)
	if err != nil {
		return "", false
	}
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(kind))
	mac.Write([]byte{0})
	mac.Write(b)
	if !hmac.Equal(got, mac.Sum(nil)) {
		return "", false
	}
	return string(b), true
}

// media serves an audio file or a picture the app asks for, by its token.
func (s *Server) media(w http.ResponseWriter, r *http.Request, kind string) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	token := strings.TrimPrefix(r.URL.Path, "/api/"+kind+"/")
	if token == "" || strings.Contains(token, "/") {
		http.NotFound(w, r)
		return
	}
	file, ok := s.untoken(kind, token)
	if !ok || !s.allowed(file) {
		http.NotFound(w, r)
		return
	}
	f, err := os.Open(file)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		http.NotFound(w, r)
		return
	}
	h := w.Header()
	h.Set("Content-Type", mediaType(file))
	h.Set("Content-Disposition", "inline")
	h.Set("X-Content-Type-Options", "nosniff")
	if kind == "audio" {
		h.Set("Cache-Control", "private, max-age=3600")
		h.Set("X-Accel-Buffering", "no")
	} else {
		h.Set("Cache-Control", "private, max-age=86400")
	}
	http.ServeContent(w, r, filepath.Base(file), info.ModTime(), f)
}

func mediaType(file string) string {
	switch strings.ToLower(filepath.Ext(file)) {
	case ".mp3":
		return "audio/mpeg"
	case ".flac":
		return "audio/flac"
	case ".m4a", ".mp4":
		return "audio/mp4"
	case ".aac":
		return "audio/aac"
	case ".ogg", ".opus":
		return "audio/ogg"
	case ".wav":
		return "audio/wav"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".webp":
		return "image/webp"
	case ".gif":
		return "image/gif"
	case ".wasm":
		return "application/wasm"
	case ".js":
		return "text/javascript; charset=utf-8"
	case ".html":
		return "text/html; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".json":
		return "application/json; charset=utf-8"
	case ".svg":
		return "image/svg+xml"
	}
	if t := mime.TypeByExtension(filepath.Ext(file)); t != "" {
		return t
	}
	return "application/octet-stream"
}

// static serves the browser app from WebDir.
func (s *Server) static(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.cfg.WebDir == "" {
		http.NotFound(w, r)
		return
	}
	name := path.Clean("/" + r.URL.Path)
	if strings.Contains(name, "..") {
		http.NotFound(w, r)
		return
	}
	if name == "/" {
		name = "/index.html"
	}
	for _, part := range strings.Split(name, "/") {
		if strings.HasPrefix(part, ".") {
			http.NotFound(w, r)
			return
		}
	}
	file := filepath.Join(s.cfg.WebDir, filepath.FromSlash(strings.TrimPrefix(name, "/")))
	if !under(file, s.cfg.WebDir) {
		http.NotFound(w, r)
		return
	}
	info, err := os.Stat(file)
	if err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}
	h := w.Header()
	h.Set("Cache-Control", "no-cache")
	t := mediaType(file)
	// The wasm is the bulk of the app: serve it gzipped when the build made
	// that, and the browser takes gzip.
	if strings.HasSuffix(file, ".wasm") && acceptsGzip(r) {
		if gz := file + ".gz"; fileExists(gz) {
			f, err := os.Open(gz)
			if err == nil {
				defer f.Close()
				if fi, err := f.Stat(); err == nil {
					h.Set("Content-Type", t)
					h.Set("Content-Encoding", "gzip")
					h.Set("Vary", "Accept-Encoding")
					http.ServeContent(w, r, filepath.Base(file), fi.ModTime(), f)
					return
				}
			}
		}
	}
	f, err := os.Open(file)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	h.Set("Content-Type", t)
	http.ServeContent(w, r, filepath.Base(file), info.ModTime(), f)
}

func acceptsGzip(r *http.Request) bool {
	for _, enc := range strings.Split(r.Header.Get("Accept-Encoding"), ",") {
		enc = strings.TrimSpace(strings.Split(enc, ";")[0])
		if strings.EqualFold(enc, "gzip") {
			return true
		}
	}
	return false
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}
