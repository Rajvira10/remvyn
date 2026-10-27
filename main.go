package main

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"log"
	"os"

	"runtime"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/menu"
	"github.com/wailsapp/wails/v2/pkg/menu/keys"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
	"github.com/wailsapp/wails/v2/pkg/options/mac"

	"remvyn/backend/updater"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var icon []byte

func main() {
	// `remvyn mcp` is the AI agent bridge, launched by MCP clients; it
	// never opens a window.
	if len(os.Args) > 1 && os.Args[1] == "mcp" {
		os.Exit(runMCP())
	}
	// Remove what the last update left behind (old binary, download).
	go updater.Cleanup()
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
		// On macOS the close button hides Remvyn so it keeps serving AI
		// agents and forwards; clicking the Dock icon brings it back and ⌘Q
		// quits. Elsewhere there is no Dock to come back from, so it quits.
		HideWindowOnClose: runtime.GOOS == "darwin",
		// Opening Remvyn again shows the running window rather than a second
		// copy fighting over the database and the agent socket.
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId:               instanceID(),
			OnSecondInstanceLaunch: func(options.SecondInstanceData) { app.showWindow() },
		},
		Bind: []interface{}{app},
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

// instanceID is the single-instance key. A custom REMVYN_DATA_DIR is a
// separate vault, so it gets its own instance.
func instanceID() string {
	id := "io.github.rajvira10.remvyn"
	if dir := os.Getenv("REMVYN_DATA_DIR"); dir != "" {
		sum := sha256.Sum256([]byte(dir))
		id += "." + hex.EncodeToString(sum[:6])
	}
	return id
}
