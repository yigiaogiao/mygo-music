package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"

	"mygo-music/internal/hanzi"
	"mygo-music/internal/i18n"
	"mygo-music/internal/local"
	"mygo-music/internal/lrclib"
	"mygo-music/internal/lyrics"
	"mygo-music/internal/music"
	"mygo-music/internal/websource"
)

type callRequest struct {
	Method string            `json:"method"`
	Args   []json.RawMessage `json:"args"`
}

type callFailure struct {
	Kind    string `json:"kind"`
	Message string `json:"message,omitempty"`
}

func (s *Server) call(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !sameOrigin(r) {
		http.Error(w, "cross-origin request refused", http.StatusForbidden)
		return
	}
	var req callRequest
	dec := json.NewDecoder(io.LimitReader(r.Body, 8<<20))
	if err := dec.Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": callFailure{Kind: "error", Message: "bad request: " + err.Error()}})
		return
	}
	if req.Method == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": callFailure{Kind: "error", Message: "no method"}})
		return
	}

	s.langMu.Lock()
	i18n.SetLanguage(language(r))
	result, err := s.dispatch(r.Context(), r, req.Method, req.Args)
	s.langMu.Unlock()

	if err != nil {
		log.Printf("server: %s: %v", req.Method, err)
		f := classify(err)
		writeJSON(w, http.StatusOK, map[string]any{"error": f})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"result": s.rewrite(r, result)})
}

func classify(err error) callFailure {
	f := callFailure{Kind: "error", Message: err.Error()}
	switch {
	case errors.Is(err, music.ErrSignIn):
		f.Kind = "signIn"
	case errors.Is(err, music.ErrSetup):
		f.Kind = "setup"
	case errors.Is(err, music.ErrUnavailable):
		f.Kind = "unavailable"
	case errors.Is(err, music.ErrUnsupported):
		f.Kind = "unsupported"
	}
	if reason := music.Reason(err); reason != "" {
		f.Message = reason
	}
	return f
}

