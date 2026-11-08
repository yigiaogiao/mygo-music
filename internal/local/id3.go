package local

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/dhowden/tag"
)

// The tag library reads an ID3v2 tag as a whole, and gives up on all of it
// when one frame is malformed: a USLT of odd-length UTF-16 loses a file its
// title, artist and lyrics alike. This reads the frames one by one instead,
// keeping what each does say, and finds the timed words the library passes
// over: SYLT, and lyrics in a TXXX or in the lyricist's TEXT frame.

// readID3 reads the ID3v2 tag at the start of r, if there's one and it's of a
// version read here. A frame it can't make out is left out, not the rest.
func readID3(r io.ReaderAt) (t tags) {
	defer func() {
		if recover() != nil {
			// Whatever was read before the odd frame stands.
		}
	}()
	var h [10]byte
	if _, err := r.ReadAt(h[:], 0); err != nil || string(h[:3]) != "ID3" {
		return t
	}
	version := int(h[3])
	if version < 2 || version > 4 {
		return t
	}
	if version == 2 && h[5]&0x40 != 0 { // the whole tag compressed
		return t
	}
	size := int(syncsafe(h[6:10]))
	if size > 32<<20 {
		return t // a tag that big is no tag
	}
	body := make([]byte, size)
	n, _ := r.ReadAt(body, 10)
	body = body[:n]
	if h[5]&0x80 != 0 {
		body = deunsync(body)
	}
	off := 0
	if h[5]&0x40 != 0 { // an extended header
		if len(body) < 4 {
			return t
		}
		if version == 4 {
			off = int(syncsafe(body[:4])) // includes its own size
		} else {
			off = 4 + int(binary.BigEndian.Uint32(body[:4]))
		}
	}
	for off >= 0 && off < len(body) {
		id, data, next, ok := id3FrameAt(body, off, version)
		if !ok {
			break
		}
		off = next
		t.addFrame(id, data, version)
	}
	return t
}

// id3FrameAt reads the frame at off of a tag's body: its ID, its data, and
// where the next frame starts.
func id3FrameAt(body []byte, off, version int) (id string, data []byte, next int, ok bool) {
	var size int
	var flags uint16
	if version == 2 {
		if off+6 > len(body) {
			return "", nil, 0, false
		}
		id = string(body[off : off+3])
		size = int(body[off+3])<<16 | int(body[off+4])<<8 | int(body[off+5])
		next = off + 6
	} else {
		if off+10 > len(body) {
			return "", nil, 0, false
		}
		id = string(body[off : off+4])
		sz := body[off+4 : off+8]
		if version == 4 {
			size = int(syncsafe(sz))
			// Taggers write v2.4 sizes plainly too; take that if it's the
			// one that fits.
			if plain := int(binary.BigEndian.Uint32(sz)); size > len(body)-off-10 && plain <= len(body)-off-10 {
				size = plain
			}
		} else {
			size = int(binary.BigEndian.Uint32(sz))
		}
		flags = uint16(body[off+8])<<8 | uint16(body[off+9])
		next = off + 10
	}
	if !id3FrameID(id) || size <= 0 {
		return "", nil, 0, false // padding, or the tag's end
	}
	if size > len(body)-next {
		size = len(body) - next // cut short
	}
	data = body[next : next+size]
	next += size
	// Frames that say they're compressed, encrypted or grouped are passed
	// over: what they hold is not the words.
	switch version {
	case 2:
		if data[0]&0x40 != 0 {
			return "", nil, 0, false
		}
	case 3:
		if flags&0xe000 != 0 {
			return "", nil, next, true
		}
	case 4:
		if flags&0x000c != 0 {
			return "", nil, next, true
		}
		if flags&0x0040 != 0 { // a group ID comes first
			if len(data) < 1 {
				return "", nil, next, true
			}
			data = data[1:]
		}
		if flags&0x0001 != 0 { // a data length indicator
			if len(data) < 4 {
				return "", nil, next, true
			}
			data = data[4:]
		}
		if flags&0x0002 != 0 {
			data = deunsync(data)
		}
	}
	return id, data, next, true
}

