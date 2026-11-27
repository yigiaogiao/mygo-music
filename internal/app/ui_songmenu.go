package app

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"

	"mygo-music/internal/music"
)

// copyText puts text on the clipboard; tests replace it.
var copyText = func(text string) error { return mygo.Clipboard.Write(transfer.TextData(text)) }

// songMenu is three dots that open the menu of tr.
func (a *app) songMenu(c *ui.Context, tr *music.Track, size float32, color ui.Color) ui.Element {
	b := iconButton(c, iconMore, size, color, T("More"))
	// A phone has no second button to open a menu of its own, and the menu
	// the system shows over the page is made for a pointer, its rows too
	// small for a finger and covering what's under it: the dots stay off.
	if a.mobile(c) {
		return b.Hidden(true)
	}
	b.Menu(a.songItems(tr, ""))
	return b
}

// songItems builds the menu of what can be done with tr, behind its dots
// or a right click on its row: copying its title, its title with its
// artists, or the link to its page, downloading it, opening its artists',
// album's or show's, and adding it to a playlist or taking it out of pid, the
// playlist it is listed in, if set; as far as its source can.
func (a *app) songItems(tr *music.Track, pid string) func(m *ui.Menu) {
	return func(m *ui.Menu) {
		lib := a.trackLib(tr)
		if m.Item(T("Copy Title")).Chosen() {
			copyText(tr.Name)
		}
		if names := tr.ArtistNames(); names != "" && m.Item(T("Copy Title and Artists")).Chosen() {
			copyText(tr.Name + " - " + names)
		}
		if tr.URL != "" {
			m.Separator()
			if m.Item(T("Copy Link")).Chosen() {
				copyText(tr.URL)
			}
		}
		if a.canDownload(tr) {
			m.Separator()
			if m.Item(T("Download…")).Chosen() {
				a.download(*tr)
			}
		}
		// Its artists', album's and show's pages, those the source has.
		linked := tr.Album.ID != "" && lib.can("album") || tr.ShowID != "" && lib.can("show")
		for _, ar := range tr.Artists {
			linked = linked || ar.ID != "" && lib.can("artist")
		}
		if linked {
			m.Separator()
		}
		if tr.ShowID != "" && lib.can("show") && m.Item(T("Go to Podcast")).Chosen() {
			a.openPage(route{page: pageShow, src: lib.id, id: tr.ShowID})
		}
		for _, ar := range tr.Artists {
			if ar.ID != "" && lib.can("artist") && m.Item(Tf("Go to Artist: %s", ar.Name)).Chosen() {
				a.openArtist(lib.id, ar.ID)
			}
		}
		if tr.Album.ID != "" && lib.can("album") && m.Item(Tf("Go to Album: %s", tr.Album.Name)).Chosen() {
			a.openPage(route{page: pageAlbum, src: lib.id, id: tr.Album.ID})
		}
		a.playlistItems(m, tr, pid)
	}
}
