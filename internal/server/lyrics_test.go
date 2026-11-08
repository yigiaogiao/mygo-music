package server

import (
	"testing"

	"mygo-music/internal/i18n"
	"mygo-music/internal/music"
)

func TestSimplifyLyricsWritesChineseInSimplified(t *testing.T) {
	old := i18n.Language()
	t.Cleanup(func() { i18n.SetLanguage(old) })

	l := &music.Lyrics{
		LRC:         "[00:01.00]為什麼失眠的聲音",
		Translation: "[00:01.00]Why can't I sleep",
		Lines: []music.LyricLine{{
			Text:  "誰和我一樣",
			Words: []music.LyricWord{{Text: "愛上你"}},
		}},
	}

	i18n.SetLanguage(i18n.Chinese)
	got := simplifyLyrics(l)
	if got.LRC != "[00:01.00]为什么失眠的声音" {
		t.Errorf("LRC %q", got.LRC)
	}
	if got.Translation != "[00:01.00]Why can't I sleep" {
		t.Errorf("Translation %q", got.Translation)
	}
	if got.Lines[0].Text != "谁和我一样" || got.Lines[0].Words[0].Text != "爱上你" {
		t.Errorf("lines %+v", got.Lines[0])
	}

	i18n.SetLanguage(i18n.English)
	got = simplifyLyrics(&music.Lyrics{LRC: "[00:01.00]為什麼失眠的聲音"})
	if got.LRC != "[00:01.00]為什麼失眠的聲音" {
		t.Errorf("English changed the words: %q", got.LRC)
	}
}
