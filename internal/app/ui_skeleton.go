package app

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// Skeletons stand in for what is loading: gray blocks in its shape, which
// it takes the place of without moving anything, and over them a lighter
// band sweeping, across all the blocks of a view in step.

// shimmer is how the blocks of a skeleton look.
type shimmer struct {
	base, sheen ui.Color
	span        float32 // the width the band crosses: the window's
	still       bool    // the desktop asks for less motion: no band
}

const (
	shimmerPeriod = 1600 * time.Millisecond
	shimmerSweep  = 1100 * time.Millisecond // of the period, the band crossing; then it rests
	shimmerBand   = 260                     // DIPs
)

// pageShimmer is the shimmer of the pages, on their background.
func (a *app) pageShimmer(c *ui.Context) shimmer {
	s := shimmer{base: ui.RGBA(0, 0, 0, 0.06), sheen: ui.RGBA(0, 0, 0, 0.022)}
	if a.pal.dark {
		s.base, s.sheen = white.Alpha(0.07), white.Alpha(0.13)
	}
	return s.in(c)
}

// npShimmer is the shimmer over the now playing view's backdrop.
func npShimmer(c *ui.Context) shimmer {
	return shimmer{base: white.Alpha(0.1), sheen: white.Alpha(0.2)}.in(c)
}

func (s shimmer) in(c *ui.Context) shimmer {
	s.span, _ = c.Size()
	s.still = c.Preferences().ReduceMotion
	return s
}

// paint fills r, its corners rounded by radius, as a block of the skeleton.
func (s shimmer) paint(p *ui.Painter, r ui.Rect, radius float32) {
	if s.still || r.W <= 0 {
		p.Fill(r, s.base, radius)
		return
	}
	at := time.Duration(p.Now().UnixNano() % int64(shimmerPeriod))
	if at >= shimmerSweep {
		p.Fill(r, s.base, radius)
		p.After(shimmerPeriod - at)
		return
	}
	p.AnimationFrameRate(60)
	t := float32(at) / float32(shimmerSweep)
	x := -shimmerBand/2 + t*(s.span+shimmerBand)
	if x+shimmerBand/2 < r.X || x-shimmerBand/2 > r.X+r.W {
		p.Fill(r, s.base, radius)
		return
	}
	// The band's place along the block, which it may lie partly beyond.
	along := func(x float32) float32 { return (x - r.X) / r.W }
	p.FillGradient(r, ui.LinearGradient{Angle: 90, Stops: []ui.GradientStop{
		{Color: s.base, At: along(x - shimmerBand/2)},
		{Color: s.sheen, At: along(x)},
		{Color: s.base, At: along(x + shimmerBand/2)},
	}}, radius)
}

// bone is a block of the skeleton, as big as it is laid out.
func (s shimmer) bone(c *ui.Context, radius float32) ui.Element {
	return ui.Box(c).Shrink(0).Draw(func(p *ui.Painter, r ui.Rect) { s.paint(p, r, radius) })
}

// text is a line of text at size, as tall as the line and as wide as it
// is laid out, its block the height of the letters.
func (s shimmer) text(c *ui.Context, size, lineHeight float32) ui.Element {
	return ui.Box(c).Shrink(0).Height(size * lineHeight).Draw(func(p *ui.Painter, r ui.Rect) {
		h := size * 0.78
		s.paint(p, ui.Rect{X: r.X, Y: r.Y + (r.H-h)/2, W: r.W, H: h}, h*0.32)
	})
}

// boneWidths vary the lines of a skeleton, as the lengths of what they
// stand for do, as percentages.
var boneWidths = [...]float32{62, 44, 78, 53, 70, 38, 66, 49, 82, 57, 41, 73}

func boneWidth(i int) float32 { return boneWidths[i%len(boneWidths)] }

// Skeletons of the pages, shaped as homePage, trackRows and the cards they
// show.

// homeSkeleton is homePage's cards while they load, or with feature, as
// browseRows starts a source's own Home: the card of its first songs, then a
// row of cards under a title.
func (a *app) homeSkeleton(c *ui.Context, s shimmer, feature bool) {
	ui.Column(c.Key("home-skeleton")).PaddingX(12).Children(func() {
		if !feature {
			a.playlistCardsSkeleton(c.Key("cards"), s, a.columns(c, 168)*2, 168, false)
			return
		}
		s.bone(c, dailyRadius).Height(dailyHeight)
		ui.Box(c).Margin(34, 0, 14, 0).Children(func() { s.text(c, 19, 1.3).Width(120) })
		a.playlistCardsSkeleton(c.Key("cards"), s, a.columns(c, 156), 156, false)
	})
}

