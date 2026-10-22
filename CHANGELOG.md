# Changelog

## Unreleased

### New

- FAT32: files bigger than a microSD card in FAT32 holds (4 GB) become a folder of parts with the archive bit, which the console reads as one file; files in parts (such a folder, or name.ns0, name.ns1...) become one file again. Each copy is checked against the original (`--fat32`)
- Rename files by what they hold, from a template such as "{name} [{id}][v{version}]", with the new names shown before (`--rename`)
- The list shows the name and the icon of each game, its title ID and its version
- Pause and go on; the files left unfinished when NXPress was closed are offered when it opens again
- While files are at work the computer does not go to sleep; a notice of the system tells when long work is done, and it can turn the computer off when done, with 60 seconds to keep it on
- Watch folders: new files copied into them are worked on by themselves (`--watch`, also on the command line, for a NAS)
- Saved settings, with three of their own: for installers, for emulators, quick
- Compare the levels on this computer: the size and the time of each, from samples (`--estimate --levels`)
- Files already checked, or written by NXPress, are not read again while they do not change (`--no-cache` to check them all)
- Copy report: the version, the system, the settings and the result of each file, to paste in an issue or a thread
- A notice in the window when a new version is out, from GitHub; it can be turned off in the menu, and it is the only time NXPress goes on the internet
- The window in 11 languages: English, Spanish, French, German, Italian, Portuguese, Dutch, Russian, Japanese, Korean and Chinese

## 1.0.0

The first version of NXPress: a compressor for Nintendo Switch games in one program, with a window and a command line compatible with nsz.

### New

- Compress NSP and XCI to NSZ and XCZ, solid (the smallest, taken by every installer) or in blocks (run as they are by emulators that read NSZ), at levels 1 to 19, with zstd in C on every thread. Multi-title files (SuperNSP) compress like any other, with each of their tickets
- Decompress NSZ to NSP and XCZ to XCI, the same files nsz writes
- Recompress NSZ and XCZ, solid or in blocks, at another level, with no NSP in between and no keys needed
- Trim: the padding after the data of XCI files, and the delta fragments of updates
- Pack a game, its updates and its DLC into one SuperNSP, or into its XCI; of a title given twice, the newest is kept. Split them again into the files they were made of, byte for byte
- Convert XCI and XCZ to NSP and NSZ
- Verify files against the hashes of their content, and against a DAT of No-Intro and others
- Details of a file: its titles, how it is stored, and the installers and emulators it works with
- Estimate the size and the time of a compression before it starts
- Every new file is written under a hidden name, checked, and only then given its name; with Delete originals, an original is deleted only once its new file passed the check
- A window in English and Spanish, light or dark: drop files or folders on it or on NXPress itself, or send them from "Send to" in the Explorer
- The options of nsz work the same on the command line, so NXPress can take its place in scripts and in the tools that call nsz
- Tested with DBI
