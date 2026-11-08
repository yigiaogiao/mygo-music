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
			if iconButton(c.Key("close"), iconX, 16, p.muted, T("Close")).Clicked() {
				f.open = false
			}
		})
		if page := f.page; page != nil {
			a.folderCrumbs(c, p, page)
			a.folderHere(c, p, page)
			if len(page.Folders) > 0 {
				a.folderChosen(c, p, page)
			}
			a.folderDirs(c, p, page)
		} else {
			ui.Box(c).Grow(1).Center().Children(func() {
				ui.Text(c, T("Loading…")).FontSize(13).TextColor(p.muted)
			})
		}
		if f.err != "" {
			ui.Text(c, f.err).FontSize(12.5).TextColor(ui.Hex("#d9455f")).LineHeight(1.4)
		}
		ui.Row(c).Gap(10).Justify(ui.End).Children(func() {
			if pillButton(c.Key("done"), nil, T("Done"), p.accent, p.onAccent).Clicked() {
				f.open = false
			}
		})
	})
	if !f.open {
		f.source = nil
	}
}

// folderCrumbs is the way back up to the netdisk's root.
func (a *app) folderCrumbs(c *ui.Context, p palette, page *websource.FolderPage) {
	ui.Row(c.Key("crumbs")).Gap(2).AlignItems(ui.Center).ClipX().Children(func() {
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
}

// folderHere is the bar over the folders under the one shown: up, and adding
// the folder itself.
func (a *app) folderHere(c *ui.Context, p palette, page *websource.FolderPage) {
	ui.Row(c.Key("here")).Gap(8).AlignItems(ui.Center).Children(func() {
		if !page.AtRoot {
			if backButton(c.Key("up"), p.muted, p.hover, T("Back")).Clicked() {
				a.browseFolders(page.Parent)
			}
		}
		ui.Text(c, page.Path).FontSize(11.5).TextColor(p.faint).SingleLine().Grow(1).Shrink(1).MinWidth(0)
		if page.Added {
			ui.Text(c, T("Added")).FontSize(12).TextColor(p.muted).Margin(0, 4, 0, 0)
		} else if pillButton(c.Key("add-here"), iconPlus, T("Add This Folder"), p.accent, p.onAccent).Clicked() {
			a.pickFolder(true, page.Path)
		}
	})
}

// folderChosen lists the folders the library reads now.
func (a *app) folderChosen(c *ui.Context, p palette, page *websource.FolderPage) {
	ui.Text(c, T("Selected Folders")).FontSize(11.5).FontWeight(600).TextColor(p.muted)
	ui.Column(c.Key("chosen")).Radius(10).Background(p.hover).Children(func() {
		for _, f := range page.Folders {
			name := f.Name
			if f.Whole {
				name = T("The whole netdisk")
			}
			ui.Row(c.Key(f.Path)).Padding(6, 10).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, iconFolder).Size(14, 14).TextColor(p.muted)
				ui.Text(c, name).FontSize(12.5).SingleLine().Grow(1).Shrink(1).MinWidth(0)
				if iconButton(c.Key("remove"), iconX, 13, p.muted, T("Remove")).Clicked() {
					a.pickFolder(false, f.Path)
				}
			})
		}
	})
}

// folderDirs lists the folders under the one shown, to go into or to add.
func (a *app) folderDirs(c *ui.Context, p palette, page *websource.FolderPage) {
	ui.Scroll(c.Key("dirs")).Grow(1).MinHeight(0).Radius(10).Border(1, p.hairline).Children(func() {
		for _, d := range page.Dirs {
			ui.Row(c.Key(d.Path)).Padding(2, 6).Gap(6).AlignItems(ui.Center).Children(func() {
				b := ui.ButtonBase(c.Key("open")).Grow(1).Height(42).PaddingX(8).Gap(10).Radius(8).AlignItems(ui.Center).Justify(ui.Start).FocusRing(false).Label(d.Name)
				if b.Hovered() {
					b.Background(p.hover)
				}
				b.Children(func() {
					ui.Icon(c, iconFolder).Size(16, 16).TextColor(p.muted)
					ui.Text(c, d.Name).FontSize(13).SingleLine().Grow(1).Shrink(1).MinWidth(0)
					if d.Symlink {
						ui.Text(c, "↗").FontSize(11).TextColor(p.faint)
					}
				})
				if b.Clicked() {
					a.browseFolders(d.Path)
				}
				if d.Added {
					ui.Text(c, T("Added")).FontSize(12).TextColor(p.muted).Margin(0, 8, 0, 8)
				} else if ui.Button(c.Key("add"), T("Add")).Clicked() {
					a.pickFolder(true, d.Path)
				}
			})
		}
		if len(page.Dirs) == 0 {
			ui.Box(c).Height(80).Center().Children(func() {
				ui.Text(c, T("This folder has no subfolders.")).FontSize(12.5).TextColor(p.muted)
			})
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
