// Package lrclib finds a song's lyrics at LRCLib, which keeps the timed
// words of songs people have uploaded, and caches what it finds on disk.
package lrclib

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"mygo-music/internal/hanzi"
)

// DefaultBase is where LRCLib answers.
const DefaultBase = "https://lrclib.net"

// Track is what is known of a song to look it up by.
type Track struct {
	Title    string
	Artist   string
	Album    string
	Duration time.Duration
}

// A Result is what LRCLib has of a song.
type Result struct {
	ID           int64   `json:"id"`
	TrackName    string  `json:"trackName"`
	ArtistName   string  `json:"artistName"`
	AlbumName    string  `json:"albumName"`
	Duration     float64 `json:"duration"` // seconds, which LRCLib writes with a decimal
	Instrumental bool    `json:"instrumental"`
	PlainLyrics  string  `json:"plainLyrics"`
	SyncedLyrics string  `json:"syncedLyrics"`
}

// A Client asks LRCLib.
type Client struct {
	BaseURL string
	HTTP    *http.Client
	// UserAgent identifies the app, as LRCLib asks.
	UserAgent string
}

// New returns a client of LRCLib.
func New() *Client {
	return &Client{
		BaseURL:   DefaultBase,
		HTTP:      &http.Client{Timeout: 10 * time.Second},
		UserAgent: "mygo-music/0.1 (https://github.com/yigiaogiao/mygo-music)",
	}
}

// Lookup finds the timed lyrics of t. It answers "" and no error when LRCLib
// has none.
func (c *Client) Lookup(ctx context.Context, t Track) (string, error) {
	// Tags written on the other side of the strait, and track numbers rippers
	// put before titles, don't name the song LRCLib knows.
	t = Track{
		Title:    cleanTitle(hanzi.ToSimplified(t.Title)),
		Artist:   hanzi.ToSimplified(t.Artist),
		Album:    hanzi.ToSimplified(t.Album),
		Duration: t.Duration,
	}
	// /api/get wants the artist and the album as well as the title, and a
	// track tagged with only a title (a WAV, say) would make it a 400. Search
	// finds those.
	if t.Artist != "" && t.Album != "" {
		if r, err := c.get(ctx, t); err == nil {
			if r.SyncedLyrics != "" && !r.Instrumental {
				return r.SyncedLyrics, nil
			}
			if r.Instrumental {
				return "", nil
			}
		} else if !errors.Is(err, errNotFound) {
			return "", err
		}
	}
	rs, err := c.search(ctx, t)
	if err != nil {
		return "", err
	}
	best := pick(rs, t)
	if best == nil {
		return "", nil
	}
	return best.SyncedLyrics, nil
}

var errNotFound = errors.New("lrclib: not found")

func (c *Client) get(ctx context.Context, t Track) (*Result, error) {
	q := url.Values{}
	q.Set("track_name", t.Title)
	if t.Artist != "" {
		q.Set("artist_name", t.Artist)
	}
	if t.Album != "" {
		q.Set("album_name", t.Album)
	}
	if t.Duration > 0 {
		q.Set("duration", fmt.Sprint(int(t.Duration.Seconds())))
	}
	var r Result
	if err := c.do(ctx, "/api/get?"+q.Encode(), &r); err != nil {
		return nil, err
	}
	return &r, nil
}

func (c *Client) search(ctx context.Context, t Track) ([]Result, error) {
	q := url.Values{}
	q.Set("track_name", t.Title)
	if t.Artist != "" {
		q.Set("artist_name", t.Artist)
	}
	// The album is left out: search keeps only what names it exactly, and a
	// song's album in its tags is often one LRCLib doesn't have, as a
	// compilation or a reissue. It still tells the best result apart.
	var rs []Result
	if err := c.do(ctx, "/api/search?"+q.Encode(), &rs); err != nil {
		return nil, err
	}
	return rs, nil
}

func (c *Client) do(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(c.BaseURL, "/")+path, nil)
	if err != nil {
		return err
	}
	if c.UserAgent != "" {
		req.Header.Set("User-Agent", c.UserAgent)
	}
	req.Header.Set("Accept", "application/json")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	switch res.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound, http.StatusBadRequest:
		// 400 says /api/get wanted more than it was given, which search can
		// still answer.
		return errNotFound
	default:
		return fmt.Errorf("lrclib: %s", res.Status)
	}
	return json.NewDecoder(io.LimitReader(res.Body, 8<<20)).Decode(out)
}

// pick is the result of rs that best fits t: one with timed words, whose
// title, artist and length are the song's.
func pick(rs []Result, t Track) *Result {
	var best *Result
	bestScore := 0
	for i := range rs {
		r := &rs[i]
		if r.SyncedLyrics == "" || r.Instrumental {
			continue
		}
		score := matchScore(*r, t)
		if best == nil || score > bestScore {
			best, bestScore = r, score
		}
	}
	if best == nil || bestScore < 2 {
		return nil
	}
	return best
}

func matchScore(r Result, t Track) int {
	score := 0
	title, artist, album := hanzi.ToSimplified(r.TrackName), hanzi.ToSimplified(r.ArtistName), hanzi.ToSimplified(r.AlbumName)
	if same(title, t.Title) {
		score += 3
	} else if contains(title, t.Title) {
		score++
	}
	if same(artist, t.Artist) {
		score += 3
	} else if contains(artist, t.Artist) {
		score++
	}
	if t.Album != "" && same(album, t.Album) {
		score++
	}
	if t.Duration > 0 && r.Duration > 0 {
		d := t.Duration - time.Duration(r.Duration*float64(time.Second))
		if d < 0 {
			d = -d
		}
		switch {
		case d <= 2*time.Second:
			score += 2
		case d <= 5*time.Second:
			score++
		case d > 15*time.Second:
			score -= 3
		}
	}
	return score
}

// trackNo is a track number before a title, as rippers write it in Chinese:
// "08梵高先生", " 05 天空之城", "03-和你在一起". A number of three digits with
// nothing after it is a title's own, as "365天" is.
var trackNo = regexp.MustCompile(`^\s*(\d{1,3})(\s*[-._、]\s*|\s+)?`)

// cleanTitle drops a track number before a Chinese title. A title of another
// script keeps it: "7 Rings" begins with one.
func cleanTitle(title string) string {
	m := trackNo.FindStringSubmatchIndex(title)
	if m == nil {
		return title
	}
	digits, sep := title[m[2]:m[3]], ""
	if m[4] >= 0 {
		sep = title[m[4]:m[5]]
	}
	rest := strings.TrimSpace(title[m[1]:])
	if rest == "" || (sep == "" && len(digits) > 2) {
		return title
	}
	if r, _ := utf8.DecodeRuneInString(rest); !unicode.Is(unicode.Han, r) {
		return title
	}
	return rest
}

func norm(s string) string { return strings.Join(strings.Fields(strings.ToLower(s)), " ") }

func same(a, b string) bool { return a != "" && b != "" && norm(a) == norm(b) }

func contains(a, b string) bool {
	a, b = norm(a), norm(b)
	return a != "" && b != "" && strings.Contains(a, b)
}
