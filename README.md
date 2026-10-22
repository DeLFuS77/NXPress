# NXPress

Compresses and decompresses Nintendo Switch NSP and XCI files to NSZ and XCZ, multi-title files (SuperNSP) included. It writes standard NSZ and XCZ, the same format nsz writes, so installers read them as usual.

It is one program with no Python or other dependencies. It takes the same options as nsz, so it can replace it in scripts and in tools that call nsz.

Work in progress: there are no downloads yet.

## Usage

```
nxpress -C game.nsp                 compress to game.nsz
nxpress -C -l 19 -t 8 -o out games  compress every NSP and XCI of a folder at level 19 into out
nxpress -D game.nsz                 decompress to game.nsp
nxpress -V game.nsz                 check a file
nxpress --split super.nsp           split a multi-title file into one file per title
nxpress --pack super.nsp a.nsp b.nsp
nxpress --convert game.xci          turn an XCI into an NSP
```

Every new file is written under a hidden name, checked, and only then given its name. With `--rm-source` the original is deleted only once its new file passed the check.

The keys are read from `--keys`, or from a `prod.keys` or `keys.txt` next to NXPress, in the working folder or in `~/.switch`.

## License

AGPL-3.0
