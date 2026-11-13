// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 DeLFuS77
#pragma once

// The program NCA of a game merged with its update, made from the two as they are installed (25D):
// the RomFS of the update over its base read through patch_view, packed by merge_pack, laid out by
// merge_nca, and written out in order, encrypted, as the console writes a placeholder. No copy of
// the game is kept in memory or on the SD card:
//  1. Measure packs the RomFS once, keeping only the hashes of what it stores, from which the IVFC
//     and the header are made;
//  2. Write packs it again (the same bytes: merge_pack gives the same on any number of threads) and
//     writes the NCA from its start; what it stores must hash as the first time, or it fails.
//
// Plain C++: AES-CTR, the AES-XTS of the header and the reads are given to it, so it is tested on
// the computer, with real games.
#include "merge_nca.hpp"
#include "merge_pack.hpp"
#include "patch_view.hpp"

#include <array>
#include <cstdint>
#include <functional>
#include <memory>
#include <string>
#include <vector>

namespace nxdock::mergegame {

// The new NCA, in order from its start.
using OutFn = std::function<bool(const uint8_t* data, size_t size)>;
// The AES-XTS of an NCA header (0xC00 bytes, in place, sectors of 0x200 from 0).
using HeaderFn = std::function<void(uint8_t* header, size_t size)>;

struct Input {
    patchview::NcaSection base;          // the RomFS section of the program NCA of the base game
    patchview::Section* base_section = nullptr; // or the base as it is read (a merged game's, 25E)
    patchview::NcaSection update;        // the RomFS section of the program NCA of the update
    int update_index = 1;                // which section of it that is
    std::vector<uint8_t> update_header;  // its header, decrypted (0xC00)
    patchview::RawRead update_raw;       // the program NCA of the update, as stored
    lz4pack::HashFn hash;                // SHA-256
    HeaderFn encrypt_header;
    int level = 9;
    uint32_t min_gain = 64;
    unsigned threads = 3;
};

class Merger {
public:
    // The base game and its update opened; false, with `error`, when they are not a pair this reads
    // (the base of another version, a damaged one: found by the hashes of the update as it reads).
    bool Open(Input in, std::string& error);
    // the RomFS as the game reads it
    uint64_t RomfsSize() const { return m_romfs.Size(); }
    // the RomFS read as the game reads it (for an estimate from a sample of it)
    bool ReadRomfs(uint64_t offset, uint8_t* buf, size_t size) { return m_romfs.Read(offset, buf, size); }
    // 1. packed once, measured: after it, NcaSize is known
    bool Measure(const lz4pack::StepFn& step, std::string& error);
    uint64_t NcaSize() const { return m_plan.size; }
    // what the RomFS takes now (the data level stored with its table)
    uint64_t StoredSize() const { return m_stored; }
    // 2. the whole NCA, in order
    bool Write(const OutFn& out, const lz4pack::StepFn& step, std::string& error);
    const mergepack::Stats& Stats() const { return m_stats; }
    // for the recipe of the merged game (merge_recipe): the indirect table of the update, the data
    // level in its patched section, the size of the base section, and whether its RomFS is stored
    // as it is read (when compressed, the places of the base are not places of the RomFS)
    bool Indirect(std::vector<patchview::IndirectEntry>& entries, uint64_t& end) const;
    uint64_t DataOffset() const { return m_data_offset; }
    uint64_t DataSize() const { return m_romfs.StoredSize(); }
    uint64_t BaseSize() const { return m_base_ptr ? m_base_ptr->Size() : 0; }
    // the base as the update reads it (for the spare of the merged game, merge_recipe)
    bool ReadBase(uint64_t offset, uint8_t* buf, size_t size) { return m_base_ptr && m_base_ptr->Read(offset, buf, size); }
    bool RomfsCompressed() const { return m_romfs.Compressed(); }
    // blocks of the base or the update found damaged as they were read
    uint64_t Damaged() const { return m_romfs.Damaged(); }

private:
    bool PackOnce(const lz4pack::WriteFn& write, const lz4pack::StepFn& step, lz4pack::Packed& out);
    Input m_in;
    std::unique_ptr<patchview::Section> m_base, m_patched;
    patchview::Section* m_base_ptr = nullptr;
    uint64_t m_data_offset = 0;
    patchview::Romfs m_romfs;
    std::vector<mergepack::Run> m_runs;
    lz4pack::Packed m_packed;
    std::vector<uint8_t> m_table, m_data_hashes;
    std::vector<lz4pack::IvfcLevel> m_levels;
    std::vector<std::vector<uint8_t>> m_level_bytes;
    mergenca::Plan m_plan;
    mergepack::Stats m_stats;
    uint64_t m_stored = 0;
    bool m_measured = false;
};

} // namespace nxdock::mergegame
