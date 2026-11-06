# NCZ with counter offsets (`NCZSECT2`)

Status: **experimental**. Written by NXPress, which also reads it back, with its extra compression (`--extra`, or the box under Made for, which also keeps the LZ4 blocks of Nintendo decompressed: docs/lz4-archive.md; `--sparse` alone turns on this one). Other tools do not read it yet.

This page describes a small extension of the NCZ format of [nsz](https://github.com/nicoboss/nsz). It exists so that NCAs in **sparse storage** can be compressed and then rebuilt byte for byte. Tools that do not know the extension stop with an error instead of writing a wrong NCA.

## Why NCZ cannot hold a sparse NCA

Some base games are released "compacted" to go with their day-one update. Their program NCA uses **sparse storage**, and holds only the pieces of the game that the update does not replace. Examples: No More Heroes 3, Lost in Random, Have A Nice Death, BlazBlue Entropy Effect X, and Hauntii on game card.

Inside such an NCA:

- **The stored pieces are packed one after the other**, starting at the physical offset of the section. The FS entry in the NCA header gives the full virtual size of the section, which is far beyond the end of the file.
- **A bucket table (`BKTR`) at the end of the packed area** maps each piece to its virtual offset. Index 0 points to the stored data and index 1 to zeros.
- **Each piece is still encrypted as it was in the full NCA.** It uses AES-CTR with the normal upper counter of the section, and the lower counter comes from its **virtual** position: FS entry start plus virtual offset, divided by 16.
- **The table is encrypted with the section's upper counter, with its generation replaced** by `sparse generation << 16`. The lower counter is its absolute position in the file.

An NCZ section re-encrypts its bytes with the lower counter taken from their **absolute position in the NCA** (`offset >> 4`). For a stored piece, that position is not the one its bytes were encrypted at. No key or upper counter can make up for a different lower counter. So until now, compressors stored these NCAs as they are: nsz copies them ("Skipping not packed"), and SAK and nut skip them too.

## The format

An NCZ in this format is a regular NCZ, with two differences in its section table:

| | `NCZSECTN` (nsz) | `NCZSECT2` (this extension) |
|---|---|---|
| Magic at NCZ offset 0x4000 | `NCZSECTN` | `NCZSECT2` |
| Section entry +0x18, 8 bytes | unused, 0 | `counterOffset`, signed 64-bit, little endian |

Everything else stays as nsz writes it:

- **The first 0x4000 bytes** are the NCA header, stored as it is.
- **The section table follows**: magic, section count (u64), then 0x40 bytes per section: offset, size, crypto type, `counterOffset`, key (16 bytes), counter (16 bytes).
- **The sections cover the NCA without gaps and in order**, from 0x4000 to its end.
- **The data follows**: one zstd stream, or `NCZBLOCK` with its blocks.

### Rebuilding a section

For a section of crypto type 3 or 4, the byte at absolute NCA position `p` is encrypted with AES-128-CTR, with the section's key and this 16-byte counter:

```
counter[0..8]  = section.counter[0..8]
counter[8..16] = big endian u64 ((p + counterOffset) >> 4)
```

The byte uses the keystream byte at index `(p + counterOffset) & 15` of that block.

- **When `counterOffset` is 0,** this is exactly NCZ as it is today.
- **`counterOffset` must be a multiple of 16**, so the index is `p & 15`, as before.
- **For any other crypto type,** `counterOffset` must be 0.

A writer uses `NCZSECT2` only when at least one section has a non-zero `counterOffset`. Any other NCZ stays `NCZSECTN`, readable by every tool.

### What a reader must do

- **Accept both magics.**
- **With `NCZSECT2`,** add `counterOffset` to the position before deriving the counter.
- **Reject the file** if `counterOffset` is not a multiple of 16, or is set on a section that is not encrypted.

For nsz, a reader accepts `NCZSECT2` where it checks the magic, reads the field at +0x18 of each section (padding today), and adds it where its decompressors place the counter:

```python
crypto.seek(i + s.counterOffset)
```

A reader that does not know `NCZSECT2` refuses it on the magic. nsz asserts `NCZSECTN` and stops. The file is never rebuilt wrong.

## Compressing a sparse NCA (how NXPress does it)

A section is sparse when the u16 at 0x170 of its FS header, the sparse generation, is not 0. For such a section, with physical offset `P`, table at `P + table.offset` with `table.size` bytes, and FS entry start `S`:

1. **Decrypt the table.** Take the upper counter of the section with its generation (the u32 at FS header 0x140) replaced by `generation << 16`. The lower counter is the absolute position of the table.
2. **Read the table as an IndirectStorage bucket tree.** Nodes are 16 KiB. Entries are `{u64 virtual offset, u64 physical offset, s32 storage index}`, 0x14 bytes each. The end offset in the first node header is the virtual size.
3. **Write one NCZ section per entry with storage index 0**, covering `[P + physical, P + physical + length)`:
   - crypto type 3,
   - the section's key and upper counter,
   - `counterOffset = (S + virtual) - (P + physical)`.
4. **Write one more section for the table** (and the meta hash data after it, if there is any), with the sparse upper counter and `counterOffset = 0`.
5. **Treat the rest of the NCA as usual:** the sections that are not sparse as nsz does, and the bytes between sections as crypto type 1.

A sparse section with an empty table (entry count 0) has no stored bytes.

**Checking.** The rebuilt NCA must have the SHA-256 its content ID tells. NXPress checks every NCZ it writes this way before keeping it.

## Notes

- **A sparse base game only runs with its update**, as it was released. Compressing it does not change that.
- **Measured gain on the stored data:** 6 % for Have A Nice Death (181 MB to 170 MB) and 8.6 % for BlazBlue Entropy Effect X (571 MB to 522 MB). Most of these games' data is compressed already; the update, which holds most of the game, compresses as usual.
- **Sources for the layout of sparse storage:** Atmosphère (`fssystem_nca_file_system_driver.cpp`: `CreateSparseStorage`, `CreateSparseStorageMetaStorage`; `fssystem_nca_header.hpp`: `NcaSparseInfo`) and nxdumptool (`bktr.h`, `nca.c`).
