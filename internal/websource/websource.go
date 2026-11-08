// Package websource is a music source that answers from a mygo-music server
// over HTTP, as the app in a browser does: the server reads the files on the
// machine it runs on, and this side draws them.
package websource

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"mygo-music/internal/i18n"
	"mygo-music/internal/music"
)

// ID is the source's ID, on every track it answers.
const ID = "lazycat"

// can is what Can says yes to, by the name of the plugin function. It is the
// local source's set: the server wraps internal/local.
var can = map[string]bool{
	"search": true, "playlists": true, "playlist": true, "album": true, "artist": true,
	"liked": true, "recent": true, "isLiked": true, "like": true,
	"createPlaylist": true, "renamePlaylist": true, "deletePlaylist": true,
	"addToPlaylist": true, "removeFromPlaylist": true, "movePlaylistTrack": true,
	"lyrics": true, "stream": true, "home": true, "browse": true,
}

// Status is what the server says of the library, for the settings.
type Status struct {
	Scanning bool     `json:"scanning"`
	Tracks   int      `json:"tracks"`
	Folders  []string `json:"folders"`
	Root     string   `json:"root"`
	Version  string   `json:"version,omitempty"`
}

// A Source is a music.Source answered by a mygo-music server.
type Source struct {
	base string
	http *http.Client

	ctx    context.Context
	cancel context.CancelFunc
	once   sync.Once

	mu       sync.Mutex
	status   Status
	onChange func()
	known    bool
}

// New returns a source answered by the server at base, such as
// "http://127.0.0.1:8090".
func New(base string) *Source {
	ctx, cancel := context.WithCancel(context.Background())
	s := &Source{
		base:   strings.TrimRight(base, "/"),
		http:   &http.Client{Timeout: 30 * time.Second},
		ctx:    ctx,
		cancel: cancel,
	}
	go s.watch()
	return s
}

// Close stops watching the server.
func (s *Source) Close() { s.once.Do(s.cancel) }

// SetupURL is the server's page for choosing folders.
func (s *Source) SetupURL() string { return s.base + "/setup" }

// Status is what the server said last, without waiting for it.
func (s *Source) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

// Rescan asks the server to scan its folders again.
func (s *Source) Rescan() error {
	var out struct{}
	return s.call("Rescan", nil, &out)
}

// OnChange sets what's called, on any goroutine, after what the server lists
// changed: a scan's batches and its end, folders, likes and plays.
func (s *Source) OnChange(fn func()) {
	s.mu.Lock()
	s.onChange = fn
	s.mu.Unlock()
}

func (s *Source) watch() {
	// The first look tells the app what the library is, without calling back:
	// it is loading anyway.
	if st, err := s.fetchStatus(); err == nil {
		s.mu.Lock()
		s.status, s.known = st, true
		s.mu.Unlock()
	}
	t := time.NewTicker(4 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-t.C:
		}
		st, err := s.fetchStatus()
		if err != nil {
			continue
		}
		s.mu.Lock()
		changed := s.known && (st.Scanning != s.status.Scanning || st.Tracks != s.status.Tracks || !sameFolders(st.Folders, s.status.Folders))
		s.status, s.known = st, true
		fn := s.onChange
		s.mu.Unlock()
		if changed && fn != nil {
			fn()
		}
	}
}

