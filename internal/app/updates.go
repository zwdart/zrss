package app

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/updater"
	"github.com/egoist/mygo/plugins/updater/native"
)

// The update plugin's check and preferences, which tests swap: the plugin's
// functions panic when the plugin is not used, and the settings view must
// stay usable without it.
var (
	checkForUpdates    = updater.CheckForUpdates
	updateChecks       = updater.AutomaticChecks
	setUpdateChecks    = updater.SetAutomaticChecks
	updateDownloads    = updater.AutomaticDownloads
	setUpdateDownloads = updater.SetAutomaticDownloads
	lastUpdateCheck    = updater.LastCheck
)

// releasesURL is where a new version can be downloaded when the app cannot
// replace itself, as a .deb installed in /opt cannot.
const releasesURL = "https://github.com/zwdart/zrss/releases/latest"

// useUpdater checks GitHub's releases for a new version soon after launch
// and then once a day, and offers it in a window of native UI, in Chinese.
// It must run before App.Run.
func (a *app) useUpdater() {
	mygo.Use(native.New(updater.Options{Language: "zh-Hans"}))
	a.updates = true
	// Where a package manager installed the app, it owns the files: the
	// update is the package manager's to install, so the app links to the
	// release instead of offering a download that cannot replace it.
	a.canSelfUpdate = mygo.Updater.Enabled()
	// A check or an answer in the update window changes what the settings
	// show.
	updater.OnChange(func() {
		if a.settingsWin != nil {
			a.settingsWin.Invalidate()
		}
	})
}

// checkUpdates asks the update window for a new version, or opens the
// releases page when the app cannot replace itself: a package manager
// installed it, so the new version comes from the package manager.
func (a *app) checkUpdates() {
	if a.canSelfUpdate {
		checkForUpdates()
		return
	}
	a.openReleases()
}

// openReleases opens the page of the newest release in the browser.
func (a *app) openReleases() {
	if err := mygo.Shell.OpenExternal(releasesURL); err != nil {
		mygo.Dialog.Error("无法打开发布页", "请手动访问：\n"+releasesURL)
	}
}
