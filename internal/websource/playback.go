package websource

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"mygo-music/internal/i18n"
	"mygo-music/internal/playback"
)

// Playback asks the server what the listener's devices play together, and
// who may pick it up.
func (s *Source) Playback() (*playback.State, error) {
	return s.playback(http.MethodGet, nil)
}

// ReportPlayback tells the server what this device plays, or asks it to hand
// the session to another device, and answers the state it has after.
func (s *Source) ReportPlayback(rep playback.Report) (*playback.State, error) {
	b, err := json.Marshal(rep)
	if err != nil {
		return nil, err
	}
	return s.playback(http.MethodPost, b)
}

// playback asks or tells the server, naming this device with what the answer
// named it last. The browser sends the gateway's device header itself, and
// its cookie; the header covers a server that named this one first.
func (s *Source) playback(method string, body []byte) (*playback.State, error) {
	ctx, cancel := context.WithTimeout(s.ctx, 15*time.Second)
	defer cancel()
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, s.base+"/api/playback", r)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("X-MyGo-Language", i18n.Language())
	s.mu.Lock()
	device := s.device
	s.mu.Unlock()
	if device != "" {
		req.Header.Set("X-MyGo-Device", device)
	}
	res, err := s.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 4<<10))
		msg := strings.TrimSpace(string(b))
		if msg == "" {
			msg = res.Status
		}
		return nil, fmt.Errorf("playback: %s", msg)
	}
	var st playback.State
	if err := json.NewDecoder(io.LimitReader(res.Body, 8<<20)).Decode(&st); err != nil {
		return nil, fmt.Errorf("playback: %w", err)
	}
	if st.DeviceID != "" {
		s.mu.Lock()
		s.device = st.DeviceID
		s.mu.Unlock()
	}
	return &st, nil
}