// playlistCardsSkeleton is n of playlistCard's cards, in columns at least
// min DIPs wide, with a line for how often they update if freq.
func (a *app) playlistCardsSkeleton(c *ui.Context, s shimmer, n int, min float32, freq bool) {
	ui.Grid(c).Columns(a.columns(c, min)).GapX(20).GapY(28).Children(func() {
		for i := range n {
			ui.Column(c.Key(i)).Gap(10).AlignItems(ui.Stretch).Children(func() {
				s.bone(c, 10).AspectRatio(1)
				ui.Column(c).Gap(0).Children(func() {
					s.text(c, 13, 1.35).WidthPercent(92)
					s.text(c, 13, 1.35).WidthPercent(boneWidth(i))
				})
				if freq {
					s.text(c, 12, 1.3).WidthPercent(40).Margin(-6, 0, 0, 0)
				}
			})
		}
	})
}

// trackSkeleton is trackRows while its tracks load: rows of blocks under
// the header, which may hold blocks of its own.
func (a *app) trackSkeleton(c *ui.Context, s shimmer, covers bool, header func(c *ui.Context)) {
	w, h := c.Size()
	wide := !a.mobile(c) && w-sidebarWidth > 820
	top, bottom := a.insets(c)
	a.underTop, a.underBottom = false, true
	ui.Column(c.Key("skeleton")).Grow(1).Padding(top+pageGap, a.pagePadding(c)-12, bottom+32, a.pagePadding(c)-12).Children(func() {
		a.startMark(c)
		header(c)
		for i := range max(8, int(h/54)) {
			ui.Row(c.Key(i)).Height(54).Shrink(0).PaddingX(12).Gap(14).AlignItems(ui.Center).Children(func() {
				ui.Box(c).Width(26).Center().Children(func() { s.text(c, 13, 1.3).Width(12) })
				if covers {
					s.bone(c, 6).Size(38, 38)
				}
				ui.Column(c).Grow(3).Basis(0).MinWidth(0).Gap(3).Children(func() {
					s.text(c, 14, 1.3).WidthPercent(boneWidth(i) * 0.8)
					s.text(c, 12.5, 1.3).WidthPercent(boneWidth(i+5) * 0.55)
				})
				if wide {
					ui.Box(c).Grow(2).Basis(0).MinWidth(0).Children(func() {
						s.text(c, 12.5, 1.3).WidthPercent(boneWidth(i+3) * 0.8)
					})
				}
				ui.Box(c).Size(32, 32)
				ui.Row(c).Width(44).Justify(ui.End).Children(func() { s.text(c, 12.5, 1.3).Width(30) })
			})
		}
	})
}

// playlistHeaderSkeleton is playlistHeader while the playlist loads.
func (a *app) playlistHeaderSkeleton(c *ui.Context, s shimmer) {
	ui.Row(c).Gap(a.headerGap(c)).AlignItems(ui.End).Padding(0, 0, 26, 0).Children(func() {
		s.bone(c, 14).Size(a.headerCover(c), a.headerCover(c))
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(8).Children(func() {
			s.text(c, 12, 1.3).Width(36)
			s.text(c, 30, 1.2).WidthPercent(48)
			s.text(c, 13, 1.3).Width(180)
			ui.Column(c).Margin(2, 0, 0, 0).Children(func() {
				s.text(c, 12.5, 1.45).WidthPercent(78)
				s.text(c, 12.5, 1.45).WidthPercent(52)
			})
			ui.Row(c).Gap(10).Margin(10, 0, 0, 0).Children(func() {
				s.bone(c, 18).Size(92, 36)
				s.bone(c, 18).Size(120, 36)
			})
		})
	})
}

// Skeletons of the now playing view.

// commentSkeleton is n of commentRow's comments.
func commentSkeleton(c *ui.Context, s shimmer, n int) {
	for i := range n {
		ui.Row(c.Key(i)).Padding(12).Gap(12).AlignItems(ui.Start).Children(func() {
			s.bone(c, 17).Size(34, 34)
			ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(6).Children(func() {
				s.text(c, 13, 1.3).Width(60 + boneWidth(i))
				ui.Column(c).Children(func() {
					s.text(c, 15, 1.55).WidthPercent(94)
					if i%3 != 2 {
						s.text(c, 15, 1.55).WidthPercent(boneWidth(i + 2))
					}
				})
				s.text(c, 11.5, 1.3).Width(96)
			})
		})
	}
}

// lyricsSkeleton is lyricsPane's lines while they load.
func lyricsSkeleton(c *ui.Context, s shimmer) {
	ui.Column(c).Gap(6).Padding(0, 8, 0, 0).Children(func() {
		for i := range 7 {
			ui.Box(c.Key(i)).Padding(10, 12).Children(func() {
				s.text(c, 30, 1.25).WidthPercent(boneWidth(i+1) * 0.9)
			})
		}
	})
}

// stageSkeleton is stageLine's words while the lyrics load.
func stageSkeleton(c *ui.Context, s shimmer, f, w float32) {
	ui.Row(c).Wrap().Justify(ui.Center).AlignItems(ui.Center).Padding(0, w*0.1).Gap(f * 0.35).Children(func() {
		for i, n := range []float32{2.2, 1.3, 3.1, 1.8} {
			s.bone(c.Key(i), f*0.22).Size(f*n, f*0.85)
		}
	})
}
