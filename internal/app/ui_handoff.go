package app

import (
	"github.com/egoist/mygo/ui"

	"mygo-music/internal/playback"
)

// devicesButton is the button in the player bar that lists the listener's
// devices, and hands what plays to one of them. It shows in the accent while
// another device is asked to pick the music up.
func (a *app) devicesButton(c *ui.Context, color ui.Color) {
	if !a.handoffReady() {
		return
	}
	p := a.pal
	h := &a.pb
	label := T("Play on Another Device")
	col := color
	if h.state.PendingFrom == h.state.DeviceID && h.state.Pending != "" {
		col = p.accent
	}
	b := iconButton(c.Key("devices"), iconDevices, 16, col, label)
	if b.Clicked() {
		h.picking = !h.picking
		if h.picking {
			a.refreshHandoff()
			a.handoffTick()
		}
	}
	pop := popoverColors(p.dark)
	ui.PopoverBase(c.Key("devices-popover"), b, &h.picking, func(panel ui.Element) {
		panel.AttachTo(b, ui.AnchorTopEnd, ui.AnchorBottomEnd).Margin(0, 0, 8, 0).Width(264).Padding(5).Gap(1).
			Radius(12).Background(pop.fill).Border(0.5, pop.edge).Shadow(0, 8, 24, -4, ui.RGBA(0, 0, 0, 0.2)).
			Role(ui.RoleMenu).FocusGroup(ui.Vertical).Label(label)
		a.deviceList(c, pop)
	})
}

// deviceList is the panel of the devices button: a way to take the music
// over from the device that plays it, the devices the listener has used, and
// what is waiting on a takeover.
func (a *app) deviceList(c *ui.Context, pop popover) {
	p := a.pal
	h := &a.pb
	ui.Text(c, T("Play on Another Device")).FontSize(11).FontWeight(600).TextColor(p.faint).SingleLine().
		Padding(4, 8, 2, 8)
	if h.err != "" {
		ui.Text(c, T("Couldn't reach the playback service.")).FontSize(11).TextColor(p.accent).
			Padding(2, 8, 4, 8).LineHeight(1.35)
	}
	// The other device plays, and this one takes it over, without going to
	// the other device to ask.
	if h.have && h.state.Track != nil && !h.state.Mine() {
		if a.handoffItem(c.Key("continue"), iconPlay, T("Continue on This Device"), true).Clicked() {
			a.continueHandoff()
		}
		ui.Box(c).Height(0.5).Margin(4, 8).Background(pop.edge)
	}
	if !h.have {
		ui.Text(c, T("Looking for your devices…")).FontSize(12).TextColor(p.muted).Padding(3, 8, 5, 8)
		return
	}
	for _, d := range h.state.Devices {
		a.deviceRow(c, pop, d)
	}
	if h.state.PendingFrom == h.state.DeviceID && h.state.Pending != "" {
		ui.Box(c).Height(0.5).Margin(4, 8).Background(pop.edge)
		ui.Text(c, Tf("Waiting for %s to open MyGo Music", deviceLabel(h.state.PendingName))).
			FontSize(11).TextColor(p.muted).Padding(3, 8, 2, 8).LineHeight(1.35)
		if a.handoffItem(c.Key("cancel"), iconX, T("Cancel"), false).Clicked() {
			a.cancelHandoff()
		}
	} else if len(h.state.Devices) < 2 {
		ui.Text(c, T("Open MyGo Music on another device to see it here.")).
			FontSize(11).TextColor(p.faint).Padding(3, 8, 5, 8).LineHeight(1.35)
	}
}

