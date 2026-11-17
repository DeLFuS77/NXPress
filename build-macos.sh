#!/usr/bin/env bash
# Builds NXPress on macOS (Apple silicon or Intel), the way the release build does.
#   ./build-macos.sh          build only
#   ./build-macos.sh check    also run go vet and go test first
# Leaves ./nxpress (the program, next to the "settings" folder: portable) and out/NXPress.app (the
# app, which opens from the Finder; it keeps its settings in ~/Library/Application Support/NXPress).
# Everything it prints is also saved in build-log.txt.
set -uo pipefail
cd "$(dirname "$0")"
LOG="$PWD/build-log.txt"
: > "$LOG"
VERSION="${NXPRESS_VERSION:-1.7.19-details}"

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

[ "$(uname -s)" = "Darwin" ] || fail "this is for macOS (use build-linux.sh on Linux, build-windows.bat on Windows)"
command -v go >/dev/null 2>&1 || fail "Go is not installed or not on the PATH (https://go.dev/dl: NXPress needs the version in go.mod)"
xcode-select -p >/dev/null 2>&1 && command -v clang >/dev/null 2>&1 || fail "the Xcode command line tools are missing: run  xcode-select --install  and try again"

export CGO_ENABLED=1
# the oldest macOS Go supports
export MACOSX_DEPLOYMENT_TARGET="13.0"
# the file dialogs of Wails use UTType, a framework Wails does not link itself
export CGO_LDFLAGS="-framework UniformTypeIdentifiers"
tags="desktop,production"

step "Go version" go version
if [ "${1:-}" = "check" ]; then
  step "go vet" go vet -tags "$tags" ./...
  step "go test" go test -tags "$tags" ./...
fi
step "Building nxpress" go build -tags "$tags" -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o nxpress .
step "Checking it starts" ./nxpress --version
step "Making NXPress.app" bash .github/scripts/macos_app.sh nxpress "${VERSION}" out

echo
echo "DONE"
echo "  App:      $PWD/out/NXPress.app"
echo "  Portable: $PWD/nxpress  (keeps its settings in $PWD/settings)"
echo
echo "The app is signed ad hoc, not notarized: the first time, right click it and choose Open."
echo "If macOS says it is damaged, run:  xattr -dr com.apple.quarantine out/NXPress.app"