func id3FrameID(id string) bool {
	if id == "" {
		return false
	}
	for _, c := range id {
		if !(c >= 'A' && c <= 'Z' || c >= '0' && c <= '9') {
			return false
		}
	}
	return true
}

// addFrame keeps what a frame says.
func (t *tags) addFrame(id string, data []byte, version int) {
	if len(data) == 0 {
		return
	}
	if version == 2 {
		id = strings.NewReplacer(
			"TT2", "TIT2", "TP1", "TPE1", "TAL", "TALB", "TP2", "TPE2",
			"TYE", "TYER", "TRK", "TRCK", "TPA", "TPOS", "ULT", "USLT",
			"TXX", "TXXX", "PIC", "APIC", "SLT", "SYLT",
		).Replace(id)
	}
	switch id {
	case "TIT2":
		t.setTitle(id3Text(data))
	case "TPE1":
		t.setArtist(id3Text(data))
	case "TALB":
		t.setAlbum(id3Text(data))
	case "TPE2":
		t.setAlbumArtist(id3Text(data))
	case "TYER", "TDRC":
		if v := id3Text(data); t.year == 0 && len(v) >= 4 {
			t.year, _ = strconv.Atoi(v[:4])
		}
	case "TRCK":
		if v, _, _ := strings.Cut(id3Text(data), "/"); t.track == 0 {
			t.track, _ = strconv.Atoi(strings.TrimSpace(v))
		}
	case "TPOS":
		if v, _, _ := strings.Cut(id3Text(data), "/"); t.disc == 0 {
			t.disc, _ = strconv.Atoi(strings.TrimSpace(v))
		}
	case "USLT":
		// Timed words are what the player can follow; plain ones only when
		// nothing timed turns up.
		if v := id3USLT(data); v != "" && (t.lyrics == "" || lrcTime.MatchString(v) && !lrcTime.MatchString(t.lyrics)) {
			t.lyrics = v
		}
	case "SYLT":
		if v := id3SYLT(data); v != "" && !lrcTime.MatchString(t.lyrics) {
			t.lyrics = v
		}
	case "TXXX":
		desc, value := id3TXXX(data)
		switch strings.ToLower(desc) {
		case "replaygain_track_gain":
			t.gain = parseGain(value)
		case "replaygain_track_peak":
			t.peak = parseGain(value)
		default:
			// TXXX is where taggers put lyrics the standard has no frame
			// for: "LYRICS", "UNSYNCEDLYRICS", "LYRICS-XXX".
			if strings.Contains(strings.ToLower(desc), "lyric") && (!lrcTime.MatchString(t.lyrics) || t.lyrics == "") {
				t.lyrics = value
			}
		}
	case "TEXT":
		// The lyricist's frame, which some taggers fill with the words
		// themselves, timed.
		if v := id3Text(data); lrcTime.MatchString(v) && !lrcTime.MatchString(t.lyrics) {
			t.lyrics = v
		}
	case "APIC":
		if t.picture == nil {
			t.picture = id3Picture(data, version == 2)
		}
	}
}

func (t *tags) setTitle(v string) {
	if t.title == "" {
		t.title = cleanTag(v)
	}
}

func (t *tags) setArtist(v string) {
	if t.artist == "" {
		t.artist = cleanTag(v)
	}
}

func (t *tags) setAlbum(v string) {
	if t.album == "" {
		t.album = cleanTag(v)
	}
}

func (t *tags) setAlbumArtist(v string) {
	if t.albumArtist == "" {
		t.albumArtist = cleanTag(v)
	}
}

// id3Text is the text of a frame body, whose first byte says its encoding.
func id3Text(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	s, _ := id3String(body[0], body[1:])
	return s
}

// id3String reads an encoded string, and the bytes after its terminator, as
// some frames hold two strings.
func id3String(enc byte, b []byte) (string, []byte) {
	switch enc {
	case 1, 2:
		be := enc == 2
		if enc == 1 && len(b) >= 2 {
			switch {
			case b[0] == 0xff && b[1] == 0xfe:
				be = false
				b = b[2:]
			case b[0] == 0xfe && b[1] == 0xff:
				be = true
				b = b[2:]
			default:
				be = noBOMBigEndian(b)
			}
		}
		var units []uint16
		i := 0
		for ; i+1 < len(b); i += 2 {
			var u uint16
			if be {
				u = uint16(b[i])<<8 | uint16(b[i+1])
			} else {
				u = uint16(b[i+1])<<8 | uint16(b[i])
			}
			if u == 0 {
				i += 2
				break
			}
			units = append(units, u)
		}
		return string(utf16.Decode(units)), b[min(i, len(b)):]
	default:
		i := bytes.IndexByte(b, 0)
		if i < 0 {
			i = len(b)
		}
		text := string(b[:i])
		if i < len(b) {
			i++
		}
		return decodeLegacy(text), b[i:]
	}
}

