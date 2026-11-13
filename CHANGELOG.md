# Changelog

## Unreleased

### Fixes

- Optimize for install: the help, the window and the docs tell to delete the update of the game from the console before installing an optimized game. An update left there is read over it and the game closes with an error. Thanks to Trickster101 (GBAtemp) for reporting it

## 1.7.12

### Improvements

- An image of your own for an icon (Add cover art icon in Combine, Choose image in Details) opens in a crop first: drag and zoom it under the square, or fit it whole, and see the icon as it will be

### Fixes

- Closing NXPress while files are at work asks in a dialog of the window, in its light or dark theme, not in a white one of the system. Thanks to NikoMuffin for reporting it (#115)

## 1.7.11

### Improvements

- Combine: **Add cover art icon** on each dump folder without an icon of its own. The image chosen (JPG, PNG, WebP or BMP) goes into its NUP at 256×256 instead of the one of titledb, which knows few Switch 2 games
- `-V` tells when the CNMT of an NSP or NSZ lists a content the file does not hold, which installers such as Sphaira do not find; `--rebuild` pairs it with the NCA of its type. Thanks to lightwo (GBAtemp) for reporting it
- macOS: NXPress now comes as a disk image (`.dmg`) instead of a `.pkg`: open it and drag NXPress to Applications. It runs no script. Thanks to NikoMuffin for the idea (#112)

## 1.7.10

### Improvements

- `--rebuild` writes a package again as a clean NSP (an NSZ with `-C`): its NCA headers made right, standard crypto with no ticket, a new CNMT and a new PFS0, its data as it is. For a package another tool left that some installers do not take; it needs sigpatches. What it makes right: docs/rebuild.md. Thanks to lightwo (GBAtemp) for the idea and the testing
- Details and `-i` tell when the NCA of a file are not signed by Nintendo: it needs sigpatches, and some installers may not take it
- NXDock is now among the installers Details and `-i` list, also for the extra compression

## 1.7.9

### Improvements

- **Optimize for install** (experimental): turns a game and its update into one game, already at the version of the update, that takes much less room once installed: from a few percent to more than 80% less than the game and its update installed apart. Its data is compressed with LZ4, which the system reads directly, and what the game holds twice is stored only once
  - In the window: Compress › **Optimize for install**, next to Extra compression. Add the game and its update (NSP, NSZ, XCI or XCZ); an NSZ comes out next to the game, which installs as that same game
  - On the command line: `nxpress --optimize GAME UPDATE` writes an NSP; with `-C`, an NSZ
  - Each file is checked whole before it gets its name: the hashes of every section, and its RomFS, which must be the one of the game with its update
  - It installs with NXDock or DBI, with sigpatches and firmware 12.0.0 or newer. Tested with DBI
  - Official updates do not go over it: when a newer one comes, optimize the original game again with it. The originals are never deleted
  - DLC are left as they are. How it is made: docs/optimize-install.md

**What you lose with it**

- **Official updates.** They cannot be installed over it, not even from the eShop. To move to a newer version, optimize the original game again with the new update, so keep the original game and its update on your PC
- **The signature of Nintendo.** The program and the metadata are made again, so sigpatches and firmware 12.0.0 or newer are needed. DBI marks them as Fake when it installs them
- **Going back to version 1.0 just like that.** The optimized game is already at the version of the update; to go back, install the original again
- **Time and room on the PC.** Optimizing takes from seconds to about a minute per game in the tests. While it works it needs room for the new file and, for an NSZ or XCZ, for the decompressed NSP too
- **DLC save nothing.** They stay as they are

What you do not lose: your saves, the DLC installed and the game itself. What the game reads is the same as with its update, byte for byte, and that is checked before the file gets its name.

## 1.7.8

### Improvements

- An installer for Windows, now the recommended download, and one for macOS: in the 11 languages of NXPress, with the shortcuts and options of Settings, and an uninstaller. `nxpress.exe`, the portable zip and the archives of macOS are still there
- An NXPress installed in Program Files updates itself too, after asking Windows for an administrator

## 1.7.7

### Improvements

- The list of every section has a search field, as the log has: as you type, it shows only the files whose name, game, title ID, folder or note have what is written, and tells how many of the list it shows. The files left out of sight are still worked on (thanks @NikoMuffin, #105)
- Check now, under “Look for new versions on GitHub” in Settings, looks for a new version at once and tells when this one is the newest; the answer is asked fresh, not one GitHub kept from a moment ago (thanks @NikoMuffin, #106)

### Fixes

- The cover of a NUP in the list and in the grid shows the icon given to it at once, without leaving Details and coming back (thanks @NikoMuffin, #107)

## 1.7.6

### Improvements

- The box under “Open Switch files with NXPress” in Settings › Windows Explorer reads “Verify them as soon as they open”, which tells better what it does (thanks @NikoMuffin, #87)
- A NUP whose game titledb has no icon of, as many of the Switch 2, can get one from the eShop: Search image in its card opens a window that searches the eShop of Nintendo by the name of the game and shows the games it finds with their square image (500×500, which makes a sharp icon of 256×256), the Switch 2 only unless that box is unticked. The name searched is the one the game carries, without the marks of file names, and can be changed, as file names and title IDs are often wrong. The eShop is asked only when a search is started. For a game it does not have, Search the web opens a search of images in the browser (DuckDuckGo), and an image copied there is pasted into the card or the window with Ctrl+V (thanks @NikoMuffin, #102)
- A NUP given an icon can have it changed: Change image in its card offers the same ways again (titledb, the eShop, an image of one's own or one pasted). The icons a NUP carries from the game itself are kept (thanks @NikoMuffin, #103)

## 1.7.5

### Improvements

- A NUP made of a dump folder without control data gets the icon of its game: NXPress finds the game in titledb by its title ID (the one of the game for an update or a DLC), downloads its icon and writes it into the NUP as a JPEG of 256×256, as the icons of the console are. A NUP made already without one gets it from its card (Icon from titledb), and an image of one's own (PNG, JPEG, WEBP or BMP, cut square at its middle; the note tells its size, and when it was made bigger) with Choose image, for a game titledb does not know; on the command line, `--fix-nup-icon`, `--nup-icon FILE` and `--no-nup-icon`. Nothing the NUP has changes: the metadata of the game is written again after its end with the icon, the files stay as they were, and a NUP with an icon keeps it. It goes on the internet only with “Warn of updates you do not have” on, which now also downloads the list of the icons of titledb when a NUP needs one, once a week at most; with no connection, or a game titledb does not know, the NUP is made all the same and the note tells why (thanks @NikoMuffin, #97, and QuickWick (Wicked on Discord), who suggested it on Discord and GBAtemp)

### Next

- NUP v2, the new version of the format, as its specification settles: reading and verifying first

## 1.7.4

### Improvements

- Each section keeps its own view, list or covers; one not switched yet shows the view chosen before (thanks @NikoMuffin, #84)
- The Log is no longer in Settings › Help: it is in the side menu, under Check, with its settings on its page (thanks @NikoMuffin, #85)
- Settings › Help opens the Discord server NXPress shares with Switch Library Manager Web (thanks @NikoMuffin, #86)
- Windows: NXPress can be the program of NSP, NSZ, XCI, XCZ and NUP files, from Settings › Windows Explorer: a double click opens them in Verify, and checks them at once when its box is ticked. Where another program opens them already, NXPress is chosen once with Open with › Choose another app › Always (thanks @NikoMuffin, #87)
- Windows: an NXPress menu on a right click of those files, with Verify, Details, Compress or Decompress, and Combine, and of folders, with Compress, Verify and Combine; on Windows 11 it is under Show more options. Send to stays. Both are for the user only, need no administrator, and Settings tells when they are for another copy of NXPress, as after moving it (thanks @NikoMuffin, #88)
- Windows: files sent from the Explorer while NXPress is open go to its window, which comes to the front, as a new list that Undo takes back; during a work they come in when it ends. Several files opened at once come together in one window
- Export, in Details and in the Log, saves the list to Excel (XLSX) or CSV: in Details a row for each title of each file, with its title ID, kind, versions, name, publisher and the newest version published; in the Log a row for each file of the works shown (thanks @NikoMuffin, #89)
- Recompress has the extra compression too, both ways: ticked, a file gets it; unticked, a file with the archive format NCZSECT3 becomes a plain NSZ that installers read. A file that changes format goes through an NSP checked by its hashes, so the round trip stays byte for byte; it needs the keys and room for the NSP. A game in sparse storage (NCZSECT2) has no plain NSZ, so without the box it stays as it is (thanks @NikoMuffin, #92)
- Saving settings with a name in use asks first: Save becomes Replace, in orange, with a line that says so, and Undo brings the old ones back; the names of the settings of NXPress are not taken (thanks @NikoMuffin, #93)
- NXPress can install its new versions itself: the notice of a new version offers Install and restart, which opens a window with what is new while the version is downloaded and checked, then Restart now or Later. Each release now signs its checksums with the key of the project (SHA256SUMS.txt.minisig, minisign), and NXPress checks that signature, made for that version, and the checksum of the file it downloads before it replaces itself; a version without its signature is only offered to download, as before. Nothing is installed without a click, a beta only by a beta, and the restart waits for every work to end: the work of the files (a watch of folders too), the files sent from the Explorer meanwhile and the check they start, the log of the work, and a count to turn the computer off; no work starts while it restarts. Windows and Linux; on macOS it still only tells (thanks @NikoMuffin, #91)

### Fixes

- Windows: many files sent from the Explorer at once while NXPress is minimized no longer leave its window not responding: it comes back to the front and takes them all
- A NUP made to harm, such as one from the Internet, no longer makes NXPress take gigabytes of memory or hours, or close, when it is added, checked or unpacked: names and paths longer than the ones of the console, folders that share their children, packages that share their folders and a file at more than one place are refused, and the check of the code decompresses as it reads
- A NUP whose metadata types are counted from 0 is told, and fixed, also when a package has no NPDM, such as a DLC: a record 0x0007 that is not the 32 bytes of a build ID marks it, the same rule as the fix of the reference packer (a55uka/nup#32)

### Next

- NUP v2, the new version of the format, as its specification settles: reading and verifying first

## 1.7.3

### Improvements

- Extra compression sits under Made for in Compress, and its warning shows in Compress only (thanks @NikoMuffin, #76)
- Watch the folders says which folders it watches (thanks @NikoMuffin, #77)
- Combine can delete the dump folders once their NUP is read back whole and checked, with a box under Delete originals, off by default (`--rm-dumps`): a file the NUP does not hold, or one changed during the packing, stays (thanks @NikoMuffin, #79)
- Shorter texts under each section of the side menu, in every language (thanks @NikoMuffin, #80)
- Verify and Details tell whether Eglantine and Lichii, the emulators that open NUP files, open a NUP and can start its game, and why not
- A NUP whose metadata types are counted from 0, as an early build of the packer of the format wrote them, is told, and Details fixes it in place (`--fix-nup-meta`), so the emulators show its name, version and icon right
- NUP files made by Combine suit the emulators better: files over 256 MiB are stored as they are by default, since the emulators decompress them whole to the disk each time the game starts (`--nup-zstd-big` compresses them), each update carries its own version, and the NUP of a Switch 2 dump without control data takes the name of its game

### Fixes

- Verify reads the code of NUP files and dump folders, and tells a dump whose code lost bytes, a game that stops as it starts: it must be dumped again. Combine still makes its NUP, and the work fails with the warning
- Rename and the list give a NUP that carries no name a readable one, from its dump

### Next

- NUP v2, the new version of the format, as its specification settles: reading and verifying first

## 1.7.2

### Improvements

- Rename takes NUP files too, without keys: a NUP is named by what it holds, with the same fields as a game file (the name of the game, its title ID, the version of its newest update and the rest); a NUP that does not carry the name of its game keeps the one of its file (thanks @NikoMuffin, #82)

### Fixes

- The note told when a NUP is given to a work that does not take it no longer says NXPress does not compress NUP files, which Combine makes compressed with zstd: it now tells which work does what with them

## 1.7.1

### Improvements

- Expand all and Collapse all, in the Log next to its filter, open or close at once every work shown, with the filter and the search as they are (thanks @NikoMuffin, #75)

### Fixes

- The Log no longer jerks when a work is opened, and the hour stays on one line, also as "12:01 AM" (thanks @NikoMuffin, #78)

## 1.7.0

> **Note:** the support of NUP files is new. It was tried with the games at hand, and packed NUP files were tried in Eglantine, but not with every case yet, so some rough edges may remain.

### New

- NUP files (.nup), the containers of decrypted Switch and Switch 2 games, are read and checked without keys: Details shows each package (the game, an update or a DLC) with its files and sizes, and Verify checks every file by its SHA-256. NUP files whose metadata types a build of the packer of the format counted from 0 show the name of their game all the same (thanks @brumouta, DeLFuS77/switch-library-manager-web#157)
- Verify also looks at the metadata of each package of a NUP, which no hash of the format covers: a build ID of the wrong size, text that is not UTF-8 or records of languages that run past their block are told as damaged, and an NPDM or a build ID other than the ones of its ExeFS, or an icon that is neither JPEG nor PNG, are told in its result
- Dump folders become NUP files in Combine (`--nup`), and NUP files become folders again in Decompress (`-D`), without keys. The dump folders of OunceCake are taken in its old layout and its new one; each file is stored with the zstd library in C, a big one on every thread, or as it is, without trying zstd on data that is compressed already (the .zs files of games, videos and images); files with the same data are kept once, the same folder gives the same NUP byte for byte whatever the number of threads, the name of the game goes in the NUP so emulators show it, and the new NUP is read back and checked before it gets its name. Unpacking checks every file by its SHA-256 as it writes it and never writes into a folder that is there already

### Improvements

- Combine and Split name the SuperXCI too, not only the SuperNSP: a game card with its updates and DLC is made by Combine when an XCI is among the files, and Split writes each of its titles to a file of its own (thanks @NikoMuffin, #73)

### Security

- github.com/klauspost/compress 1.20.3 (GO-2026-6865), which fixes a crash with zstd data crafted for it

## 1.6.0

### New

- Log, under Check: every work NXPress did, the newest first and by day, each with its files, how they ended and what they saved. It finds a game or a file, and can show only the works with problems. It is kept in log.txt in the settings folder too, with how many works to keep (100, 500, 1000 or no limit) and a button to clear it, which Undo takes back. It takes the place of Last report in Settings › Help, and opens by itself after the computer was shut down at the end of a work (thanks @NikoMuffin, #69)

### Improvements

- The help of the options no longer fills the panels: a small ⓘ after the name of an option tells there is some, and it shows in a card while the mouse is on the name or the option has the focus of the keyboard. The description of each section is beside its title the same way, and so is the help in Settings; warnings stay in sight (thanks @NikoMuffin, #67)

### Fixes

- Low priority now reaches the window too: on Windows its WebView2 processes go below normal with NXPress, and its memory is the first given to the other programs when memory runs short (thanks @NikoMuffin, #70)
- The extra compression no longer takes every processor for each file: its LZ4 blocks are checked and made again on the threads of each file, and never on more than its share of the processors when several files are worked on at once, so the computer keeps answering while they are compressed, decompressed or verified (thanks @NikoMuffin, #70)

## 1.5.1

### Improvements

- Re-verify files already checked, as it is now called, is back at the top of Verify, out of Advanced; only Compare with a DAT stays there (thanks @NikoMuffin, #65)
- When done says Shut down instead of Turn off, and choosing it no longer moves the bar at the bottom (thanks @NikoMuffin, #66)

## 1.5.0

### New

- Updates that are out and not in the list: once a day at most, NXPress downloads the versions published (the versions.json of Switch Library Manager Web, about 200 KB), and the newest file of each game says which update is out and which one you have. On by default; it can be turned off in Settings › System, and `-i` on the command line tells it too
- Colors to choose in Settings › General: NXPress's own and three more, RuneStone, Vampire and Huntress, in dark and in light, each with its own scene while the list is empty. Red, green and orange keep meaning errors, savings and warnings

### Improvements

- Verify says VERIFIED and CORRUPTED, as other tools do; a file that could not be checked keeps FAILED with its reason, and the command line keeps OK, MODIFIED and FAILED (thanks @NikoMuffin, #55)
- Verify: Compare with a DAT and Check again files already checked move to the bottom of Advanced (#52), and the hint of the watched folders is clearer (#51) (thanks @NikoMuffin)
- "Replace if the new file is already there", with a line that says what it does (thanks @NikoMuffin, #56)
- A game in sparse storage says how to compress it: with Extra compression under Advanced (thanks @NikoMuffin, #57)
- Settings in groups (General, While working, Keys, System and Help), with Low priority in it for the whole app (thanks @NikoMuffin, #60)
- Show in folder is a small button beside the name of the game (thanks @NikoMuffin, #61)
- When done: Nothing, Turn off or Hibernate, with the same 60 seconds to call it off (thanks @NikoMuffin, #62)
- Extra compression makes again two more LZ4 compressors of Nintendo that many recent games use, level 16 and the fast one built for 32 bits: CHRONO CROSS THE RADICAL DREAMERS EDITION is now 9% smaller than as a plain NSZ. A file that uses them is decompressed by NXPress 1.5.0 or newer and Switch Library Manager Web 1.72.1 or newer; older versions refuse it with an error, never with a wrong file (thanks @NikoMuffin, #63)

### Fixes

- Extra compression no longer stops a game with "invalid bucket tree" or "invalid NCZSECT3 table", as on BZZZT, CHRONO CROSS THE RADICAL DREAMERS EDITION and its update: the LZ4 table of Nintendo is read as Atmosphère reads it, and an NCA that cannot use it is written as a plain NCZ instead of failing the game (thanks @NikoMuffin, #63)

### Security

- A file crafted to make a parser give up fails that one file instead of closing the window; the versions.json downloaded is decompressed with a bounded window; the window declares a Content-Security-Policy; built with Go 1.27.3

## 1.4.1

### Improvements

- The files in an experimental format of NXPress say so in the list and in the covers view: "Experimental" next to the name of the game, with the format beside it (NCZSECT2, NCZSECT3), so they are found again without decompressing anything. Rename has a new `{experimental}` field that writes the format in the name, and is left empty, with its brackets, for any other file. The card of such a file in Details no longer lists DBI, Tinfoil and the other installers as able to read it: only NXPress (thanks @NikoMuffin, #58)

## 1.4.0

### New

- Extra compression, experimental and NXPress only: `--extra`, or "Extra compression, NXPress only" under Advanced, off by default. Base games in sparse storage, such as No More Heroes 3 or BlazBlue Entropy Effect X, which no compressor could shrink, go into an extension of NCZ, NCZSECT2 (docs/ncz-sparse.md), 6 to 9% smaller; and the blocks Nintendo compressed with LZ4 inside many recent games and updates are kept decompressed and made again byte for byte with its two LZ4 compressors, NCZSECT3 (docs/lz4-archive.md), about 6% smaller than a plain NSZ. Each file gets what it needs, and one that needs neither stays a plain NSZ; `--sparse` and `--archive` turn on one half each
- Installers and emulators do not read the two extensions yet: a file that uses them is decompressed with NXPress before installing it, and it comes back byte for byte. Tested with DBI

### Improvements

- Verify: Whole is called Hash now. A file whose NCA are not signed by Nintendo while nothing in it is damaged is MODIFIED, in orange, and no longer counts as failed: a tool rewrote it, and it usually installs and plays. A damaged file is still FAILED, in red. With Both, the hashes are checked even when the signature fails. On the command line, `--verify-level hash` (`full` still works) (thanks @NikoMuffin, #42)
- The fields of the name templates of Rename and Combine are in a table, one to a row, with what each writes, an example and its name as Switch Library Manager writes it (thanks @NikoMuffin, #43)
- The bar at the top with the name and the version is gone, as both are in the title of the window; the notice of a new version still shows there when one is out (thanks @NikoMuffin, #44)
- Combine: the pack of a folder dropped or chosen goes next to the folder, not inside it, so a collection with each game in a folder of its own gets the SuperNSP of each game next to its folder. Files added one by one keep it next to them. The same on the command line (thanks @NikoMuffin, #45)
- The empty list tells what NXPress has saved since always only once a work of the session has saved space, not each time the window opens (thanks @NikoMuffin, #48)

## 1.3.1

### Fixes

- On macOS, the layer shown while files are dragged over the window no longer stays after they are dropped, covering the window so that nothing could be clicked (thanks @ong19th, #46)

### Security

- golang.org/x/net 0.62.0 (GO-2026-6860 and GO-2026-6866)

## 1.3.0

> **Caution:** this version brings many changes at once, and not every test planned for them has been done yet, so some bugs may remain. Keeping a copy of the games before using Delete originals is recommended, and any problem is welcome in the issues.

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
