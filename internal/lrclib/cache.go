package lrclib

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

// How long what was found, or not found, is good for. A song's lyrics don't
// change; a song LRCLib didn't have may be uploaded later.
const (
	foundFor    = 90 * 24 * time.Hour
	notFoundFor = 7 * 24 * time.Hour
)

type entry struct {
	LRC   string    `json:"lrc,omitempty"`
	Found bool      `json:"found"`
	At    time.Time `json:"at"`
}

// A Cache keeps what was found in files in Dir, and asks LRCLib once for
// songs asked for at the same time.
type Cache struct {
	Dir string
	// Now is the time, which tests replace.
	Now func() time.Time

	mu     sync.Mutex
	flying map[string]chan struct{}
}

// NewCache returns a cache keeping its files in dir.
func NewCache(dir string) *Cache {
	return &Cache{Dir: dir, Now: time.Now, flying: map[string]chan struct{}{}}
}

// Lookup answers the timed lyrics of t, from the cache if they are there and
// still good, or else from LRCLib, whose answer it keeps. It answers "" and
// no error when there are none.
func (c *Cache) Lookup(ctx context.Context, client *Client, t Track) (string, error) {
	key := cacheKey(t)
	old, had := c.read(key)
	if had && !c.stale(old) {
		return old.LRC, nil
	}

	c.mu.Lock()
	if ch, ok := c.flying[key]; ok {
		c.mu.Unlock()
		select {
		case <-ch:
		case <-ctx.Done():
			return "", ctx.Err()
		}
		if e, ok := c.read(key); ok {
			return e.LRC, nil
		}
		return "", nil
	}
	ch := make(chan struct{})
	c.flying[key] = ch
	c.mu.Unlock()

	lrc, err := client.Lookup(ctx, t)
	if err == nil {
		c.write(key, entry{LRC: lrc, Found: lrc != "", At: c.now()})
	}

	c.mu.Lock()
	delete(c.flying, key)
	close(ch)
	c.mu.Unlock()

	if err != nil {
		// What was found before is better than nothing while LRCLib can't be
		// reached.
		if had {
			return old.LRC, nil
		}
		return "", err
	}
	return lrc, nil
}

func (c *Cache) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func (c *Cache) stale(e entry) bool {
	age := c.now().Sub(e.At)
	if !e.Found {
		return age > notFoundFor
	}
	return age > foundFor
}

func (c *Cache) path(key string) string { return filepath.Join(c.Dir, key+".json") }

func (c *Cache) read(key string) (entry, bool) {
	b, err := os.ReadFile(c.path(key))
	if err != nil {
		return entry{}, false
	}
	var e entry
	if json.Unmarshal(b, &e) != nil || e.At.IsZero() {
		return entry{}, false
	}
	return e, true
}

func (c *Cache) write(key string, e entry) {
	if err := os.MkdirAll(c.Dir, 0o755); err != nil {
		return
	}
	b, err := json.Marshal(e)
	if err != nil {
		return
	}
	tmp := c.path(key) + ".part"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return
	}
	if err := os.Rename(tmp, c.path(key)); err != nil {
		os.Remove(tmp)
	}
}

// cacheVersion changes when what a lookup asks, or how it picks its answer,
// does: a song LRCLib had all along isn't kept without lyrics by an answer
// an older version found.
const cacheVersion = 2

func cacheKey(t Track) string {
	// Length may be a second or two off from LRCLib's, as the tags and the
	// audio disagree: five-second buckets keep near lengths together.
	length := int64(t.Duration.Round(5*time.Second) / time.Second)
	key := strconv.Itoa(cacheVersion) + "\x00" + norm(t.Title) + "\x00" + norm(t.Artist) + "\x00" + norm(t.Album) + "\x00" + strconv.FormatInt(length, 10)
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:16])
}
