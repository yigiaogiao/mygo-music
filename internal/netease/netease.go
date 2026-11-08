// Package netease finds a song's timed lyrics at NetEase Cloud Music's web
// API, whose catalogue is wider than LRCLib's for Chinese songs.
package netease

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"mygo-music/internal/songmatch"
)

// DefaultBase is where NetEase answers.
const DefaultBase = "https://music.163.com"

// A Client asks NetEase.
type Client struct {
	BaseURL string
	HTTP    *http.Client
	// UserAgent and Referer are what the web API expects of a browser.
	UserAgent string
	Referer   string
}

// New returns a client of NetEase.
func New() *Client {
	return &Client{
		BaseURL:   DefaultBase,
		HTTP:      &http.Client{Timeout: 10 * time.Second},
		UserAgent: "Mozilla/5.0 (X11; Linux x86_64) mygo-music/0.1",
		Referer:   DefaultBase + "/",
	}
}

// Lookup finds the timed lyrics of t. It answers "" and no error when NetEase
// has none.
func (c *Client) Lookup(ctx context.Context, t songmatch.Track) (string, error) {
	t = songmatch.Normalize(t)
	songs, err := c.search(ctx, t)
	if err != nil {
		return "", err
	}
	type found struct {
		id  int64
		fit int
	}
	var fits []found
	for _, s := range songs {
		artist := make([]string, len(s.Artists))
		for i, a := range s.Artists {
			artist[i] = a.Name
		}
		fit := songmatch.Score(songmatch.Track{
			Title:    s.Name,
			Artist:   strings.Join(artist, "/"),
			Album:    s.Album.Name,
			Duration: time.Duration(s.Duration) * time.Millisecond,
		}, t)
		if fit >= 2 {
			fits = append(fits, found{id: s.ID, fit: fit})
		}
	}
	sort.SliceStable(fits, func(i, j int) bool { return fits[i].fit > fits[j].fit })
	// A song's first result may be an instrumental, or one NetEase has no
	// words for: the next of those that fit the song may.
	for i := 0; i < len(fits) && i < 3; i++ {
		lrc, err := c.lyric(ctx, fits[i].id)
		if err != nil {
			return "", err
		}
		if lrc != "" {
			return lrc, nil
		}
	}
	return "", nil
}

type song struct {
	ID       int64                   `json:"id"`
	Name     string                  `json:"name"`
	Duration int64                   `json:"duration"` // milliseconds
	Artists  []struct{ Name string } `json:"artists"`
	Album    struct{ Name string }   `json:"album"`
}

func (c *Client) search(ctx context.Context, t songmatch.Track) ([]song, error) {
	q := url.Values{}
	query := strings.TrimSpace(t.Title + " " + t.Artist)
	q.Set("s", query)
	q.Set("type", "1")
	q.Set("limit", "20")
	q.Set("offset", "0")
	var out struct {
		Result struct {
			Songs []song `json:"songs"`
		} `json:"result"`
	}
	if err := c.do(ctx, "/api/search/get/web?"+q.Encode(), &out); err != nil {
		return nil, err
	}
	return out.Result.Songs, nil
}

// timed is a line of LRC, which plain words lack.
var timed = regexp.MustCompile(`\[\d{1,3}:\d{2}`)

func (c *Client) lyric(ctx context.Context, id int64) (string, error) {
	q := url.Values{}
	q.Set("id", fmt.Sprint(id))
	q.Set("lv", "-1")
	q.Set("kv", "-1")
	q.Set("tv", "-1")
	var out struct {
		LRC struct {
			Lyric string `json:"lyric"`
		} `json:"lrc"`
		NoLyric     bool `json:"nolyric"`
		Uncollected bool `json:"uncollected"`
	}
	if err := c.do(ctx, "/api/song/lyric?"+q.Encode(), &out); err != nil {
		return "", err
	}
	if out.NoLyric || out.Uncollected || !timed.MatchString(out.LRC.Lyric) {
		return "", nil
	}
	return strings.TrimSpace(out.LRC.Lyric), nil
}

func (c *Client) do(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(c.BaseURL, "/")+path, nil)
	if err != nil {
		return err
	}
	if c.UserAgent != "" {
		req.Header.Set("User-Agent", c.UserAgent)
	}
	if c.Referer != "" {
		req.Header.Set("Referer", c.Referer)
	}
	req.Header.Set("Accept", "application/json")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("netease: %s", res.Status)
	}
	return json.NewDecoder(io.LimitReader(res.Body, 8<<20)).Decode(out)
}
