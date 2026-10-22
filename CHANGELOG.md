# Changelog

## Unreleased

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
