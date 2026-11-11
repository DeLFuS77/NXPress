# Optimize for install (experimental)

*Optimize for install* turns a game and its update into one NSP that takes the least room once it
is installed. It is experimental: the NSP it writes is a game laid out in a way Nintendo never
ships, so it needs **sigpatches** and **firmware 12.0.0 or newer**, and it is installed with
**DBI**. **Do not install official updates over it**: keep the game and its update, and optimize
them again from those originals when a newer update comes.

```
nxpress --optimize GAME UPDATE              # one NSP per game, next to its files
nxpress --optimize -C -o OUT FOLDER         # an NSZ per game (zstd over the NSP), into OUT
```

In the window it is the box **Optimize for install** of Compress, next to Extra compression: with it
on, the list takes NSP, NSZ, XCI and XCZ files, and each game with its update becomes one NSZ.

## What it writes

The files given are grouped by game, as `--pack-games` groups them (a SuperNSP, an XCI or an XCZ
holding the game and its update works too). For each game with its base and an update, the newest
update is taken; a game without its update, an update without its game and the DLC are left as they
are, each told why. The NSP holds:

| File | What it is |
|---|---|
| `<id>.nca` | The program NCA, merged (below). Its content ID is the first half of its SHA-256. |
| the other contents of the update | Its control NCA, legal information, HTML documents..., copied as they are. Its delta fragments are left out. |
| `<id>.cnmt.nca` | A new meta NCA (below). |
| `<rights ID>.tik`, `.cert` | The ticket and certificate of the update, for the NCAs that have a rights ID. |

It is named by `--pack-name` (the fields of `--rename`), by default
`{name} [{id}][v{version}] (optimized)`: the title ID of the game at the version of the update.

### The merged program NCA

It is the program NCA of the update made whole, so it no longer needs the base game:

- **Header**: the one of the program NCA of the update, with the same rights ID, key generation and
  program ID (its ticket keeps opening it). Only the sections and their FS header hashes change, so
  its header is no longer signed by Nintendo: this is why sigpatches are needed.
- **ExeFS and logo**: those of the update, byte for byte, moved to their new place.
- **RomFS**: the RomFS the game reads with its update (the base game through the indirect table of
  the update, its sparse storage when it has one, and the compressed storage of either), written as
  a plain RomFS section:
  - AES-CTR with the counter of the section (no AES-CTR-Ex, no indirect table, not sparse);
  - its IVFC laid out as Nintendo lays it out for the RomFS decompressed: hash levels from 0, one
    after another, each a whole number of 16 KiB blocks, the data level last. The levels hash the
    data **as it is stored** (compressed), as the FS checks it; each level keeps the size it would
    have for the data decompressed, with zeros after its hashes, as in the games Nintendo
    compresses;
  - the **compressed storage layer** of the NCA (the one the firmware reads since 12.0.0): a table
    of entries, each a block of the RomFS stored as LZ4 or as it is, in blocks of 64 KiB:
    - each block is compressed with LZ4 HC at level 9 (the LZ4 of Nintendo, `lz4nintendo`);
    - when the game was compressed by Nintendo, each block keeps the smaller of Nintendo's own LZ4
      of it and the new one, so nothing grows;
    - a block that does not get at least 64 bytes smaller is stored as it is;
    - what the update reads more than once from its base game, and any 64 KiB block that is the
      same as one stored before it, is stored once: several entries of the table point at the same
      stored block (also back to blocks stored before them).

The NCA is written in one pass after a first pass that measures it (and keeps only the hashes of
what it stores); the second pass must give the same hashes, or nothing is written. The bytes are
the same for any number of threads.

### The new CNMT and meta NCA

The CNMT is the one of the base game (same title ID, attributes and extended header) with:

- the **version of the update**, and its type still Application: the console sees one game of that
  version and no update;
- the contents of the update, the merged program in place of its program, without the delta
  fragments;
