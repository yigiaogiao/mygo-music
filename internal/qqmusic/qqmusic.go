// Package qqmusic finds a song's timed lyrics at QQ Music's public web API,
// which is a second chance for songs LRCLib and NetEase miss.
package qqmusic

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

// DefaultBase is where QQ Music's web API answers.
const DefaultBase = "https://c.y.qq.com"

// A Client asks QQ Music.
type Client struct {
	BaseURL string
	HTTP    *http.Client
	// UserAgent and Referer are what the web API expects of a browser.
	UserAgent string
	Referer   string
}

// New returns a client of QQ Music.
func New() *Client {
	return &Client{
		BaseURL:   DefaultBase,
		HTTP:      &http.Client{Timeout: 10 * time.Second},
		UserAgent: "Mozilla/5.0 (X11; Linux x86_64) mygo-music/0.1",
		Referer:   "https://y.qq.com/",
	}
}

// Lookup finds the timed lyrics of t. It answers "" and no error when QQ
// Music has none.
func (c *Client) Lookup(ctx context.Context, t songmatch.Track) (string, error) {
	t = songmatch.Normalize(t)
	songs, err := c.search(ctx, t)
	if err != nil {
		return "", err
	}
	type found struct {
		mid string
		fit int
	}
	var fits []found
	for _, s := range songs {
		fit := c.fit(s, t)
		if fit >= 2 {
			fits = append(fits, found{mid: s.SongMid, fit: fit})
		}
	}
	sort.SliceStable(fits, func(i, j int) bool { return fits[i].fit > fits[j].fit })
	for i := 0; i < len(fits) && i < 3; i++ {
		lrc, err := c.lyric(ctx, fits[i].mid)
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
	SongMid   string                  `json:"songmid"`
	SongName  string                  `json:"songname"`
	Interval  int64                   `json:"interval"` // seconds
	AlbumName string                  `json:"albumname"`
	AlbumMid  string                  `json:"albummid"`
	Singer    []struct{ Name string } `json:"singer"`
}

func (c *Client) search(ctx context.Context, t songmatch.Track) ([]song, error) {
	q := url.Values{}
	query := t.Title
	if query == "" {
		// An album's cover is found by the album's name.
		query = t.Album
	}
	q.Set("w", strings.TrimSpace(query+" "+t.Artist))
	q.Set("format", "json")
	q.Set("n", "20")
	q.Set("p", "1")
	var out struct {
		Data struct {
			Song struct {
				List []song `json:"list"`
			} `json:"song"`
		} `json:"data"`
	}
	if err := c.do(ctx, "/soso/fcgi-bin/client_search_cp?"+q.Encode(), &out); err != nil {
		return nil, err
	}
	return out.Data.Song.List, nil
}

// Cover finds the album art of t, by its album's ID at QQ Music. It answers
// "" and no error when QQ Music has none.
func (c *Client) Cover(ctx context.Context, t songmatch.Track) (string, error) {
	t = songmatch.Normalize(t)
	songs, err := c.search(ctx, t)
	if err != nil {
		return "", err
	}
	mid, fit := "", 0
	for _, s := range songs {
		if s.AlbumMid == "" {
			continue
		}
		// An album's art is the art of the album asked for, not of
		// another by the same artist.
		if t.Title == "" && !strings.EqualFold(strings.TrimSpace(s.AlbumName), t.Album) {
			continue
		}
		if n := c.fit(s, t); n > fit {
			mid, fit = s.AlbumMid, n
		}
	}
	if fit < 2 {
		return "", nil
	}
	return "https://y.qq.com/music/photo_new/T002R300x300M000" + mid + ".jpg", nil
}

// fit scores how well s is the song t: its title, artist, album and length.
func (c *Client) fit(s song, t songmatch.Track) int {
	artist := make([]string, len(s.Singer))
	for i, a := range s.Singer {
		artist[i] = a.Name
	}
	return songmatch.Score(songmatch.Track{
		Title:    s.SongName,
		Artist:   strings.Join(artist, "/"),
		Album:    s.AlbumName,
		Duration: time.Duration(s.Interval) * time.Second,
	}, t)
}

// timed is a line of LRC, which plain words lack.
var timed = regexp.MustCompile(`\[\d{1,3}:\d{2}`)

func (c *Client) lyric(ctx context.Context, mid string) (string, error) {
	q := url.Values{}
	q.Set("songmid", mid)
	q.Set("format", "json")
	q.Set("nobase64", "1")
	var out struct {
		Lyric   string `json:"lyric"`
		Retcode int    `json:"retcode"`
	}
	if err := c.do(ctx, "/lyric/fcgi-bin/fcg_query_lyric_new.fcg?"+q.Encode(), &out); err != nil {
		return "", err
	}
	if !timed.MatchString(out.Lyric) {
		return "", nil
	}
	return strings.TrimSpace(out.Lyric), nil
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
		return fmt.Errorf("qqmusic: %s", res.Status)
	}
	return json.NewDecoder(io.LimitReader(res.Body, 8<<20)).Decode(out)
}