func (s *Server) dispatch(ctx context.Context, r *http.Request, method string, raw []json.RawMessage) (any, error) {
	switch method {
	case "Info":
		return s.local.Info(), nil
	case "Can":
		var what string
		if err := args(raw, &what); err != nil {
			return nil, err
		}
		return s.local.Can(what), nil
	case "Setting":
		var key string
		if err := args(raw, &key); err != nil {
			return nil, err
		}
		return s.local.Setting(key), nil
	case "SetSetting":
		var key, value string
		if err := args(raw, &key, &value); err != nil {
			return nil, err
		}
		return nil, s.local.SetSetting(key, value)
	case "SignIn":
		return nil, music.ErrUnsupported
	case "SignOut":
		return nil, music.ErrUnsupported
	case "Account":
		return nil, nil
	case "Search":
		var query string
		var kind music.SearchKind
		var offset int
		if err := args(raw, &query, &kind, &offset); err != nil {
			return nil, err
		}
		return s.local.Search(query, kind, offset)
	case "Suggest":
		var query string
		if err := args(raw, &query); err != nil {
			return nil, err
		}
		return s.local.Suggest(query)
	case "Playlists":
		return s.local.Playlists()
	case "FolderPlaylists":
		return s.local.FolderPlaylists()
	case "Playlist":
		var id string
		if err := args(raw, &id); err != nil {
			return nil, err
		}
		return s.local.Playlist(id)
	case "Album":
		var id string
		if err := args(raw, &id); err != nil {
			return nil, err
		}
		return s.local.Album(id)
	case "Artist":
		var id string
		if err := args(raw, &id); err != nil {
			return nil, err
		}
		return s.local.Artist(id)
	case "Liked":
		var offset int
		if err := args(raw, &offset); err != nil {
			return nil, err
		}
		return s.local.Liked(offset)
	case "Recent":
		return s.local.Recent()
	case "Home":
		return s.local.Home()
	case "Browse":
		var page string
		if err := args(raw, &page); err != nil {
			return nil, err
		}
		return s.local.Browse(page)
	case "IsLiked":
		var ids []string
		if err := args(raw, &ids); err != nil {
			return nil, err
		}
		return s.local.IsLiked(ids)
	case "Like":
		var id string
		var on bool
		if err := args(raw, &id, &on); err != nil {
			return nil, err
		}
		return nil, s.local.Like(id, on)
	case "LikeEpisode":
		var id string
		var on bool
		if err := args(raw, &id, &on); err != nil {
			return nil, err
		}
		return nil, s.local.LikeEpisode(id, on)
	case "CreatePlaylist":
		var name string
		if err := args(raw, &name); err != nil {
			return nil, err
		}
		return s.local.CreatePlaylist(name)
	case "RenamePlaylist":
		var id, name string
		if err := args(raw, &id, &name); err != nil {
			return nil, err
		}
		return nil, s.local.RenamePlaylist(id, name)
	case "DeletePlaylist":
		var id string
		if err := args(raw, &id); err != nil {
			return nil, err
		}
		return nil, s.local.DeletePlaylist(id)
	case "AddToPlaylist":
		var id string
		var ids []string
		if err := args(raw, &id, &ids); err != nil {
			return nil, err
		}
		return nil, s.local.AddToPlaylist(id, ids)
	case "RemoveFromPlaylist":
		var id string
		var ids []string
		if err := args(raw, &id, &ids); err != nil {
			return nil, err
		}
		return nil, s.local.RemoveFromPlaylist(id, ids)
	case "MovePlaylistTrack":
		var id string
		var from, to int
		if err := args(raw, &id, &from, &to); err != nil {
			return nil, err
		}
		return nil, s.local.MovePlaylistTrack(id, from, to)
	case "SavePlaylist":
		var id string
		var on bool
		if err := args(raw, &id, &on); err != nil {
			return nil, err
		}
		return nil, s.local.SavePlaylist(id, on)
	case "FollowArtist":
		var id string
		var on bool
		if err := args(raw, &id, &on); err != nil {
			return nil, err
		}
		return nil, s.local.FollowArtist(id, on)
	case "SaveAlbum":
		var id string
		var on bool
		if err := args(raw, &id, &on); err != nil {
			return nil, err
		}
		return nil, s.local.SaveAlbum(id, on)
	case "SavedAlbums":
		var offset int
		if err := args(raw, &offset); err != nil {
			return nil, err
		}
		return s.local.SavedAlbums(offset)
	case "Shows":
		return s.local.Shows()
	case "Show":
		var id string
		var offset int
		if err := args(raw, &id, &offset); err != nil {
			return nil, err
		}
		return s.local.Show(id, offset)
	case "SubscribeShow":
		var id string
		var on bool
		if err := args(raw, &id, &on); err != nil {
			return nil, err
		}
		return nil, s.local.SubscribeShow(id, on)
	case "Stations":
		return s.local.Stations()
	case "StationNext":
		var id string
		if err := args(raw, &id); err != nil {
			return nil, err
		}
		return s.local.StationNext(id)
	case "Comments":
		var id string
		var offset int
		if err := args(raw, &id, &offset); err != nil {
			return nil, err
		}
		return s.local.Comments(id, offset)
	case "LikeComment":
		var trackID, commentID string
		var on bool
		if err := args(raw, &trackID, &commentID, &on); err != nil {
			return nil, err
		}
		return nil, s.local.LikeComment(trackID, commentID, on)
	case "PostComment":
		var trackID, text string
		if err := args(raw, &trackID, &text); err != nil {
			return nil, err
		}
		return s.local.PostComment(trackID, text)
	case "Lyrics":
		var id string
		if err := args(raw, &id); err != nil {
			return nil, err
		}
		return s.lyricsFor(ctx, id)
	case "Stream":
		var id, quality string
		if err := args(raw, &id, &quality); err != nil {
			return nil, err
		}
		return s.stream(r, id, quality)
	case "Played":
		var track music.Track
		if err := args(raw, &track); err != nil {
			return nil, err
		}
		s.local.Played(track)
		return nil, nil
	case "Rescan":
		s.local.Rescan()
		return nil, nil
	case "BrowseFolders":
		var path string
		if err := args(raw, &path); err != nil {
			return nil, err
		}
		return s.browseFolders(path)
	case "AddFolder":
		var path string
		if err := args(raw, &path); err != nil {
			return nil, err
		}
		return nil, s.addFolder(path)
	case "RemoveFolder":
		var path string
		if err := args(raw, &path); err != nil {
			return nil, err
		}
		s.local.RemoveFolder(path)
		return nil, nil
	case "RemotePlay", "RemotePause", "RemoteResume", "RemoteSeek", "RemoteVolume", "RemoteState", "RemoteDevices", "RemoteUse":
		return nil, music.ErrUnsupported
	}
	return nil, music.ErrUnsupported
}

func args(raw []json.RawMessage, dst ...any) error {
	if len(raw) != len(dst) {
		return fmt.Errorf("want %d arguments, got %d", len(dst), len(raw))
	}
	for i := range dst {
		if err := json.Unmarshal(raw[i], dst[i]); err != nil {
			return fmt.Errorf("argument %d: %w", i+1, err)
		}
	}
	return nil
}

