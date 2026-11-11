// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 DeLFuS77
#pragma once

// The program NCA of a game merged with its update (25C): the header and the sections of the
// program NCA of the update (its ExeFS, and its logo when it has one), with its RomFS made a plain
// one: AES-CTR with the counter of the section, no indirect table, not sparse, its IVFC laid out as
// Nintendo lays it out for the RomFS decompressed, and the LZ4 layer of the NCA (merge_pack). The
// rights ID, the key generation and the program ID stay those of the update, so its ticket keeps
// opening it. The header is no longer signed by Nintendo: sigpatches are needed, as for any game
// NXDock compresses.
//
// Plain C++: tested on the computer.
#include "romfs_lz4.hpp"

#include <array>
#include <cstdint>
#include <vector>

namespace nxdock::mergenca {

// The IVFC of a RomFS as Nintendo lays it out: hash levels from 0, one after the other, each a
// whole number of blocks and sized for `size` bytes of data (the RomFS decompressed, or what is
// stored when that is bigger), then the data level.
std::vector<lz4pack::IvfcLevel> IvfcLayout(uint64_t size, uint32_t log2 = 14, int hash_levels = 5);

// The FS header of the new RomFS, from the one of the update: AES-CTR, its patch and sparse
// information gone, the levels, the master hash and the compression table set.
std::array<uint8_t, 0x200> RomfsHeader(const uint8_t* update_fsh, const std::vector<lz4pack::IvfcLevel>& levels, const uint8_t master[32],
                                       uint64_t table_offset, uint64_t table_size, uint32_t table_count);

// Where each section of the new NCA goes.
struct Placed {
    int index = 0;
    uint64_t old_start = 0, old_end = 0; // in the program NCA of the update
    uint64_t start = 0, end = 0;         // in the new one
    bool romfs = false;
};

struct Plan {
    std::vector<uint8_t> header; // 0xC00, decrypted
    std::vector<Placed> sections; // in the order they are in the NCA
    uint64_t size = 0;
};

// The new NCA from the decrypted header (0xC00) of the program NCA of the update, the index of its
// RomFS section, the size of the new RomFS section (from the start of its levels to the end of the
// table, rounded up here to 0x4000) and its FS header. False when the header is not one this
// handles. `hash` makes the SHA-256 of the FS headers kept in the NCA header.
bool PlanNca(const uint8_t* update_header, int romfs_index, uint64_t romfs_size, const std::array<uint8_t, 0x200>& romfs_fsh,
             const lz4pack::HashFn& hash, Plan& out);

} // namespace nxdock::mergenca
