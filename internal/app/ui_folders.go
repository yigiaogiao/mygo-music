package app

import (
	"time"

	"github.com/egoist/mygo/ui"

	"mygo-music/internal/websource"
)

// folderPicker is the app's window on the netdisk: where it is, and what the
// server said of it. It takes the place of the server's own page, which a
// browser cannot open over the player.
type folderPicker struct {
	open   bool
	source *websource.Source
	gen    int // the ask last made; an older answer does nothing
	page   *websource.FolderPage
	busy   bool
	err    string
}

// openFolderPicker shows the netdisk's folders, opening at the top.
func (a *app) openFolderPicker(w *websource.Source) {
	a.folders = folderPicker{open: true, source: w}
	a.browseFolders("")
}

// browseFolders asks the server what the netdisk has at path.
func (a *app) browseFolders(path string) {
	f := &a.folders
	if f.source == nil {
		return
	}
	f.gen++
	gen := f.gen
	f.busy, f.err = true, ""
	w := f.source
	go func() {
		page, err := w.BrowseFolders(path)
		a.post(func() {
			if f.gen != gen || !f.open {
				return
			}
			f.busy = false
			if err != nil {
				f.err = err.Error()
				return
			}
			f.page = page
		})
	}()
}

// pickFolder adds a folder for the library to read, or stops reading one,
// and shows the folder again as it is then.
func (a *app) pickFolder(add bool, path string) {
	f := &a.folders
	if f.source == nil || f.busy {
		return
	}
	f.gen++
	gen := f.gen
	f.busy, f.err = true, ""
	w := f.source
	at := f.page.Path
	go func() {
		var err error
		if add {
			err = w.AddFolder(path)
		} else {
			err = w.RemoveFolder(path)
		}
		a.post(func() {
			if f.gen != gen || !f.open {
				return
			}
			f.busy = false
			if err != nil {
				f.err = err.Error()
				return
			}
			a.loadFolders(a.library)
			a.browseFolders(at)
		})
	}()
}

// folderPickerDialog shows the netdisk's folders over the page, to choose
// which the library reads.
func (a *app) folderPickerDialog(c *ui.Context) {
	p := a.pal
	f := &a.folders
	ui.DialogBase(c.Key("folder-picker"), &f.open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.35))
		w, h := c.Size()
		panel.Width(min(540, w-28)).Height(min(620, h-48)).Padding(18, 18, 18, 18).Radius(18).Gap(12).
			Background(p.bg).Border(1, p.hairline).Shadow(0, 24, 60, -12, ui.RGBA(0, 0, 0, 0.4)).
			Transition(ui.ElementTransition{Animation: &ui.Snappy, Enter: &ui.Motion{Scale: 0.94}, Exit: &ui.Motion{Scale: 0.96}})
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Text(c, T("Choose Folders")).FontSize(17).FontWeight(700).SingleLine().Shrink(1).MinWidth(0)
			ui.Box(c).Grow(1)
		})
		if page := f.page; page != nil {
			a.folderCrumbs(c, p, page)
			a.folderDirs(c, p, page)
			a.folderFooter(c, p, page)
		} else {
			ui.Box(c).Grow(1).Center().Children(func() {
				ui.Text(c, T("Loading…")).FontSize(13).TextColor(p.muted)
			})
		}
		if f.err != "" {
			ui.Text(c, f.err).FontSize(12.5).TextColor(ui.Hex("#d9455f")).LineHeight(1.4)
		}
	})
	if !f.open {
		f.source = nil
	}
}

// folderCrumbs is the way back up to the netdisk's root: the folders gone
// through, each a click away.
func (a *app) folderCrumbs(c *ui.Context, p palette, page *websource.FolderPage) {
	ui.Row(c.Key("crumbs")).Gap(4).AlignItems(ui.Center).Height(30).Children(func() {
		if !page.AtRoot {
			if backButton(c.Key("up"), p.text, p.hover, T("Back")).Clicked() {
				a.browseFolders(page.Parent)
			}
			ui.Box(c).Width(2)
		}
		ui.Row(c.Key("path")).Gap(2).AlignItems(ui.Center).Grow(1).Shrink(1).MinWidth(0).ClipX().Children(func() {
			for i, crumb := range page.Crumbs {
				if i > 0 {
					ui.Text(c, "/").FontSize(12).TextColor(p.faint)
				}
				at := i == len(page.Crumbs)-1
				b := ui.ButtonBase(c.Key(crumb.Path)).Height(24).PaddingX(6).Radius(6).AlignItems(ui.Center).FocusRing(false).Label(crumb.Name)
				if !at && b.Hovered() {
					b.Background(p.hover)
				}
				color := p.muted
				if at {
					color = p.text
				}
				b.Children(func() {
					ui.Text(c, crumb.Name).FontSize(12.5).SingleLine().TextColor(color)
				})
				if b.Clicked() && !at {
					a.browseFolders(crumb.Path)
				}
			}
		})
	})
}

