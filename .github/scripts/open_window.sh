#!/usr/bin/env bash
# Opens the window of the program just built, waits, checks it is still open and keeps a
# screenshot of the screen as window-<name>.png. On Linux the screen is a virtual one (Xvfb).
#   open_window.sh <name>
set -euo pipefail

name="$1"
shot="window-${name}.png"
wait_seconds=15

case "$(uname -s)" in
	Linux)
		# WebKitGTK draws in software on a screen without a GPU
		export WEBKIT_DISABLE_DMABUF_RENDERER=1 LIBGL_ALWAYS_SOFTWARE=1
		dbus-run-session -- xvfb-run -a -s "-screen 0 1280x860x24" bash -c '
			./out/nxpress &
			pid=$!
			sleep '"${wait_seconds}"'
			if ! kill -0 "${pid}" 2>/dev/null; then
				echo "the window closed by itself" >&2
				exit 1
			fi
			import -window root "'"${shot}"'"
			kill "${pid}"
		'
		;;
	Darwin)
		open -n out/NXPress.app
		sleep "${wait_seconds}"
		if ! pgrep -x nxpress >/dev/null; then
			echo "the window closed by itself" >&2
			exit 1
		fi
		screencapture -x "${shot}"
		pkill -x nxpress
		;;
	*)
		./out/nxpress.exe &
		pid=$!
		sleep "${wait_seconds}"
		if ! tasklist //FI "IMAGENAME eq nxpress.exe" | grep -qi nxpress; then
			echo "the window closed by itself" >&2
			exit 1
		fi
		powershell -NoProfile -Command "Add-Type -AssemblyName System.Windows.Forms, System.Drawing; \
			\$screen = [System.Windows.Forms.Screen]::PrimaryScreen.Bounds; \
			\$image = New-Object System.Drawing.Bitmap \$screen.Width, \$screen.Height; \
			[System.Drawing.Graphics]::FromImage(\$image).CopyFromScreen(\$screen.Location, [System.Drawing.Point]::Empty, \$screen.Size); \
			\$image.Save('${shot}')"
		taskkill //IM nxpress.exe //F >/dev/null
		;;
esac
ls -l "${shot}"