func sameFolders(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (s *Source) fetchStatus() (Status, error) {
	req, err := http.NewRequestWithContext(s.ctx, http.MethodGet, s.base+"/api/status", nil)
	if err != nil {
		return Status{}, err
	}
	res, err := s.http.Do(req)
	if err != nil {
		return Status{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return Status{}, fmt.Errorf("status: %s", res.Status)
	}
	var st Status
	if err := json.NewDecoder(res.Body).Decode(&st); err != nil {
		return Status{}, err
	}
	return st, nil
}

// call asks the server to run method with args and decodes its result into
// out, if out is not nil.
func (s *Source) call(method string, args []any, out any) error {
	body, err := json.Marshal(map[string]any{"method": method, "args": args})
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(s.ctx, 60*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.base+"/api/call", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-MyGo-Language", i18n.Language())
	res, err := s.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	var msg struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Kind    string `json:"kind"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 64<<20)).Decode(&msg); err != nil {
		return fmt.Errorf("music server: %s", res.Status)
	}
	if msg.Error != nil {
		return fail(msg.Error.Kind, msg.Error.Message)
	}
	if out != nil && len(msg.Result) > 0 && string(msg.Result) != "null" {
		if err := json.Unmarshal(msg.Result, out); err != nil {
			return fmt.Errorf("music server: %w", err)
		}
	}
	return nil
}

func fail(kind, message string) error {
	switch kind {
	case "signIn":
		return music.Fail(music.ErrSignIn, message)
	case "setup":
		return music.Fail(music.ErrSetup, message)
	case "unavailable":
		return music.Fail(music.ErrUnavailable, message)
	case "unsupported":
		return music.Fail(music.ErrUnsupported, message)
	}
	if message == "" {
		return errors.New("music server: request failed")
	}
	return errors.New(message)
}

func (s *Source) Info() music.Info {
	return music.Info{
		ID:       ID,
		Name:     i18n.T("Netdisk Music"),
		Playback: music.PlaybackStream,
		Navigation: []music.NavItem{
			{Builtin: "home"},
			{Title: i18n.T("Songs"), Page: "songs", Icon: "note"},
			{Title: i18n.T("Albums"), Page: "albums", Icon: "album"},
			{Title: i18n.T("Artists"), Page: "artists", Icon: "artist"},
			{Builtin: "liked"},
		},
	}
}

func (s *Source) Can(what string) bool { return can[what] }

func (s *Source) Setting(key string) string          { return "" }
func (s *Source) SetSetting(key, value string) error { return nil }

func (s *Source) SignIn() (*music.Account, error)  { return nil, music.ErrUnsupported }
func (s *Source) SignOut() error                   { return music.ErrUnsupported }
func (s *Source) Account() (*music.Account, error) { return nil, nil }

func (s *Source) Search(query string, kind music.SearchKind, offset int) (*music.SearchResult, error) {
	var out music.SearchResult
	err := s.call("Search", []any{query, kind, offset}, &out)
	return &out, err
}

func (s *Source) Suggest(query string) ([]string, error) {
	var out []string
	err := s.call("Suggest", []any{query}, &out)
	return out, err
}

func (s *Source) Playlists() ([]music.Playlist, error) {
	var out []music.Playlist
	err := s.call("Playlists", nil, &out)
	return out, err
}

func (s *Source) Playlist(id string) (*music.Playlist, error) {
	var out music.Playlist
	err := s.call("Playlist", []any{id}, &out)
	return &out, err
}

func (s *Source) Album(id string) (*music.AlbumPage, error) {
	var out music.AlbumPage
	err := s.call("Album", []any{id}, &out)
	return &out, err
}

func (s *Source) Artist(id string) (*music.ArtistPage, error) {
	var out music.ArtistPage
	err := s.call("Artist", []any{id}, &out)
	return &out, err
}

func (s *Source) Liked(offset int) (*music.Page, error) {
	var out music.Page
	err := s.call("Liked", []any{offset}, &out)
	return &out, err
}

func (s *Source) Recent() ([]music.Track, error) {
	var out []music.Track
	err := s.call("Recent", nil, &out)
	return out, err
}

func (s *Source) Home() (*music.Browse, error) {
	var out music.Browse
	err := s.call("Home", nil, &out)
	return &out, err
}

func (s *Source) Browse(page string) (*music.Browse, error) {
	var out music.Browse
	err := s.call("Browse", []any{page}, &out)
	return &out, err
}

func (s *Source) IsLiked(ids []string) (map[string]bool, error) {
	var out map[string]bool
	err := s.call("IsLiked", []any{ids}, &out)
	return out, err
}

func (s *Source) Like(id string, on bool) error {
	return s.call("Like", []any{id, on}, nil)
}

func (s *Source) LikeEpisode(id string, on bool) error {
	return s.call("LikeEpisode", []any{id, on}, nil)
}

func (s *Source) CreatePlaylist(name string) (*music.Playlist, error) {
	var out music.Playlist
	err := s.call("CreatePlaylist", []any{name}, &out)
	return &out, err
}

func (s *Source) RenamePlaylist(id, name string) error {
	return s.call("RenamePlaylist", []any{id, name}, nil)
}

func (s *Source) DeletePlaylist(id string) error {
	return s.call("DeletePlaylist", []any{id}, nil)
}

func (s *Source) AddToPlaylist(id string, trackIDs []string) error {
	return s.call("AddToPlaylist", []any{id, trackIDs}, nil)
}

func (s *Source) RemoveFromPlaylist(id string, trackIDs []string) error {
	return s.call("RemoveFromPlaylist", []any{id, trackIDs}, nil)
}

func (s *Source) MovePlaylistTrack(id string, from, to int) error {
	return s.call("MovePlaylistTrack", []any{id, from, to}, nil)
}

func (s *Source) SavePlaylist(id string, on bool) error {
	return s.call("SavePlaylist", []any{id, on}, nil)
}

func (s *Source) FollowArtist(id string, on bool) error {
	return s.call("FollowArtist", []any{id, on}, nil)
}

func (s *Source) SaveAlbum(id string, on bool) error {
	return s.call("SaveAlbum", []any{id, on}, nil)
}

func (s *Source) SavedAlbums(offset int) (*music.AlbumList, error) {
	var out music.AlbumList
	err := s.call("SavedAlbums", []any{offset}, &out)
	return &out, err
}

func (s *Source) Shows() ([]music.Show, error) {
	var out []music.Show
	err := s.call("Shows", nil, &out)
	return out, err
}

func (s *Source) Show(id string, offset int) (*music.ShowPage, error) {
	var out music.ShowPage
	err := s.call("Show", []any{id, offset}, &out)
	return &out, err
}

func (s *Source) SubscribeShow(id string, on bool) error {
	return s.call("SubscribeShow", []any{id, on}, nil)
}

func (s *Source) Stations() ([]music.Station, error) {
	var out []music.Station
	err := s.call("Stations", nil, &out)
	return out, err
}

func (s *Source) StationNext(id string) ([]music.Track, error) {
	var out []music.Track
	err := s.call("StationNext", []any{id}, &out)
	return out, err
}

func (s *Source) Comments(trackID string, offset int) (*music.CommentPage, error) {
	var out music.CommentPage
	err := s.call("Comments", []any{trackID, offset}, &out)
	return &out, err
}

func (s *Source) LikeComment(trackID, commentID string, on bool) error {
	return s.call("LikeComment", []any{trackID, commentID, on}, nil)
}

func (s *Source) PostComment(trackID, text string) (*music.Comment, error) {
	var out music.Comment
	err := s.call("PostComment", []any{trackID, text}, &out)
	return &out, err
}

func (s *Source) Lyrics(id string) (*music.Lyrics, error) {
	var out music.Lyrics
	err := s.call("Lyrics", []any{id}, &out)
	return &out, err
}

func (s *Source) Stream(id, quality string) (*music.Stream, error) {
	var out music.Stream
	err := s.call("Stream", []any{id, quality}, &out)
	return &out, err
}

// Played tells the server what's played, for its Recent.
func (s *Source) Played(t music.Track) {
	_ = s.call("Played", []any{t}, nil)
}

func (s *Source) RemotePlay(ids []string, start int, position int64) error {
	return music.ErrUnsupported
}
func (s *Source) RemotePause() error              { return music.ErrUnsupported }
func (s *Source) RemoteResume() error             { return music.ErrUnsupported }
func (s *Source) RemoteSeek(position int64) error { return music.ErrUnsupported }
func (s *Source) RemoteVolume(percent int) error  { return music.ErrUnsupported }
func (s *Source) RemoteState() (*music.RemoteState, error) {
	return nil, music.ErrUnsupported
}
func (s *Source) RemoteDevices() ([]music.Device, error) { return nil, music.ErrUnsupported }
func (s *Source) RemoteUse(deviceID string) error        { return music.ErrUnsupported }
