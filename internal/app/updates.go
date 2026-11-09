package app

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/updater"
	"github.com/egoist/mygo/plugins/updater/native"

	"mygo-music/internal/i18n"
)

// The update plugin's check and preferences, which tests swap: they don't
// use the plugin, whose functions panic without it.
var (
	checkForUpdates    = updater.CheckForUpdates
	updateChecks       = updater.AutomaticChecks
	setUpdateChecks    = updater.SetAutomaticChecks
	updateDownloads    = updater.AutomaticDownloads
	setUpdateDownloads = updater.SetAutomaticDownloads
	lastUpdateCheck    = updater.LastCheck
)

// useUpdater checks GitHub's releases for a new version soon after launch
// and then once a day, and offers it in an update window of native UI, in the
// listener's language as the app starts: the plugin takes it once. It must run
// before App.Run.
func (a *app) useUpdater(language string) {
	tag := "en"
	if language == i18n.Chinese {
		tag = "zh-Hans"
	}
	mygo.Use(native.New(updater.Options{Language: tag}))
	// The browser, and a build that can't write where it's installed, load
	// the newest version with the page or their package manager: they have
	// no page for updates to show.
	a.updates = mygo.Updater.Enabled()
	// A check or an answer in the update window changes what the settings
	// show.
	updater.OnChange(func() {
		if a.settingsWin != nil {
			a.settingsWin.Invalidate()
		}
	})
}