// deviceRow shows a device, and hands the music to it when this one plays it.
func (a *app) deviceRow(c *ui.Context, pop popover, d playback.Device) {
	p := a.pal
	h := &a.pb
	this := d.ID == h.state.DeviceID
	owner := d.ID == h.state.Owner
	name := deviceLabel(d.Name)
	if this {
		name = T("This Device")
	}
	var status string
	col := p.faint
	switch {
	case h.state.Pending == d.ID:
		status, col = T("Waiting…"), p.accent
	case owner && h.state.Playing:
		status, col = T("Playing"), p.accent
	case owner:
		status = T("Paused")
	case this:
	case d.Online:
		status = T("Online")
	default:
		status = T("Offline")
	}
	canHand := !this && h.state.Mine() && h.state.Track != nil
	b := ui.ButtonBase(c.Key("device-" + d.ID)).Role(ui.RoleMenuItem).Label(name + " " + status).
		Height(32).PaddingX(8).Gap(9).Radius(7).AlignItems(ui.Center).Justify(ui.Start).FocusRing(false)
	if canHand && (b.Hovered() || b.FocusVisible()) {
		b.Background(p.press)
	}
	b.Children(func() {
		ui.Icon(c, deviceIcon(d.Kind)).Size(15, 15).TextColor(p.muted)
		ui.Text(c, name).FontSize(13).Grow(1).Shrink(1).MinWidth(0).SingleLine()
		if status != "" {
			ui.Text(c, status).FontSize(11).TextColor(col).SingleLine()
		}
	})
	if canHand && b.Clicked() {
		a.requestHandoff(d.ID, d.Name)
	}
}

// handoffItem is a row of the devices panel that does something rather than
// naming a device.
func (a *app) handoffItem(c *ui.Context, icon *ui.SVG, label string, accent bool) ui.Element {
	p := a.pal
	b := ui.ButtonBase(c).Role(ui.RoleMenuItem).Label(label).Height(32).
		PaddingX(8).Gap(9).Radius(7).AlignItems(ui.Center).Justify(ui.Start).FocusRing(false)
	if b.Hovered() || b.FocusVisible() {
		b.Background(p.press)
	}
	b.Children(func() {
		ui.Icon(c, icon).Size(15, 15).TextColor(p.muted)
		text := ui.Text(c, label).FontSize(13).Grow(1).Shrink(1).MinWidth(0).SingleLine()
		if accent {
			text.TextColor(p.accent).FontWeight(600)
		}
	})
	return b
}

// takeoverDialog asks whether to go on with what another device plays, as
// the listener opened the app there, or tapped to take it over.
func (a *app) takeoverDialog(c *ui.Context) {
	if !a.handoffReady() {
		return
	}
	p := a.pal
	h := &a.pb
	ui.DialogBase(c.Key("takeover"), &h.open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.35))
		panel.Width(380).Padding(24).Radius(18).Gap(8).
			Background(p.bg).Border(1, p.hairline).Shadow(0, 24, 60, -12, ui.RGBA(0, 0, 0, 0.4)).
			Transition(ui.ElementTransition{Animation: &ui.Snappy, Enter: &ui.Motion{Scale: 0.94}, Exit: &ui.Motion{Scale: 0.96}})
		st := h.state
		ui.Text(c, T("Continue Playing on This Device")).FontSize(17).FontWeight(700)
		if st.Track != nil {
			name := st.Track.Name
			if st.Track.Artist != "" {
				name = Tf("%s — %s", st.Track.Name, st.Track.Artist)
			}
			ui.Text(c, name).FontSize(13.5).FontWeight(600).SingleLine()
		}
		if from := deviceLabel(st.PendingFromName); from != "" {
			ui.Text(c, Tf("Playing on %s", from)).FontSize(13).TextColor(p.muted).SingleLine()
		}
		ui.Row(c).Gap(10).Justify(ui.End).Margin(10, 0, 0, 0).Children(func() {
			if pillButton(c.Key("cancel"), nil, T("Cancel"), p.hover.Over(p.bg), p.text).Clicked() {
				a.cancelHandoff()
			}
			if pillButton(c.Key("play-here"), nil, T("Play Here"), p.accent, p.onAccent).AutoFocus().Clicked() {
				a.continueHandoff()
			}
		})
	})
	// The dialog closed without a choice, as under the Escape key: the
	// other device is told not to wait.
	if !h.open && h.pending {
		a.cancelHandoff()
	}
}

// deviceIcon is the icon of a device, by the kind the server knows it as.
func deviceIcon(kind string) *ui.SVG {
	switch kind {
	case "phone", "tablet":
		return iconPhone
	case "computer":
		return iconLaptop
	}
	return iconDevices
}
