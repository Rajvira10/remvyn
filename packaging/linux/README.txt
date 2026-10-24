Remvyn for Linux (x86_64)

1. Install the runtime libraries:
     Arch / Omarchy:  sudo pacman -S --needed gtk3 webkit2gtk-4.1
     Debian / Ubuntu: sudo apt install libgtk-3-0 libwebkit2gtk-4.1-0
     Fedora:          sudo dnf install gtk3 webkit2gtk4.1
2. Run ./install.sh  (installs to ~/.local/bin plus a launcher entry)

Or just run ./remvyn from this folder.

Blank or flickering window on NVIDIA? Start it with
  WEBKIT_DISABLE_DMABUF_RENDERER=1 remvyn
Data lives in ~/.config/Remvyn.
