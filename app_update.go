package main

import (
	"time"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"remvyn/backend/updater"
)

// How long after launch, and how often after that, Remvyn looks for a new
// release when Settings → Updates → Check automatically is on.
const (
	updateFirstCheck = 10 * time.Second
	updateInterval   = 6 * time.Hour
)

func (a *App) autoCheckUpdates() {
	t := time.NewTimer(updateFirstCheck)
	defer t.Stop()
	for {
		select {
		case <-a.ctx.Done():
			return
		case <-t.C:
		}
		if a.GetSettings().AutoUpdateCheck {
			switch a.upd.Status().State {
			case updater.StateDownloading, updater.StateInstalling, updater.StateRestarting:
			default:
				_, _ = a.upd.Check(a.ctx)
			}
		}
		t.Reset(updateInterval)
	}
}

// GetUpdateStatus returns the last known update state.
func (a *App) GetUpdateStatus() updater.Status { return a.upd.Status() }

// CheckForUpdate asks GitHub for the latest release now.
func (a *App) CheckForUpdate() (updater.Status, error) { return a.upd.Check(a.ctx) }

// InstallUpdate downloads and installs the available release, then quits so
// the relaunch helper can start the new version. Progress arrives as
// "update:status" events.
func (a *App) InstallUpdate() error {
	if err := a.upd.Install(a.ctx); err != nil {
		return err
	}
	go func() {
		// Let the UI show "Restarting…" before the window goes away.
		time.Sleep(600 * time.Millisecond)
		wruntime.Quit(a.ctx)
	}()
	return nil
}

// OpenReleasePage opens the latest release (or the releases list) in the
// browser, for copies that can't update themselves.
func (a *App) OpenReleasePage() {
	url := "https://github.com/" + updater.DefaultRepo + "/releases/latest"
	if s := a.upd.Status(); s.Latest != nil && s.Latest.URL != "" {
		url = s.Latest.URL
	}
	wruntime.BrowserOpenURL(a.ctx, url)
}
