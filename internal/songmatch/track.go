// Package songmatch compares a song with the answers a lyrics service gives
// it: services write Chinese on either side of the strait, and rippers put
// track numbers before titles.
package songmatch

import (
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"mygo-music/internal/hanzi"
)

// Track is what is known of a song to look it up by.
type Track struct {
	Title    string
	Artist   string
	Album    string
	Duration time.Duration
}

// Normalize writes a track the way a service's answers can be compared with
// it: in simplified characters, and a Chinese title without its track number.
func Normalize(t Track) Track {
	t.Title = CleanTitle(hanzi.ToSimplified(t.Title))
	t.Artist = hanzi.ToSimplified(t.Artist)
	t.Album = hanzi.ToSimplified(t.Album)
	return t
}

// trackNo is a track number before a title, as rippers write it in Chinese:
// "08梵高先生", " 05 天空之城", "03-和你在一起". A number of three digits with
// nothing after it is a title's own, as "365天" is.
var trackNo = regexp.MustCompile(`^\s*(\d{1,3})(\s*[-._、]\s*|\s+)?`)

// CleanTitle drops a track number before a Chinese title. A title of another
// script keeps it: "7 Rings" begins with one.
func CleanTitle(title string) string {
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

// Score says how well cand answers target: its title, artist and album, and
// its length. Neither needs to be Normalize's; Score does it. A candidate of
// another length than the song, by more than fifteen seconds, is taken for
// another recording.
func Score(cand, target Track) int {
	cand, target = Normalize(cand), Normalize(target)
	score := 0
	if same(cand.Title, target.Title) {
		score += 3
	} else if contains(cand.Title, target.Title) {
		score++
	}
	if same(cand.Artist, target.Artist) {
		score += 3
	} else if contains(cand.Artist, target.Artist) {
		score++
	}
	if target.Album != "" && same(cand.Album, target.Album) {
		score++
	}
	if target.Duration > 0 && cand.Duration > 0 {
		d := target.Duration - cand.Duration
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

func norm(s string) string { return strings.Join(strings.Fields(strings.ToLower(s)), " ") }

func same(a, b string) bool { return a != "" && b != "" && norm(a) == norm(b) }

func contains(a, b string) bool {
	a, b = norm(a), norm(b)
	return a != "" && b != "" && strings.Contains(a, b)
}
