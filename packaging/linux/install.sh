#!/usr/bin/env bash
# Installs Remvyn for the current user (no root needed).
# Needs GTK 3 and WebKitGTK 4.1:
#   Arch / Omarchy:  sudo pacman -S --needed gtk3 webkit2gtk-4.1
#   Debian / Ubuntu: sudo apt install libgtk-3-0 libwebkit2gtk-4.1-0
#   Fedora:          sudo dnf install gtk3 webkit2gtk4.1
set -euo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
bin="${XDG_BIN_HOME:-$HOME/.local/bin}"
apps="${XDG_DATA_HOME:-$HOME/.local/share}/applications"
icons="${XDG_DATA_HOME:-$HOME/.local/share}/icons/hicolor/1024x1024/apps"
mkdir -p "$bin" "$apps" "$icons"
install -m755 "$here/remvyn" "$bin/remvyn"
install -m644 "$here/remvyn.png" "$icons/remvyn.png"
sed "s|^Exec=remvyn|Exec=$bin/remvyn|" "$here/remvyn.desktop" > "$apps/remvyn.desktop"
command -v update-desktop-database >/dev/null && update-desktop-database "$apps" || true
command -v gtk-update-icon-cache >/dev/null && gtk-update-icon-cache -q "${XDG_DATA_HOME:-$HOME/.local/share}/icons/hicolor" || true
if ! ldconfig -p 2>/dev/null | grep -q libwebkit2gtk-4.1; then
  echo "Note: WebKitGTK 4.1 wasn't found. Install it first (see the top of this script)."
fi
echo "Installed. Launch Remvyn from your app launcher, or run: $bin/remvyn"
