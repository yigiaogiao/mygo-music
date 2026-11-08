package local

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/dhowden/tag"
)

// rawFrame is an ID3v2.3 frame of data as it is.
func rawFrame(id string, data []byte) []byte {
	b := []byte(id)
	b = binary.BigEndian.AppendUint32(b, uint32(len(data)))
	b = append(b, 0, 0)
	return append(b, data...)
}

// rawID3 is an ID3v2.3 tag of the frames.
func rawID3(frames ...[]byte) []byte {
	body := bytes.Join(frames, nil)
	n := len(body)
	return append([]byte{
		'I', 'D', '3', 3, 0, 0,
		byte(n >> 21 & 0x7f), byte(n >> 14 & 0x7f), byte(n >> 7 & 0x7f), byte(n & 0x7f),
	}, body...)
}

// A malformed USLT loses the library the whole tag; the frames around it are
// still read.
func TestReadID3SkipsAMalformedFrame(t *testing.T) {
	// Odd-length UTF-16, which the library refuses.
	uslt := []byte{1, 'e', 'n', 'g', 0xff, 0xfe, 'a', 0, 0, 0, 'x', 0, 'y'}
	data := rawID3(
		rawFrame("TIT2", append([]byte{3}, "Song"...)),
		rawFrame("USLT", uslt),
		rawFrame("TPE1", append([]byte{3}, "Band"...)),
	)
	if _, err := tag.ReadFrom(bytes.NewReader(data)); err == nil {
		t.Fatal("the library reads the tag, so this doesn't test the fallback")
	}
	got := readID3(bytes.NewReader(data))
	if got.title != "Song" || got.artist != "Band" {
		t.Errorf("read %+v", got)
	}
}

// readTags keeps a file its tags when one frame is malformed.
func TestReadTagsFallsBackPastAMalformedFrame(t *testing.T) {
	uslt := []byte{1, 'e', 'n', 'g', 0xff, 0xfe, 'a', 0, 0, 0, 'x', 0, 'y'}
	data := append(rawID3(
		rawFrame("TIT2", append([]byte{3}, "Song"...)),
		rawFrame("USLT", uslt),
		rawFrame("TPE1", append([]byte{3}, "Band"...)),
	), fixture(t, "seek.mp3")[:4096]...)
	path := write(t, filepath.Join(t.TempDir(), "broken.mp3"), data)
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	got := readTags(f, "mp3", int64(len(data)))
	if got.title != "Song" || got.artist != "Band" {
		t.Errorf("read %+v", got)
	}
}

// Taggers that keep lyrics in a TXXX, as Chinese ones do.
func TestReadID3FindsLyricsInTXXX(t *testing.T) {
	data := rawID3(rawFrame("TXXX", append([]byte{3}, "LYRICS\x00[00:01.00]words"...)))
	if got := readID3(bytes.NewReader(data)); !lrcTime.MatchString(got.lyrics) {
		t.Errorf("lyrics %q", got.lyrics)
	}
}

// The lyricist's frame sometimes holds the words, timed.
func TestReadID3FindsLyricsInTEXT(t *testing.T) {
	data := rawID3(rawFrame("TEXT", append([]byte{3}, "[00:12.00]words"...)))
	if got := readID3(bytes.NewReader(data)); got.lyrics != "[00:12.00]words" {
		t.Errorf("lyrics %q", got.lyrics)
	}
}

// SYLT's milliseconds become LRC's times.
func TestReadID3MakesLRCFromSYLT(t *testing.T) {
	data := []byte{3, 'e', 'n', 'g', 2, 1, 0} // UTF-8, English, milliseconds, lyrics, no descriptor
	data = append(data, "first"...)
	data = append(data, 0)
	data = binary.BigEndian.AppendUint32(data, 1500)
	data = append(data, "second"...)
	data = append(data, 0)
	data = binary.BigEndian.AppendUint32(data, 61000)
	got := readID3(bytes.NewReader(rawID3(rawFrame("SYLT", data))))
	want := "[00:01.50]first\n[01:01.00]second\n"
	if got.lyrics != want {
		t.Errorf("lyrics %q, want %q", got.lyrics, want)
	}
}

// UTF-16 text without a BOM, little-endian: its zeros fall in the high half.
func TestReadID3ReadsUTF16WithoutABOM(t *testing.T) {
	data := rawID3(rawFrame("TIT2", []byte{1, 'S', 0, 'o', 0, 'n', 0, 'g', 0}))
	if got := readID3(bytes.NewReader(data)); got.title != "Song" {
		t.Errorf("title %q", got.title)
	}
}

// A picture comes back even when the library refuses the tag.
func TestReadID3ReadsAPicture(t *testing.T) {
	data := rawID3(rawFrame("APIC", append([]byte("\x00image/jpeg\x00\x03\x00"), jpegData...)))
	got := readID3(bytes.NewReader(data))
	if got.picture == nil || !bytes.Equal(got.picture.Data, jpegData) {
		t.Errorf("picture %+v", got.picture)
	}
}
