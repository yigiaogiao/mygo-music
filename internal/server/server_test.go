package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mygo-music/internal/local"
)

func testServer(t *testing.T) *Server {
	t.Helper()
	dir := t.TempDir()
	s := &Server{
		secret:  []byte("0123456789abcdef0123456789abcdef"),
		library: filepath.Join(dir, "library"),
		local:   local.New(filepath.Join(dir, "library")),
	}
	if err := os.MkdirAll(s.library, 0o755); err != nil {
		t.Fatal(err)
	}
	// A scan started by a test may still be writing into the directory as
	// it's cleaned up.
	t.Cleanup(func() {
		for s.local.Scanning() {
			time.Sleep(time.Millisecond)
		}
	})
	return s
}

func TestTokenRoundTrip(t *testing.T) {
	s := testServer(t)
	for _, p := range []string{
		"/music/Album A/01.mp3",
		"/music/坂本龍一/曲.flac",
		"/lzcapp/media/RemoteFS/a b&c.m4a",
	} {
		got, ok := s.untoken("audio", s.token("audio", p))
		if !ok || got != p {
			t.Errorf("round trip %q: got %q, ok %v", p, got, ok)
		}
	}
}

func TestTokenRejects(t *testing.T) {
	s := testServer(t)
	p := "/music/song.mp3"
	tok := s.token("audio", p)

	// The same token must not open a different kind of URL.
	if _, ok := s.untoken("asset", tok); ok {
		t.Error("token accepted for another kind")
	}

	enc, sig, _ := strings.Cut(tok, ".")
	flip := func(s string) string {
		b := []byte(s)
		if b[0] == 'A' {
			b[0] = 'B'
		} else {
			b[0] = 'A'
		}
		return string(b)
	}
	for _, bad := range []string{
		"",
		".",
		"." + sig,
		enc + ".",
		flip(enc) + "." + sig,
		enc + "." + flip(sig),
		strings.ReplaceAll(tok, ".", ".."),
	} {
		if _, ok := s.untoken("audio", bad); ok {
			t.Errorf("accepted tampered token %q", bad)
		}
	}

	other := &Server{secret: []byte("another secret another secret yes!")}
	if _, ok := other.untoken("audio", tok); ok {
		t.Error("token accepted under another secret")
	}
}

func TestUnder(t *testing.T) {
	root := "/music"
	cases := []struct {
		path string
		want bool
	}{
		{"/music", true},
		{"/music/a.mp3", true},
		{"/music/Album/a.mp3", true},
		{"/music2/a.mp3", false},
		{"/music/../secret.mp3", false},
		{"/", false},
		{"/etc/passwd", false},
	}
	for _, c := range cases {
		if got := under(c.path, root); got != c.want {
			t.Errorf("under(%q, %q) = %v, want %v", c.path, root, got, c.want)
		}
	}
}

func TestMediaServesOnlyAllowed(t *testing.T) {
	s := testServer(t)
	music := t.TempDir()
	file := filepath.Join(music, "song.mp3")
	if err := os.WriteFile(file, []byte("ID3-test-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "secret.mp3")
	if err := os.WriteFile(outside, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	s.local.AddFolders(music)

	serve := func(kind, token string, rng string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/api/"+kind+"/"+token, nil)
		if rng != "" {
			req.Header.Set("Range", rng)
		}
		rec := httptest.NewRecorder()
		s.media(rec, req, kind)
		return rec
	}

	rec := serve("audio", s.token("audio", file), "bytes=0-2")
	if rec.Code != http.StatusPartialContent {
		t.Fatalf("range request: code %d", rec.Code)
	}
	if got := rec.Body.String(); got != "ID3" {
		t.Errorf("range body %q", got)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "audio/mpeg" {
		t.Errorf("content type %q", ct)
	}

	if rec := serve("audio", s.token("audio", outside), ""); rec.Code != http.StatusNotFound {
		t.Errorf("file outside the folders: code %d", rec.Code)
	}
	// A valid token is still checked against the folders, and a forged one
	// against the signature.
	if rec := serve("audio", s.token("audio", file)+"x", ""); rec.Code != http.StatusNotFound {
		t.Errorf("forged token: code %d", rec.Code)
	}
	if rec := serve("asset", s.token("audio", file), ""); rec.Code != http.StatusNotFound {
		t.Errorf("token used for another kind: code %d", rec.Code)
	}
}

func TestMediaRejectsMethods(t *testing.T) {
	s := testServer(t)
	file := filepath.Join(t.TempDir(), "song.mp3")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/audio/"+s.token("audio", file), nil)
	rec := httptest.NewRecorder()
	s.media(rec, req, "audio")
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST: code %d", rec.Code)
	}
}

func TestSetupPageBrowsesTheNetdisk(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "netdisk")
	if err := os.MkdirAll(filepath.Join(root, "Music", "Album"), 0o755); err != nil {
		t.Fatal(err)
	}
	s := testServer(t)
	s.cfg = Config{MusicDir: root, Version: "test"}

	rec := httptest.NewRecorder()
	s.setup(rec, httptest.NewRequest(http.MethodGet, "/setup", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("setup page: %d %s", rec.Code, rec.Body)
	}
	body := rec.Body.String()
	for _, want := range []string{"Music", root, `name="action" value="add"`} {
		if !strings.Contains(body, want) {
			t.Errorf("setup page is missing %q", want)
		}
	}
}

func TestSetupPageOpensTheSingleNetdiskUser(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "RemoteFS")
	if err := os.MkdirAll(filepath.Join(root, "shenhai", "音乐"), 0o755); err != nil {
		t.Fatal(err)
	}
	s := testServer(t)
	s.cfg = Config{MusicDir: root, Version: "test"}

	rec := httptest.NewRecorder()
	s.setup(rec, httptest.NewRequest(http.MethodGet, "/setup", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("setup page: %d %s", rec.Code, rec.Body)
	}
	if body := rec.Body.String(); !strings.Contains(body, "音乐") {
		t.Fatalf("setup page did not open the user's home: %s", body)
	}
}
