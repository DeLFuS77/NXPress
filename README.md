# NXPress

Compresses and decompresses Nintendo Switch NSP and XCI files to NSZ and XCZ, multi-title files (SuperNSP) included. It writes standard NSZ and XCZ, the same format nsz writes, so installers read them as usual.

It is one program with no Python or other dependencies. Opened with no arguments it shows a window: drop files or folders on it, choose what to do and start. With arguments it is a command line that takes the same options as nsz, so it can replace it in scripts and in tools that call nsz.

Work in progress: there are no downloads yet.

## Usage

```
nxpress -C game.nsp                 compress to game.nsz
nxpress -C -l 19 -t 8 -o out games  compress every NSP and XCI of a folder at level 19 into out
nxpress -D game.nsz                 decompress to game.nsp (an XCZ to an XCI)
nxpress -V game.nsz                 check a file
nxpress --split super.nsp           split a multi-title file into one file per title
nxpress --pack super.nsp a.nsp b.nsp
nxpress --convert game.xci          turn an XCI into an NSP
nxpress --recompress -B game.nsz    write an NSZ again in blocks (or -S, solid), no NSP in between
nxpress --trim game.xci update.nsp  take away the padding of an XCI and the delta fragments of an update
nxpress --estimate -l 19 game.nsp   tell the size and the time a compression would give
nxpress -i game.nsz                 show its titles, how it is stored and what it works with
```

In the window, files and folders can be dropped on it, or on NXPress itself; on Windows it can add itself to "Send to" in the Explorer, so the files chosen there open in it. It follows the light or dark theme of the system, or the one chosen in its menu.

Solid or in blocks: a solid NSZ (the default) is the smallest and every installer takes it (DBI, Tinfoil, Awoo, CyberFoil, Sphaira). With `-B` it is written in blocks, which emulators that read NSZ can run as it is, without installing it; every installer but Awoo takes it too. XCZ files are always in blocks, as nsz writes them.

Every new file is written under a hidden name, checked, and only then given its name. With `--rm-source` the original is deleted only once its new file passed the check.

The keys are read from `--keys`, or from a `prod.keys` or `keys.txt` next to NXPress, in the working folder or in `~/.switch`.

## Building

NXPress needs Go and a C compiler (for zstd). The window uses [Wails](https://wails.io): WebView2 on Windows, WebKitGTK on Linux.

```
go build -tags desktop,production -ldflags "-s -w" .
```

## License

AGPL-3.0
