# Changelog

## Unreleased

### New

- Pack a SuperNSP per game: give Pack a whole collection, and each game goes with its updates and DLC into a file of its own, found by the title IDs inside the files, not by their names, and named by a template, by default "{name} [{id}][v{version}] ({contents})": Game [0100...][v65536] (1G+1U+2D).nsp. A game with one file only is SKIPPED. One file with everything stays as a choice; on the command line, `--pack-games` and `--pack-name` (thanks @NikoMuffin, #3)
- New fields for Rename and for the packs: {version_txt}, the version as the game shows it (1.0.5), read from the control data of its update or of the game (#18); {contents}, the titles of each kind a file holds (1G+1U+2D), and {size_gb} and {size_mb} (#19). The fields as Switch Library Manager writes them work too ({TITLE_NAME}, {TITLE_ID}, {VERSION}, {VERSION_TXT}, {TYPE}, {PACKAGE_CONTENTS}, {SIZE_GB}, {SIZE_MB}), and brackets left empty are taken away. For a game with its updates, {version} is the version of the newest update (thanks @NikoMuffin, #18, #19)
- Packs by game and names with these fields were already in Switch Library Manager Web, which this program comes from: they are brought here
- The list of files sorts by name, size or state with a click on the column; a second click turns the order round and a third goes back to the order the files came in. The work goes in the order shown, and the order is kept for the next time (thanks @NikoMuffin, #17)
- Rename has a field {dlc_number}, the number of a DLC among the ones of its game (1, 2...), for short names like Game [DLC 1] [ID] instead of the long name of the DLC, and "Keep the names that already have their title ID" (--keep-named) leaves the files you named yourself. Both come from Switch Library Manager Web, which has them in Organize
- An icon beside each section of the menu (thanks @NikoMuffin, #30), and Settings as a page of its own at the bottom of the menu, by group: prod.keys, the look, what happens when the work is done and the app. prod.keys shows at the bottom left, and the menu of the top bar is gone (thanks @NikoMuffin, #27)
- Pack, a SuperNSP per game, shows the games before they are packed, each with a box to leave it out, and the number of its files. A game already packed where its pack would go (a file with its title ID that holds more than one title) is SKIPPED and told, unless Replace files already there is on (-w on the command line); a game of one file is shown as having nothing to pack it with
- Verify in levels: Whole (every NCA by its hashes, as before), Signature (the header of every NCA checked for the signature of Nintendo, in seconds: an NCA a tool rewrote is told by name) or Both, with a sentence for each; on the command line, --verify-level full, signature or both (thanks @NikoMuffin, #9). Signature needs Switch Library Manager Web with the same check

### Improvements

- The title of the window shows the version of NXPress, as the corner of the window already did

- Measure this PC takes a heavier setting when it is at least 5% faster, not 10%: in a measure where 7 threads with 2 files at once went 60 MB/s and 14 threads with one 55, the first is chosen now. The speed is told as what it is, all the files at once together, and when a setting a little faster is not taken, the window says why: the lighter one uses less memory and finishes each file sooner (thanks @NikoMuffin, #22)
- The level buttons show the levels each one covers, Light 1–6, Medium 7–14 and Maximum 15–19, and the hint tells that 18, the level of the presets and the default of nsz, is barely bigger than 19 and faster (thanks @NikoMuffin, #28)
- Trim shows that it takes the NSP and NSZ files of updates, the only ones with delta fragments: "NSP/NSZ (updates)" (thanks @NikoMuffin, #23)
- A game in sparse storage is SKIPPED with a plain sentence: no compressor can read that layout yet, nsz included, and the game installs as it is (thanks @NikoMuffin, #29)
- Info tells more of each title: the firmware the console needs for a game or an update, the version of its game a DLC needs, and the NCA files it is made of, by kind (thanks @NikoMuffin, #20)
- A new look: blue for what you choose and start, red only for the name of the app and for what went wrong; the icon of each section on a tile in the color of its group; the options of a section on one panel instead of boxes in a box; labels in sentence case; states with a dot of their color; sizes in figures of the same width; and, until the icon of a game is read, a cover with the kind of file and its name without the [ID][version] tags
- Motion that tells what changed: a section comes in and the tile chosen pops, files come into the list one after the other and fold away when taken out, a file that ends lights up in the color of its state, the bars of the work in progress move, the space saved counts up at the end and the bar of actions glows when nothing failed, and the drop zone answers a file dragged over it. With less motion asked in Windows, none of it plays
- More to see and less to fear: each section wears the color of its group beside its title; a file that ends shows the space it saved, its bar shrinking to the new size; a drop is answered over the whole window with how many items it brings; covers shimmer while a game is read; taking files out of the list can be undone (Ctrl+Z); an error opens to say what to do, with its detail to copy; a card at the end of the work tells what it saved and what saved the most, with the folder to open; a gauge shows what the level trades, size against speed; keyboard shortcuts (listed in Settings), and on a narrow window the menu keeps its tiles only
- The space NXPress has saved you since always, on the empty list, counting up, with a notice at 100 GB, 250 GB, 500 GB, 1 TB and on; the speed of the work as a line of the last minute, with the time left; the progress on the button of NXPress in the taskbar of Windows, filling up while it works and red when a file failed (thanks @NikoMuffin, #31); in Info, a card of each file with its size beside the one as NSP or XCI and what it works with; and the list as a grid of covers, with a switch above it
- A card over the cover of a game read in the list, after a moment: its cover in big, its name and publisher, title ID, kind (and Demo), version, the titles of each kind it holds, the game of an update or a DLC and the version of it a DLC needs, the firmware it needs and its languages; beside the cover in the grid. Escape or a scroll takes it away

### Fixes

- The window cannot be made smaller than 960 x 600, where everything fits (it went down to 900 x 520, where the bar of actions broke its words apart); on a smaller window the menu, the panel of options and the columns narrow, the bar of actions puts its buttons under the summary when they do not fit, Estimate leaves its place while the files are at work, and on a low window the sentence under the title gives its room to the list
- On a smaller or lower window the new look packs too: the menu keeps its tiles closer, so Settings at its foot stays in sight, and the line of the speed keeps one row
- With --rm-source (or "Delete originals once checked"), a new file that could only be checked in part, an NCA rewritten by a tool with a section that cannot be checked on its own, no longer replaces its original: the original is kept and the row says why, as Switch Library Manager Web does. This covers Convert, Decompress, Recompress, Trim, Split and Pack

## 1.2.2

### New

- A sound when the work is done, turned on in the menu: a chime of NXPress, or a sound file of your own (WAV, MP3, OGG or M4A, up to 5 MB), with a button to try it and one to go back to the chime (thanks @NikoMuffin, #32)
- Portable: a new download for Windows, nxpress-…-windows-x64-portable.zip, is a folder NXPress with the program and its folder `settings`, ready to unpack anywhere. With a folder named `settings` next to the program, NXPress keeps everything it remembers there (the settings of the window, its size and place, and the files already checked), and nothing in the folders of the user. Put `prod.keys` next to the program too, and the whole folder can go on a USB stick (thanks @NikoMuffin, #5)

### Improvements

- The download for Macs with an Apple chip is named macos-apple-silicon (thanks @NikoMuffin, #25)

## 1.2.1

### Improvements

- The program for Windows is published as nxpress.exe, without the version in its name: a new version replaces the old one without breaking its shortcuts or asking the firewall again. The version is in its properties (thanks @NikoMuffin, #2)
- "Go on" is now "Resume" (thanks @NikoMuffin, #11)
- The window opens where it was left, as big, and maximised when it was (thanks @NikoMuffin, #4)
- Advanced is open, and stays as it was left; the saved settings chosen last are chosen again when NXPress opens (thanks @NikoMuffin, #13, #14)
- Last report, in the menu: the files of the last work, how each ended and why, with the problems first and how many of each. After NXPress turned the computer off, it shows by itself the next time

### Fixes

- Turn off when done did not turn the computer off when a file had failed, and said nothing: it now turns it off once the work is done, failed files or not. When a program keeps the computer on, NXPress tells it, and Force it, beside it, closes such programs without saving (thanks @NikoMuffin, #8)
- The box of the DAT cut its sentence at the start when none was chosen ("…e hashes of the content only"): the sentence shows whole, and only paths are cut (thanks @NikoMuffin, #7)
- Trim marked an NSP or NSZ with nothing to trim OK in green: it is SKIPPED, and says that only updates have delta fragments (thanks @NikoMuffin, #12)
- Measure this PC looked as if it did nothing: the game stayed "Waiting" for the half minute of the first trial, the button did nothing with an empty list, and the threads it set were hidden in Advanced. Each trial now tells it starts ("Measuring 2/6"), the button says it needs an NSP or XCI, Advanced opens on the fields it changed, which light up, and a window shows every setting tried, how fast it went and the one applied. It measures with the first NSP or XCI of the list that can be compressed, and when none can it is SKIPPED with why (thanks @NikoMuffin, #6)
- A file with nothing to compress was marked OK in green with a general sentence: it is now SKIPPED, in yellow, with why (an NCA in sparse storage, sections with an encryption NXPress does not read, NCA files with no game data, or an NSZ named as an NSP), and it is not counted as done. With `--machine-readable` its result is "skipped" (thanks @NikoMuffin, #15)
- FAT32 on Windows: a file split in its own folder (without -o) could not give way to its folder of parts, as NXPress still had it open, so the split failed once its parts were written and checked; with --rm-source into another folder, the original stayed, with an error. It is closed first now

## 1.2.0

### New

- Measure this PC (this Mac, this computer): tries several threads and files at once on a few seconds of the first game of the list, at the level and kind chosen, and puts the fastest in the fields under Advanced; `--measure` on the command line tells the `-t` and `-m` to give. Each trial gives every thread whole pieces of work, so many threads are not measured as slower than they are

### Improvements

- Block compression, and every XCZ, now uses the zstd library in C at the level chosen, as solid compression already did: smaller files at the same level. The estimate uses it too, so it matches what a compression gives
- Decompressing and verifying is faster, from about 1.5 to 3 times: the zstd stream is decoded on several threads and ahead of the decryption, the blocks of NSZ in blocks and of XCZ several at once, and what is written is hashed beside the writing
- The threads of each file are, unless chosen, every processor but an eighth (at least one left free, so the computer stays responsive), instead of half of them and 8 at most: the zstd library in C compresses one file on all of them, 38 % faster with 24 threads than with 12 on a processor of 24. Fewer are used when the memory is short

## 1.1.0

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
- On Linux, install.sh puts NXPress in the menu of applications with its icon (`install.sh --uninstall` takes it away)
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
