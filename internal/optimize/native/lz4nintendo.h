// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 DeLFuS77
//
// The two LZ4 compressors of Nintendo's tools, which made the LZ4 blocks of the compressed
// storage of an NCA (firmware 12 and later). They make the same blocks again byte for byte, so an
// NCZSECT3 of NXPress, which keeps those blocks decompressed, can be turned back into its NCA.
#pragma once

#include <stddef.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

// The level an NCZSECT3 gives a block of level 0 made by the fast compressor built for 32 bits
// (a hash of 4 bytes); level 0 is the one built for 64 bits (a hash of 5 bytes). Levels 1 to 16
// are LZ4HC r1xx, searching its hash chain 1 << (level - 1) times.
#define NX_LZ4_LEVEL_FAST4 64

// The memory a compression needs as work space, given by the caller (no allocation inside).
#define NX_LZ4_WORK_SIZE ((size_t)(1u << 15) * 4u + (size_t)(1u << 16) * 2u)

// The most bytes a block of n bytes may take compressed.
size_t nx_lz4_bound(size_t n);

// Whether level is one of the levels above.
int nx_lz4_level_known(int level);

// Compresses src as the block of that level was made, into dst (cap bytes), with work of
// NX_LZ4_WORK_SIZE bytes. Gives the size written, or -1 for an unknown level, a dst too small or
// a block the compressor cannot make.
int64_t nx_lz4_compress(const uint8_t* src, size_t n, uint8_t* dst, size_t cap, int level, void* work);

// Decompresses an LZ4 block into exactly size bytes; gives 0, or -1 for a damaged block.
int nx_lz4_decompress(const uint8_t* src, size_t n, uint8_t* dst, size_t size);

#ifdef __cplusplus
}
#endif
