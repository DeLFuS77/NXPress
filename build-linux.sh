#!/usr/bin/env bash
# Builds NXPress on Linux (x64 or arm64), the way the release build does.
#   ./build-linux.sh          build only
#   ./build-linux.sh check    also run go vet and go test first
# Leaves ./nxpress next to the "settings" folder (portable: it keeps everything there), and
# dist/NXPress/ with the program, its icon, its menu entry and install.sh (sh install.sh).
# Everything it prints is also saved in build-log.txt.
set -uo pipefail
cd "$(dirname "$0")"
LOG="$PWD/build-log.txt"
: > "$LOG"
VERSION="${NXPRESS_VERSION:-1.7.16-details}"

fail() {
  echo; echo "FAILED: $1"; echo "The full output is in $LOG"; exit 1
}
step() { # step "what" command...
  local what="$1"; shift
  echo; echo "=== $what"; echo "=== $what" >> "$LOG"
  if "$@" >> "$LOG" 2>&1; then echo "ok"; else
    echo "--- last lines of the output:"; tail -n 30 "$LOG"; fail "$what"
  fi
}

command -v go >/dev/null 2>&1 || fail "Go is not installed or not on the PATH (https://go.dev/dl: NXPress needs the version in go.mod)"
command -v gcc >/dev/null 2>&1 || command -v cc >/dev/null 2>&1 || fail "no C compiler: install build-essential (Debian, Ubuntu), gcc (Fedora) or base-devel (Arch)"
command -v pkg-config >/dev/null 2>&1 || fail "pkg-config is not installed"

# the window: GTK 3 and WebKitGTK. 4.1 is what current distributions ship (and what the release uses)
tags="desktop,production"
if pkg-config --exists webkit2gtk-4.1; then
  tags="${tags},webkit2_41"
elif pkg-config --exists webkit2gtk-4.0; then
  echo "Using webkit2gtk-4.0 (older)"
else
  cat <<'MSG'
WebKitGTK development files not found. Install them, then run this again:
  Debian / Ubuntu:  sudo apt install build-essential pkg-config libgtk-3-dev libwebkit2gtk-4.1-dev
  Fedora:           sudo dnf install gcc pkgconf-pkg-config gtk3-devel webkit2gtk4.1-devel
  Arch:             sudo pacman -S base-devel gtk3 webkit2gtk-4.1
MSG
  exit 1
fi
pkg-config --exists gtk+-3.0 || fail "GTK 3 development files not found (libgtk-3-dev / gtk3-devel / gtk3)"

export CGO_ENABLED=1
step "Go version" go version
if [ "${1:-}" = "check" ]; then
  step "go vet" go vet -tags "$tags" ./...
  step "go test" go test -tags "$tags" ./...
fi
step "Building nxpress" go build -tags "$tags" -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o nxpress .
step "Checking it starts" ./nxpress --version

# the same folder the release downloads hold: program, icon, menu entry and install.sh
rm -rf dist/NXPress && mkdir -p dist/NXPress
cp nxpress build/linux/nxpress.desktop build/linux/nxpress.png build/linux/install.sh dist/NXPress/
chmod 755 dist/NXPress/nxpress dist/NXPress/install.sh

echo
echo "DONE"
echo "  Portable: $PWD/nxpress  (keeps its settings in $PWD/settings)"
echo "  Install:  cd dist/NXPress && sh install.sh   (menu entry and ~/.local/bin; --uninstall removes them)"