// folderDirs lists the folders under the one shown, each to go into: the one
// the listener makes their way to is the one the footer adds, as a folder
// chooser does.
func (a *app) folderDirs(c *ui.Context, p palette, page *websource.FolderPage) {
	ui.Scroll(c.Key("dirs")).Grow(1).MinHeight(0).Radius(12).Border(1, p.hairline).Children(func() {
		for _, d := range page.Dirs {
			ui.Row(c.Key(d.Path)).Padding(2, 6).Gap(6).AlignItems(ui.Center).Children(func() {
				b := ui.ButtonBase(c.Key("open")).Grow(1).Height(44).PaddingX(10).Gap(10).Radius(8).AlignItems(ui.Center).Justify(ui.Start).FocusRing(false).Label(d.Name)
				if b.Hovered() {
					b.Background(p.hover)
				}
				b.Children(func() {
					ui.Icon(c, iconFolder).Size(16, 16).TextColor(p.muted)
					ui.Text(c, d.Name).FontSize(13).SingleLine().Grow(1).Shrink(1).MinWidth(0)
					if d.Symlink {
						ui.Text(c, "↗").FontSize(11).TextColor(p.faint)
					}
					if d.Added {
						ui.Icon(c, iconCheck).Size(13, 13).TextColor(p.accent)
					}
					ui.Icon(c, iconForward).Size(13, 13).TextColor(p.faint)
				})
				if b.Clicked() {
					a.browseFolders(d.Path)
				}
			})
		}
		if len(page.Dirs) == 0 {
			ui.Box(c).Height(96).Center().Gap(6).Children(func() {
				ui.Text(c, T("This folder has no subfolders.")).FontSize(12.5).TextColor(p.muted)
				ui.Text(c, T("You can add this folder.")).FontSize(11.5).TextColor(p.faint)
			})
		}
	})
}

// folderFooter adds the folder the picker has opened on, as the Open button
// of a folder chooser chooses the folder it shows; Done closes it, and the
// listener may add more meanwhile. Folders chosen are taken away in the
// settings, where they are listed.
func (a *app) folderFooter(c *ui.Context, p palette, page *websource.FolderPage) {
	f := &a.folders
	ui.Row(c.Key("footer")).Gap(12).AlignItems(ui.Center).Children(func() {
		if ui.Button(c.Key("done"), T("Done")).Clicked() {
			f.open = false
		}
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(1).Children(func() {
			ui.Text(c, T("Folder to add")).FontSize(10.5).TextColor(p.faint)
			name := page.Path
			if page.AtRoot {
				name = T("The whole netdisk")
			}
			ui.Text(c, name).FontSize(11.5).TextColor(p.muted).SingleLine()
		})
		if page.Added {
			ui.Row(c.Key("added")).Gap(5).Height(32).PaddingX(6).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, iconCheck).Size(14, 14).TextColor(p.accent)
				ui.Text(c, T("Added")).FontSize(12.5).TextColor(p.muted)
			})
		} else if pillButton(c.Key("add"), iconPlus, T("Add This Folder"), p.accent, p.onAccent).Clicked() {
			a.pickFolder(true, page.Path)
		}
	})
}

// backButton is the button that goes back a level: an arrow and the word, on
// a soft round ground, for the pages, Now Playing and the settings.
func backButton(c *ui.Context, color, bg ui.Color, label string) ui.Element {
	b := ui.ButtonBase(c).Height(30).PaddingX(11).Gap(5).Radius(999).AlignItems(ui.Center).FocusRing(false).Label(label)
	b.Background(b.AnimateColor("bg", bg, ui.Animation{Duration: 120 * time.Millisecond}))
	b.Children(func() {
		ui.Icon(c, iconBack).Size(15, 15)
		ui.Text(c, label).FontSize(13).FontWeight(600).SingleLine()
	})
	return b
}
