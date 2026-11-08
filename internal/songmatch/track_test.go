package songmatch

import (
	"testing"
	"time"
)

func TestCleanTitle(t *testing.T) {
	tests := []struct{ in, want string }{
		{"08梵高先生", "梵高先生"},
		{" 05 天空之城", "天空之城"},
		{"03-和你在一起", "和你在一起"},
		{"365天", "365天"},
		{"2002年的第一场雪", "2002年的第一场雪"},
		{"7 Rings", "7 Rings"},
		{"Song", "Song"},
	}
	for _, tt := range tests {
		if got := CleanTitle(tt.in); got != tt.want {
			t.Errorf("CleanTitle(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestScoreTakesTheSongWrittenInTraditional(t *testing.T) {
	song := Track{Title: "08梵高先生", Artist: "李誌", Album: "梵高先生2005", Duration: 250 * time.Second}
	cand := Track{Title: "梵高先生", Artist: "李志", Album: "梵高先生", Duration: 250 * time.Second}
	if got := Score(cand, song); got < 8 {
		t.Errorf("score %d, want the title, artist, album and length", got)
	}
}

func TestScoreRejectsAnotherRecording(t *testing.T) {
	song := Track{Title: "他们", Artist: "李志", Duration: 269 * time.Second}
	cand := Track{Title: "他们", Artist: "Another", Duration: 244 * time.Second}
	if got := Score(cand, song); got >= 2 {
		t.Errorf("score %d, want less than 2", got)
	}
}
