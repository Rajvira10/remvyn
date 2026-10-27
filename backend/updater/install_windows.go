//go:build windows

package updater

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"unicode/utf16"
)

// A copy installed with the NSIS installer is updated by running the new
// installer silently into the same folder, so the uninstall entry stays
// right. A portable copy (from the .zip) swaps Remvyn.exe in place: Windows
// lets a running .exe be renamed, not overwritten.

const createNoWindow = 0x08000000

func plan() (string, error) {
	if runtime.GOARCH != "amd64" {
		return "", errors.New("updates are only published for 64-bit x86 Windows")
	}
	exe, err := executable()
	if err != nil {
		return "", err
	}
	if installedWithSetup(exe) {
		return "-windows-x64-setup.exe", nil
	}
	if dir := filepath.Dir(exe); !writable(dir) {
		return "-windows-x64.zip", fmt.Errorf("Remvyn can't write to %s; download the new version from the release page", dir)
	}
	return "-windows-x64.zip", nil
}

func installedWithSetup(exe string) bool {
	_, err := os.Stat(filepath.Join(filepath.Dir(exe), "uninstall.exe"))
	return err == nil
}

func install(file string) error {
	exe, err := executable()
	if err != nil {
		return err
	}
	wait := fmt.Sprintf("Wait-Process -Id %d -ErrorAction SilentlyContinue\n", os.Getpid())
	if strings.HasSuffix(file, "-setup.exe") {
		// /D must come last and unquoted. If the installer fails or UAC is
		// declined, the old version starts again.
		args := "/S /D=" + filepath.Dir(exe)
		return powershell(wait +
			"try { Start-Process -FilePath " + psQuote(file) + " -ArgumentList " + psQuote(args) + " -Wait } catch {}\n" +
			"Start-Process -FilePath " + psQuote(exe) + "\n" +
			"Remove-Item -LiteralPath " + psQuote(file) + " -ErrorAction SilentlyContinue")
	}

	staged, old := exe+".new", exe+".old"
	if err := extractZip(file, "Remvyn.exe", staged, 0o755); err != nil {
		return err
	}
	_ = os.Remove(file)
	_ = os.Remove(old)
	if err := os.Rename(exe, old); err != nil {
		os.Remove(staged)
		return err
	}
	if err := os.Rename(staged, exe); err != nil {
		_ = os.Rename(old, exe)
		os.Remove(staged)
		return err
	}
	return powershell(wait + "Start-Process -FilePath " + psQuote(exe))
}

func cleanupPlatform() {
	if exe, err := executable(); err == nil {
		_ = os.Remove(exe + ".old")
		_ = os.Remove(exe + ".new")
	}
}

// powershell runs script in a hidden, detached PowerShell that outlives us.
func powershell(script string) error {
	u := utf16.Encode([]rune(script))
	b := make([]byte, 2*len(u))
	for i, r := range u {
		b[2*i], b[2*i+1] = byte(r), byte(r>>8)
	}
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass",
		"-EncodedCommand", base64.StdEncoding.EncodeToString(b))
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

func psQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }
