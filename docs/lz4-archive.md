# Archive format with the LZ4 of Nintendo made again (`NCZSECT3`)

Status: **experimental, for keeping files only**. Written and read by NXPress, with its extra compression (`--extra`, or the box under Advanced, which also turns on NCZSECT2 for games in sparse storage; `--archive` alone turns on this one). No installer reads it: a file in this format is decompressed with NXPress before it is installed. An NCA without compressed storage is written as NCZSECT2 or plain NCZ, as it would be without it.

## Why

Since firmware 12, an NCA section can use **compressed storage**: its data is cut into blocks, most of them of 64 KiB, and each block is stored as it is, as zeros or compressed with LZ4. Many recent games and updates use it. LZ4 has no entropy coding, and data compressed with it hides most of what zstd could find. NCZ compresses those blocks as they are.

The blocks can be kept decompressed only if they can be made again byte for byte when the NCA is rebuilt: the hash of the NCA must match its content ID. The tools of Nintendo use these compressors, told by the level of each block in the table of the compressed storage:

- **Level 0**: the fast compressor of LZ4 before 1.8.2. It uses a hash of 5 bytes on a table of 13 bits, tries the position right after a match at once, and searches matches only up to 12 bytes before the end of the block. Some games were made with the same compressor built for 32 bits, whose hash takes 4 bytes instead of 5: a block of level 0 is tried with both, and one made with the hash of 4 bytes is told as level **64** in the table of the NCZSECT3.
- **Levels 1 to 16**: LZ4HC of LZ4 r1xx, before 1.7. It searches a hash chain `1 << (level - 1)` times (16 times at level 5, 32,768 at level 16), without the analysis of repeated patterns of 1.8, and starts the second search of a match one byte after the first one.

NXPress has them all, written after `lz4.c` and `lz4hc.c` of LZ4 (BSD 2-Clause License, Copyright Yann Collet). They make again every block of the games tried: 47,648 of an update of Hauntii (level 0), 157,669 of an update of BlazBlue Entropy Effect X and 6,978 of its base game (level 5), 5,999 of BZZZT (level 16, and one of level 0 with the hash of 4 bytes), and 102,622 of CHRONO CROSS THE RADICAL DREAMERS EDITION and 182 of its update (level 0 with the hash of 4 bytes).

Level 16 leaves the LZ4 close to its limit, so zstd gains little over its blocks (BZZZT is 0.7% smaller than as a plain NSZ) and making them again is slow, about 4 MB/s per core. The fast compressor built for 32 bits gains much more: CHRONO CROSS THE RADICAL DREAMERS EDITION is 9% smaller (3.53 GB as a plain NSZ, 3.21 GB as NCZSECT3).

A file with blocks of a level other than 0 and 5 is made by NXPress 1.5.0 or newer: NXPress 1.4 refuses it with "the LZ4 block at … cannot be made again", never with a wrong file.

## Where the blocks are

- **FS header 0x178**: the compression info, with the offset and size of the table and its BKTR header. The table sits at the end of the data level of the hash tree, the last of its levels, and its offset counts from the start of that level.
- **Table**: a bucket tree in nodes of 16 KiB, as Atmosphère reads it: a first node that tells the entry sets and the end of the decompressed data, then the entry sets, one node each, of up to 682 entries. The size the FS header tells is often bigger than the tree: many games leave unused nodes after the last set.
- **Table entries**: 0x18 bytes each. Each entry holds the offset in the decompressed data (s64), the offset in the stored data (s64), the kind (u8: 0 stored, 1 zeros, 3 LZ4), the level (s8), 2 bytes unused and the stored size (u32).
- **In an update**: the data level is in the patched space of the section. The indirect table (FS header 0x100) tells which part of it the update holds. Only those blocks are in its NCA.
- **Base games in sparse storage**: their table is in their update, so they are left as NCZSECT2 does.

## The format

An NCZ in this format is an `NCZSECT2`, with these changes:

| | `NCZSECT2` | `NCZSECT3` |
|---|---|---|
| Magic of the section table | `NCZSECT2` | `NCZSECT3` |
| Stream | the decrypted NCA | the decrypted NCA, with each block of the table decompressed in its place |
| After the stream | nothing | the table of blocks and a footer |

**After the stream:**

- `NCZLZ4RB`
- the number of blocks (u64)
- the size of the packed table (u64)
- the table, compressed with zstd: per block, its offset in the NCA (u64), its size there (u32), its decompressed size (u32) and its level (s8), 17 bytes in all

**The last 16 bytes:** where `NCZLZ4RB` starts, counted from the start of the NCZ (u64), then `NCZLZ4FT`.

A block is kept decompressed only when it is made again byte for byte while the NCZ is written. Any other block stays as LZ4 in the stream and is not in the table. The data is always one stream, never `NCZBLOCK`.

An NCA none of whose blocks is made again is written as `NCZSECT2` or plain NCZ, as it would be without this format, and so is one whose table cannot be read, told with why in the note of the file: the game is still compressed.

## Rebuilding

The stream is read up to the table. At the offset of each block of the table, its decompressed size is read and compressed again at its level. The result must have the size of the table entry, or the NCZ is refused. The NCA is then encrypted by its sections as in any NCZ, and its hash is checked against its content ID.

Tools that do not know `NCZSECT3` stop at the magic: the file is never rebuilt wrong.

## Measured

zstd at level 18, as one stream:

| | Size as NSP | NSZ | NSZ with `NCZSECT3` |
|---|---|---|---|
| Hauntii, update v458752 | 698 MB | 516 MB | 486 MB (47,606 blocks kept decompressed) |
| BlazBlue Entropy Effect X, update v655360 | 4,544 MB | 3,622 MB | 3,475 MB (152,405 blocks) |

Every NSP came back byte for byte. While the NCZ is written, each block is checked on one of the workers, one per processor but one, and goes into the stream in the order of the NCA: compressing the BlazBlue update takes 4 minutes 7 seconds, against 3 minutes 53 seconds as a plain NSZ, the difference being zstd compressing 7.5 GB of decompressed blocks instead of 2.5 GB of LZ4. When the file is rebuilt, the blocks are made again the same way while the stream is read ahead. On a processor of 24 threads, rebuilding takes 2 seconds for the Hauntii update (level 0 blocks) and 17 to 21 seconds for the BlazBlue one (level 5), against 13 to 14 seconds for the same update as a plain NSZ. On one core it took 5 seconds and 1 minute 13 seconds: the level 5 compressor makes about 55 MB/s per core.
