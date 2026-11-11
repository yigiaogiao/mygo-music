//go:build js

// This file is a local patch to the vendored MyGo, not upstream code: the
// web backend turned pointer events into presses and moves but never into
// scrolls, so a phone, which has no wheel, could not scroll at all. It is
// wired up in surface.go, and both files must be kept when the vendored
// module is updated.

package web

import (
	"math"
	"time"

	"github.com/egoist/mygo/internal/platform"
)

// touchSlop is how far a finger may move before its press becomes a
// drag, in DIPs, as Linux's surface has it.
const touchSlop = 8

// touchGesture follows a finger on a touchscreen, which the browser
// reports as pointer events: the press goes to the content at once, as a
// mouse's does, so that buttons and sliders respond, and a finger moving
// further than touchSlop drags a widget sideways or scrolls the content
// under it. A drag that scrolls takes the press back with PointerCancel
// before the first scroll, so that lifting the finger clicks nothing, as
// a browser scrolls before it clicks; a sideways drag keeps the press,
// which a slider follows, and scrolls a horizontal list by DX.
//
// This is the web's version of internal/linux/touch.go, which sees the
// fingers themselves rather than pointer events.
type touchGesture struct {
	active bool
	// pending is set while the press sent to the content waits for the
	// release that will commit it; scrolling takes it back.
	pending bool
	// startX and startY are where the finger landed; x and y are where
	// its last move went, which the next scroll is measured from.
	startX, startY float64
	x, y           float64
	// scrolling is set once the drag went along the content, and no
	// release follows for the press taken back.
	scrolling bool
	// dragging is set once the drag went across, keeping the press for a
	// slider and scrolling a horizontal list.
	dragging bool
	// vx and vy are the finger's velocity in the direction the content
	// scrolls, in DIPs per millisecond, smoothed over the moves, and
	// lastT is the time of the last one, in milliseconds.
	vx, vy, lastT float64
}

// begin starts following the finger that landed at x, y.
func (g *touchGesture) begin(x, y float64) {
	g.active, g.pending = true, true
	g.startX, g.startY, g.x, g.y = x, y, x, y
	g.scrolling, g.dragging = false, false
	g.vx, g.vy, g.lastT = 0, 0, 0
}

// move follows the finger to x, y, at time t in milliseconds. It reports
// whether the press is still the content's to move, which a drag across
// does and a scroll does not: the caller sends PointerMove then, before
// the scroll of a sideways drag.
func (g *touchGesture) move(x, y, t float64, send func(platform.SurfaceEvent) bool) (press bool) {
	if !g.active {
		return false
	}
	if !g.scrolling && !g.dragging {
		dx, dy := x-g.startX, y-g.startY
		if math.Hypot(dx, dy) < touchSlop {
			return g.pending
		}
		if math.Abs(dy) >= math.Abs(dx) {
			// Along the content: take the press back first, so that the
			// lift clicks nothing where the finger lands.
			if g.pending {
				send(platform.SurfaceEvent{Kind: platform.PointerCancel})
				g.pending = false
			}
			g.scrolling = true
		} else {
			g.dragging = true
		}
	}
	if g.scrolling {
		g.scrollTo(x, y, t, send, 1, 1)
		return false
	}
	// Across: the press moves the slider under it, and the horizontal
	// list under it scrolls, if one is there.
	g.scrollTo(x, y, t, send, 1, 0)
	return g.pending
}

// scrollTo scrolls by the move from where the finger last was to x, y at
// time t, keeping the part of it along the given axes: 1 scrolls that
// axis, 0 leaves it. Positive DY moves the view down the content, so a
// finger moving up scrolls down.
func (g *touchGesture) scrollTo(x, y, t float64, send func(platform.SurfaceEvent) bool, keepX, keepY float64) {
	dx, dy := 0.0, 0.0
	if keepX != 0 && x != g.x {
		dx = g.x - x
	}
	if keepY != 0 && y != g.y {
		dy = g.y - y
	}
	if dx != 0 || dy != 0 {
		send(platform.SurfaceEvent{Kind: platform.PointerScroll, X: g.x, Y: g.y, DX: dx, DY: dy, Precise: true})
	}
	// The velocity of the content, which a fling goes on from: smoothed
	// over the moves, and from the whole move from the last one.
	if t > 0 && (dx != 0 || dy != 0) {
		dt := 16.0
		if t > g.lastT && g.lastT > 0 {
			dt = t - g.lastT
		}
		dt = max(1, min(dt, 100))
		if g.lastT > 0 {
			g.vx = 0.6*g.vx + 0.4*dx/dt
			g.vy = 0.6*g.vy + 0.4*dy/dt
		} else {
			g.vx, g.vy = dx/dt, dy/dt
		}
		g.lastT = t
	}
	g.x, g.y = x, y
}

// end lifts the finger at x, y at time t, reporting whether the caller
// sends the release: a tap, or a sideways drag, whose press is still
// outstanding. A scroll took its press back and sends no release; the
// caller may go on with its velocity.
func (g *touchGesture) end(x, y, t float64, send func(platform.SurfaceEvent) bool) bool {
	if !g.active {
		return false
	}
	if g.scrolling {
		g.scrollTo(x, y, t, send, 1, 1)
	}
	g.active = false
	return g.pending
}

// stopFling stops the content a lifted finger left moving.
func (s *surface) stopFling() {
	s.flingGen++
}

// fling scrolls on from a finger lifted at x, y with velocity vx, vy in
// DIPs per millisecond, slowing to a stop over about half a second, as a
// browser gives a scroll on a phone. It takes a flick, not a drag that
// stopped before it lifted.
func (s *surface) fling(vx, vy, x, y float64) {
	const (
		frame = 16 * time.Millisecond
		decay = 0.95
		least = 0.05 // DIPs per millisecond
	)
	if math.Hypot(vx, vy) < 0.2 {
		return
	}
	s.flingGen++
	gen := s.flingGen
	go func() {
		for {
			time.Sleep(frame)
			done := true
			s.w.b.sync(func() {
				if s.closed || s.flingGen != gen {
					return
				}
				vx, vy = vx*decay, vy*decay
				if math.Hypot(vx, vy) < least {
					return
				}
				done = false
				s.send(platform.SurfaceEvent{Kind: platform.PointerScroll, X: x, Y: y, DX: vx * 16, DY: vy * 16, Precise: true, Momentum: true})
			})
			if done {
				return
			}
		}
	}()
}

// cancel ends the gesture without a release, as pointercancel: the
// system took the pointer, and a press still pending commits nothing.
func (g *touchGesture) cancel(x, y float64, send func(platform.SurfaceEvent) bool) {
	if !g.active {
		return
	}
	if g.pending {
		send(platform.SurfaceEvent{Kind: platform.PointerCancel, X: x, Y: y})
	}
	g.active = false
}