// lyricsFor answers what the file has, or else what an online service has,
// if online lyrics are on and the file has no timed words.
func (s *Server) lyricsFor(ctx context.Context, id string) (*music.Lyrics, error) {
	l, err := s.local.Lyrics(id)
	if err != nil {
		return nil, err
	}
	if l == nil {
		l = &music.Lyrics{}
	}
	if len(l.Lines) > 0 || len(lyrics.Build(l.LRC, l.Translation)) > 0 {
		return simplifyLyrics(l), nil
	}
	if !s.cfg.OnlineLyrics {
		return simplifyLyrics(l), nil
	}
	t, ok := s.local.Track(id)
	if !ok {
		return simplifyLyrics(l), nil
	}
	lrc, err := s.lyrics.Lookup(ctx, finderFunc(s.findLyrics), lrclib.Track{
		Title:    t.Name,
		Artist:   t.ArtistNames(),
		Album:    t.Album.Name,
		Duration: t.Length(),
	})
	if err != nil {
		log.Printf("server: lyrics of %q: %v", t.Name, err)
		return simplifyLyrics(l), nil
	}
	if lrc == "" {
		return simplifyLyrics(l), nil
	}
	return simplifyLyrics(&music.Lyrics{LRC: lrc}), nil
}

// finderFunc lets a function stand as a lrclib.Finder.
type finderFunc func(context.Context, lrclib.Track) (string, error)

func (f finderFunc) Lookup(ctx context.Context, t lrclib.Track) (string, error) {
	return f(ctx, t)
}

// findLyrics asks each service in turn for timed words, LRCLib first, and
// answers with the first that has them. A service that can't be reached is
// passed over; an error is reported only when none answered.
func (s *Server) findLyrics(ctx context.Context, t lrclib.Track) (string, error) {
	var firstErr error
	for _, finder := range []lrclib.Finder{s.lrclib, s.netease, s.qqmusic} {
		lrc, err := finder.Lookup(ctx, t)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if lrc != "" {
			return lrc, nil
		}
	}
	return "", firstErr
}

// simplifyLyrics writes the words in the simplified characters the app
// speaks, in Chinese: files keep them in traditional ones, as LRCLib often
// does.
func simplifyLyrics(l *music.Lyrics) *music.Lyrics {
	if l == nil || i18n.Language() != i18n.Chinese {
		return l
	}
	l.LRC = hanzi.ToSimplified(l.LRC)
	l.Translation = hanzi.ToSimplified(l.Translation)
	for i := range l.Lines {
		line := &l.Lines[i]
		line.Text = hanzi.ToSimplified(line.Text)
		line.Translation = hanzi.ToSimplified(line.Translation)
		for j := range line.Words {
			line.Words[j].Text = hanzi.ToSimplified(line.Words[j].Text)
		}
	}
	return l
}

// stream answers the file through a URL the browser can play, with a token
// that keeps the path from being changed.
func (s *Server) stream(r *http.Request, id, quality string) (*music.Stream, error) {
	st, err := s.local.Stream(id, quality)
	if err != nil {
		return nil, err
	}
	path, ok := music.PathOf(st.URL)
	if !ok || !s.allowed(path) {
		return nil, music.ErrUnavailable
	}
	st.URL = baseURL(r) + "/api/audio/" + s.token("audio", path)
	st.Headers = nil
	return st, nil
}

func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return strings.EqualFold(u.Host, r.Host)
}

// baseURL is the scheme-relative URL of the server as the browser reached
// it: behind a proxy that may terminate TLS, the scheme the browser used is
// not the one the request came in with, and a wrong one would have the
// browser refuse the sound as mixed content.
func baseURL(r *http.Request) string {
	host := r.Host
	if h := strings.TrimSpace(r.Header.Get("X-Forwarded-Host")); h != "" {
		host = h
	}
	return "//" + host
}

// rewrite makes the picture URLs a source answers into URLs of the server,
// which the browser can load.
func (s *Server) rewrite(r *http.Request, v any) any {
	if v == nil {
		return nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return v
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var x any
	if err := dec.Decode(&x); err != nil {
		return v
	}
	return s.rewriteValue(r, x)
}

func (s *Server) rewriteValue(r *http.Request, v any) any {
	switch x := v.(type) {
	case string:
		path, ok := music.PathOf(x)
		if !ok {
			return x
		}
		if !s.allowed(path) {
			return ""
		}
		return baseURL(r) + "/api/asset/" + s.token("asset", path)
	case []any:
		for i := range x {
			x[i] = s.rewriteValue(r, x[i])
		}
		return x
	case map[string]any:
		if u := s.onlineCover(r, x); u != "" {
			x["album"].(map[string]any)["imageUrl"] = u
		} else if u := s.onlineAlbumCover(r, x); u != "" {
			x["imageUrl"] = u
		}
		for k := range x {
			// The library is the browser's source, not the local one the
			// server wraps: tracks have to say so to play through it.
			if k == "source" {
				if v, ok := x[k].(string); ok && v == local.ID {
					x[k] = websource.ID
					continue
				}
			}
			x[k] = s.rewriteValue(r, x[k])
		}
		return x
	}
	return v
}
