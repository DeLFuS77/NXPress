// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 DeLFuS77
#include "merge_nca.hpp"

#include <algorithm>
#include <cstring>

namespace nxdock::mergenca {
namespace {

void Put32(uint8_t* p, uint32_t v) { std::memcpy(p, &v, 4); }
void Put64(uint8_t* p, uint64_t v) { std::memcpy(p, &v, 8); }
uint32_t Get32(const uint8_t* p) {
    uint32_t v;
    std::memcpy(&v, p, 4);
    return v;
}
uint64_t Up(uint64_t v, uint64_t a) { return (v + a - 1) / a * a; }

} // namespace

std::vector<lz4pack::IvfcLevel> IvfcLayout(uint64_t size, uint32_t log2, int hash_levels) {
    const uint64_t block = 1ull << log2;
    std::vector<uint64_t> sizes(static_cast<size_t>(hash_levels));
    uint64_t below = size;
    for (int i = hash_levels - 1; i >= 0; i--) {
        sizes[size_t(i)] = Up((below + block - 1) / block * 32, block);
        below = sizes[size_t(i)];
    }
    std::vector<lz4pack::IvfcLevel> lv;
    uint64_t at = 0;
    for (int i = 0; i < hash_levels; i++) {
        lv.push_back({at, sizes[size_t(i)], log2});
        at += sizes[size_t(i)];
    }
    lv.push_back({at, size, log2});
    return lv;
}

std::array<uint8_t, 0x200> RomfsHeader(const uint8_t* update_fsh, const std::vector<lz4pack::IvfcLevel>& levels, const uint8_t master[32],
                                       uint64_t table_offset, uint64_t table_size, uint32_t table_count) {
    std::array<uint8_t, 0x200> f{};
    std::memcpy(f.data(), update_fsh, 0x200);
    f[2] = 0; // RomFS
    f[3] = 3; // IVFC
    f[4] = 3; // AES-CTR
    std::memset(f.data() + 0x100, 0, 0x40); // no indirect table, no subsections
    std::memset(f.data() + 0x148, 0, 0x30); // not sparse
    std::memset(f.data() + 0x178, 0, 0x28);
    std::memset(f.data() + 0x1A0, 0, 0x30); // no meta data hash (of a sparse table)
    uint8_t* h = f.data() + 8;
    std::memset(h, 0, 0xF8);
    Put32(h, 0x43465649); // IVFC
    Put32(h + 4, 0x20000);
    Put32(h + 8, 0x20);
    Put32(h + 0xC, uint32_t(levels.size() + 1));
    for (size_t i = 0; i < levels.size() && i < 6; i++) {
        Put64(h + 0x10 + i * 0x18, levels[i].offset);
        Put64(h + 0x10 + i * 0x18 + 8, levels[i].size);
        Put32(h + 0x10 + i * 0x18 + 0x10, levels[i].log2);
    }
    std::memcpy(h + 0xC0, master, 32);
    if (table_size) {
        Put64(f.data() + 0x178, table_offset);
        Put64(f.data() + 0x180, table_size);
        Put32(f.data() + 0x188, 0x52544B42); // BKTR
        Put32(f.data() + 0x18C, 1);
        Put32(f.data() + 0x190, table_count);
    }
    return f;
}

bool PlanNca(const uint8_t* update_header, int romfs_index, uint64_t romfs_size, const std::array<uint8_t, 0x200>& romfs_fsh,
             const lz4pack::HashFn& hash, Plan& out) {
    out = Plan{};
    out.header.assign(update_header, update_header + 0xC00);
    uint8_t* hd = out.header.data();
    if (std::memcmp(hd + 0x200, "NCA3", 4) || romfs_index < 0 || romfs_index > 3) return false;
    // the sections there are, in the order they are in the NCA
    for (int i = 0; i < 4; i++) {
        const uint32_t s = Get32(hd + 0x240 + i * 0x10), e = Get32(hd + 0x240 + i * 0x10 + 4);
        if (e <= s) continue;
        out.sections.push_back({i, uint64_t(s) * 0x200, uint64_t(e) * 0x200, 0, 0, i == romfs_index});
    }
    std::sort(out.sections.begin(), out.sections.end(), [](const Placed& a, const Placed& b) { return a.old_start < b.old_start; });
    if (std::none_of(out.sections.begin(), out.sections.end(), [](const Placed& p) { return p.romfs; })) return false;
    // each where it was until the RomFS, which takes its new size; those after it move with it
    int64_t shift = 0;
    for (auto& p : out.sections) {
        p.start = uint64_t(int64_t(p.old_start) + shift);
        if (p.romfs) {
            p.end = p.start + Up(romfs_size, 0x4000);
            shift = int64_t(p.end) - int64_t(p.old_end);
        } else {
            p.end = uint64_t(int64_t(p.old_end) + shift);
        }
        Put32(hd + 0x240 + p.index * 0x10, uint32_t(p.start / 0x200));
        Put32(hd + 0x240 + p.index * 0x10 + 4, uint32_t(p.end / 0x200));
    }
    std::memcpy(hd + 0x400 + romfs_index * 0x200, romfs_fsh.data(), 0x200);
    uint8_t d[32];
    hash(romfs_fsh.data(), 0x200, d);
    std::memcpy(hd + 0x280 + romfs_index * 0x20, d, 32);
    out.size = out.sections.back().end;
    Put64(hd + 0x208, out.size);
    return true;
}

} // namespace nxdock::mergenca
