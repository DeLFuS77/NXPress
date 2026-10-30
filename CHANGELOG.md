# Changelog

## 1.3.0

### New

- Combine, A SuperNSP per game: a whole collection is packed by game, each game with its updates and DLC, found by the title IDs inside the files and named by a template ("{name} [{id}][v{version}] ({contents})" by default). The games are shown first, each with a box to leave it out; a game with one file only, or combined already, is SKIPPED. On the command line, `--pack-games` and `--pack-name` (thanks @NikoMuffin, #3)
- New fields for Rename and Combine: {version_txt} (1.0.5), {contents} (1G+1U+2D), {size_gb}, {size_mb} and {dlc_number}, and the fields as Switch Library Manager writes them ({TITLE_NAME}, {VERSION_TXT}, {PACKAGE_CONTENTS}...). Rename can keep the names that already have their title ID (`--keep-named`) (thanks @NikoMuffin, #18, #19)
- Verify in levels: Whole (every NCA by its hashes), Signature (the signature of Nintendo on every NCA header, in seconds) or Both; `--verify-level` on the command line (thanks @NikoMuffin, #9)
- Low priority, under Advanced (`--low-priority`): the computer stays responsive while NXPress works (thanks @NikoMuffin, #10)
- The list sorts by name, size or state with a click on a column, and the work follows that order (thanks @NikoMuffin, #17)
- Settings as a page of its own below the sections, and an icon for each section of the menu (thanks @NikoMuffin, #27, #30)
- A menu on each file with a right click (Show in folder, Copy title ID, name or path, Verify only this, Remove from list), a card with the details of a game over its cover, warnings in the list (an older update, a title given twice, a DLC without its game) and a grid of covers
- Before a work starts, NXPress checks that the new files fit on each drive and says where they may not

### Improvements

- Pack is called Combine now (thanks @NikoMuffin, #35), and One file with everything no longer asks for the file first: as in the other sections, it goes next to the files or into Save to, named by a template; `--pack-one` on the command line (thanks @NikoMuffin, #37)
- Measure this PC takes a heavier setting when it is at least 5% faster, shows the speed of all the files together and says why a slightly faster one is not taken (thanks @NikoMuffin, #22)
- The level buttons show their ranges, Light 1–6, Medium 7–14 and Maximum 15–19 (thanks @NikoMuffin, #28); Trim shows "NSP/NSZ (updates)" (thanks @NikoMuffin, #23); and a game in sparse storage is SKIPPED with a plain sentence (thanks @NikoMuffin, #29)
- Info shows the firmware a title needs, the version of its game a DLC needs and its NCA files (thanks @NikoMuffin, #20)
- A new look: blue for what is chosen and started, red only for errors, tiles in the color of each group, motion that tells what changed (less of it when Windows asks for less), Mica on Windows 11, the progress on the button of the taskbar (thanks @NikoMuffin, #31), the space saved over all the works and a card at the end of each work
- New files are on the disk before they take their name and before an original is deleted, so a power cut no longer loses a game
- The command line takes only the files of the folders given, as nsz does; `--recursive` looks into their folders too
- The title of the window shows the version

### Fixes

- A new file never takes the place of an original or of another file of the same work: with Replace files already there and Delete originals, some cases could lose a game (files of the same name sent to one folder, a pack named as one of its files, an output folder written another way). They are now errors that touch nothing
- With Delete originals, a file that could be checked only in part no longer replaces its original
- Combine keeps the ticket of the update it keeps, and deletes the originals only when the pack holds everything
- Splitting or joining for FAT32 in place no longer loses both files when another program holds a part
- Closing the window or stopping the command line during a work asks first and removes the half written files; a turning off never cuts a work
- The list of checked files stays fast when it is long, keeps the games of drives not plugged in and starts again when it is broken; a watched folder waits until a file is copied in whole
- The command line works as nsz does with files already there, letters together (`-CV`) and its output options
- Names made by a template are safe on every system; the window is never smaller than 960 x 600 and fits small screens; and smaller fixes of the window, the keyboard, screen readers and the 11 languages

### Next, in 1.4.0

- Extra compression, experimental and for NXPress only: games in sparse storage, which no compressor reads today, compressed too, and the blocks Nintendo compressed with LZ4 kept decompressed so that zstd gets more out of them (5–9% smaller in the tests, on the games and updates that use them). Installers cannot read these files yet, so it will be off by default, under Advanced, and the files go back to NSP before installing

## 1.2.3

### Improvements

- A file worked on in two steps, the work and then the check of what it wrote (Compress, Recompress, Trim of an XCI, FAT32), has its bar split in them, each in its colour: the work orange, the check green, with the step it is in and its number (1/2, 2/2). The step done stays full and only the one being done fills up, so the bar no longer seems to start again, as in Switch Library Manager Web
- The bar of a file at work moves with stripes while it is being done

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