// noBOMBigEndian says which way UTF-16 goes when its BOM is missing: Latin
// text keeps its high half in zeros, at odd places in little-endian.
func noBOMBigEndian(b []byte) bool {
	var even, odd int
	for i := 0; i+1 < len(b); i += 2 {
		if b[i] == 0 {
			even++
		}
		if b[i+1] == 0 {
			odd++
		}
	}
	return even >= odd
}

// id3USLT is the words of an unsynced lyrics frame: its language and content
// descriptor first.
func id3USLT(body []byte) string {
	if len(body) < 4 {
		return ""
	}
	_, rest := id3String(body[0], body[4:])
	text, _ := id3String(body[0], rest)
	return strings.TrimRight(text, "\x00 \t\r\n")
}

// id3SYLT makes LRC of a synced lyrics frame, whose strings are timestamped
// in milliseconds. Frames counted in MPEG frames are passed over: their
// length depends on a bit rate this doesn't know.
func id3SYLT(body []byte) string {
	if len(body) < 6 || body[4] != 2 {
		return ""
	}
	_, rest := id3String(body[0], body[6:])
	var b strings.Builder
	for len(rest) > 0 {
		text, after := id3String(body[0], rest)
		if len(after) < 4 {
			break
		}
		ms := int64(binary.BigEndian.Uint32(after[:4]))
		rest = after[4:]
		if text = strings.TrimSpace(text); text == "" {
			continue
		}
		fmt.Fprintf(&b, "[%02d:%02d.%02d]%s\n", ms/60000, ms/1000%60, ms/10%100, text)
	}
	return b.String()
}

// id3TXXX is the description and value of a user frame.
func id3TXXX(body []byte) (desc, value string) {
	desc, rest := id3String(body[0], body[1:])
	value, _ = id3String(body[0], rest)
	return strings.TrimSpace(desc), strings.TrimRight(value, "\x00")
}

// id3Picture reads a picture frame: its MIME type, its kind, and the picture,
// or in v2.2 its three-letter format instead of a MIME type.
func id3Picture(body []byte, v22 bool) *tag.Picture {
	if len(body) < 4 {
		return nil
	}
	enc := body[0]
	var mime string
	var rest []byte
	if v22 {
		format := string(body[1:4])
		rest = body[4:]
		mime = map[string]string{"JPG": "image/jpeg", "PNG": "image/png"}[strings.ToUpper(format)]
		if mime == "" {
			mime = "image/" + strings.ToLower(format)
		}
	} else {
		i := bytes.IndexByte(body[1:], 0)
		if i < 0 {
			return nil
		}
		mime = string(body[1 : 1+i])
		rest = body[1+i+1:]
	}
	if len(rest) < 2 {
		return nil
	}
	_, data := id3String(enc, rest[1:])
	if len(data) == 0 {
		return nil
	}
	return &tag.Picture{MIMEType: mime, Data: data}
}

// syncsafe reads the four bytes of an ID3 size, seven bits each.
func syncsafe(b []byte) uint32 {
	if len(b) < 4 {
		return 0
	}
	return uint32(b[0]&0x7f)<<21 | uint32(b[1]&0x7f)<<14 | uint32(b[2]&0x7f)<<7 | uint32(b[3]&0x7f)
}

// deunsync undoes the 0x00 ID3 puts after a 0xff, for a player to tell its
// bytes from a frame's start.
func deunsync(b []byte) []byte {
	out := make([]byte, 0, len(b))
	for i := 0; i < len(b); i++ {
		out = append(out, b[i])
		if b[i] == 0xff && i+1 < len(b) && b[i+1] == 0 {
			i++
		}
	}
	return out
}
