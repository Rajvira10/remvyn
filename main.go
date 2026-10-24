package main

import (
	"embed"
	"log"

	"runtime"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/menu"
	"github.com/wailsapp/wails/v2/pkg/menu/keys"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var icon []byte

func main() {
	app := NewApp()

	// macOS routes ⌘C/⌘V/⌘A in text fields through the Edit menu.
	var appMenu *menu.Menu
	if runtime.GOOS == "darwin" {
		appMenu = menu.NewMenu()
		appMenu.Append(menu.AppMenu())
		appMenu.Append(menu.EditMenu())
		win := appMenu.AddSubmenu("Window")
		win.AddText("Minimise", keys.CmdOrCtrl("m"), func(_ *menu.CallbackData) {
			if app.ctx != nil {
				wruntimeMinimise(app)
			}
		})
	}

	err := wails.Run(&options.App{
		Title:     "Remvyn",
		Width:     1360,
		Height:    860,
		MinWidth:  900,
		MinHeight: 560,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 21, G: 22, B: 26, A: 1},
		Menu:             appMenu,
		OnStartup:        app.startup,
		OnDomReady:       app.domReady,
		OnShutdown:       app.shutdown,
		ErrorFormatter:   formatError,
		Bind:             []interface{}{app},
		Linux: &linux.Options{
			Icon: icon,
			// Matches StartupWMClass in remvyn.desktop so launchers and
			// compositors (Hyprland, GNOME) show the right icon.
			ProgramName:      "remvyn",
			WebviewGpuPolicy: linux.WebviewGpuPolicyOnDemand,
		},
		Mac: &mac.Options{
			TitleBar: mac.TitleBarHiddenInset(),
			About: &mac.AboutInfo{
				Title:   "Remvyn",
				Message: "SSH client · v" + Version,
			},
		},
	})
	if err != nil {
		log.Fatal(err)
	}
}