- **no required application version** (the console does not ask for an update it already has), and
  as the required system version the higher of the two and 12.0.0;
- no extended data (the history of the patch).

Its meta NCA is made from the one of the base game: the same header and key area, one section
holding a PFS0 with `Application_<title id>.cnmt`, hashed in blocks (HierarchicalSha256), as
Nintendo makes them.

## Checks

Before it gets its name, the new file is opened again from the disk and checked whole:

1. the RomFS of the game with its update is read first as the game reads it, every block checked
   against the IVFC of the update (a base game of another version, or a damaged one, stops it
   there), and its SHA-256 kept;
2. after it is written: the CNMT of the meta NCA lists every NCA of the NSP; the size in the header
   of the program; the SHA-256 of each FS header; every block of the ExeFS and the logo; the RomFS
   read as the game reads it, every block checked against the new IVFC and every LZ4 block
   decompressed, which must give the SHA-256 of step 1;
3. every NCA against its content ID (the check of `-V`).

With `-C`, the NSZ is then made and checked against the NSP as any NSZ.

## Limits

- One game with one program: games of several programs are not optimized yet.
- The update must come with its ticket (a common ticket): a title key of `title.keys` alone does not
  make an NSP that installs.
- `--rm-source` and `--watch` do not go with it (the originals are what a game is optimized again
  from), nor `--extra` (installers read an optimized NSZ as it is).
- Built with cgo only: a build of NXPress without a C++ compiler tells that it has no Optimize for
  install.

## Measured

Eight games with their updates, each NSP checked as above, with sizes as installed (the NCAs of the
game and its update, against the NCAs of the optimized NSP):

| Game | Installed before | Optimized | Smaller |
|---|---|---|---|
| A | 4,501.0 MB | 2,524.2 MB | 44 % |
| B | 476.7 MB | 194.6 MB | 59 % |
| C | 4,389.8 MB | 711.9 MB | 84 % |
| D | 5,255.8 MB | 1,901.6 MB | 64 % |
| E | 965.8 MB | 196.4 MB | 80 % |
| F | 3,138.2 MB | 1,645.5 MB | 48 % |
| G | 1,209.9 MB | 427.2 MB | 65 % |
| H | 5,670.5 MB | 885.7 MB | 84 % |

Games whose data is mostly audio or video already compressed save much less (7 to 16 % in the tests
of NXDock).

## The code, shared with NXDock

The merge is the C++ of NXDock, the homebrew that does the same on the console:
`internal/optimize/native` holds a copy of its `patch_view`, `romfs_lz4`, `merge_pack`, `merge_nca`,
`merge_game`, `merge_meta` and `lz4nintendo`. They are plain C++20 (and C for `lz4nintendo`): AES,
SHA-256 and the reads are given to them as functions, so NXPress gives them those of Go through a
small C layer (`bridge.h`, `bridge.cpp`, `native.go`, `callbacks.go`). Go opens the packages, finds
the keys and the title keys, and writes the NSP.

The copy is **never edited in NXPress**. `tools/nxdocksync` copies the files from a commit of NXDock
(only what is committed) and writes `NXDOCK.sum`, the commit and the SHA-256 of each file;
`TestNXDockCopiesUnchanged` fails when a copy differs from it, or when another C or C++ file is in
the folder. A change goes to NXDock first, then:

```
go run ./tools/nxdocksync -from ../NXDock -rev <commit>
```

A copy was chosen over a git submodule because NXPress builds from a plain checkout (CI, `go
install`, source archives) and Go does not fetch submodules; the files change seldom, and the sum
keeps the copy honest. NXDock is GPL-3.0-or-later and NXPress AGPL-3.0, which the GPL allows to be
combined (section 13), and both are by the same author.

cgo compiles the C++ with the compiler of the build (`CXX`): zig c++ or gcc on Windows (linked
statically, so the program needs no DLL of the C++ library), gcc on Linux, clang on macOS.
