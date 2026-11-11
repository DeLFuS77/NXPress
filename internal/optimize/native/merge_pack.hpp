// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 DeLFuS77
#pragma once

// The data level of a game merged with its update, packed as small as the LZ4 layer of the NCA
// lets it be (25C, "Merge and compress"):
//  - what the update reads more than once from its base game is stored once: the blocks are cut
//    from the start of each run of the update (its indirect table), so a run that holds the same
//    bytes as an earlier one is made of the same blocks, and its entries point at them;
//  - any block of 64 KiB the same as one stored before it is stored once too (found by its hash);
//  - each block is the smallest of Nintendo's own LZ4 of it (when the game was compressed by
//    Nintendo and the block is one of its blocks) and the LZ4 HC made here.
// Entries of the table may point back at blocks stored before them; the FS reads that (it reads
// each entry where it says), and a console played a game laid out like that (hacBrewPackLZ 1.5).
//
// Plain C++: tested on the computer, and against real games with patch_view.
#include "romfs_lz4.hpp"

#include <cstdint>
#include <functional>
#include <vector>

namespace nxdock::mergepack {

// A run of the data level: its bytes are the same as those of the earlier run at `same_as`
// (of the same size), or new (-1). A run read from the base game says from where (`base`): its
// blocks are cut on the 64 KiB of the base, so runs that read parts of the same place of the base
// (not the same start or size) are made of the same blocks inside, and those are stored once.
struct Run {
    uint64_t virt = 0, size = 0;
    int64_t same_as = -1;
    uint64_t base = UINT64_MAX;
};

// Nintendo's LZ4 of the block [virt, virt + size) when there is one exactly there: its stored
// bytes. False when there is none.
using OldBlockFn = std::function<bool(uint64_t virt, uint32_t size, std::vector<uint8_t>& stored)>;

struct Stats {
    uint64_t runs_again = 0;   // bytes stored once for runs that hold the same as another
    uint64_t blocks_again = 0; // bytes stored once for blocks the same as another
    uint64_t old_kept = 0;     // blocks where Nintendo's LZ4 was the smaller
};

// Packs a data level of virt_size bytes cut into `runs` (in order, covering it all; empty: one
// run). The stored form is written in order from 0, as lz4pack::Pack writes it, and the same
// bytes come out with any number of threads.
bool Pack(uint64_t virt_size, const std::vector<Run>& runs, int level, uint32_t min_gain, const lz4pack::ReadFn& read,
          const lz4pack::WriteFn& write, const lz4pack::StepFn& step, const lz4pack::HashFn& hash, const OldBlockFn& old,
          lz4pack::Packed& out, Stats& stats, unsigned threads = 1);

// The runs of a data level from the indirect table of an update: entries (virt, phys, storage)
// in the patched section, the data level at data_offset (size data_size). Runs from the same
// place of the base game, and of the same size, are marked as the same.
struct IndirectRun {
    uint64_t virt = 0, phys = 0;
    int32_t storage = 0;
};
std::vector<Run> RunsFromIndirect(const std::vector<IndirectRun>& entries, uint64_t end, uint64_t data_offset, uint64_t data_size);

} // namespace nxdock::mergepack
