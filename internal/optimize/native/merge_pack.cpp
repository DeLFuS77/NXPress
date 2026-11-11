// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 DeLFuS77
#include "merge_pack.hpp"

#include "lz4nintendo.h"

#include <algorithm>
#include <cstring>
#include <map>
#include <string>
#include <thread>
#include <unordered_map>

namespace nxdock::mergepack {
namespace {

using lz4pack::Entry;
using lz4pack::kBlock;
using lz4pack::kHead;
using lz4pack::kLz4;
using lz4pack::kNone;
using lz4pack::kNoneSize;

uint64_t Align16(uint64_t v) { return (v + 15) & ~uint64_t(15); }

// What a block already stored is: where, and how.
struct Ref {
    uint64_t phys = 0;
    uint32_t size = kNoneSize;
    uint8_t type = kNone;
    int8_t level = 0;
};

// One thing to do, in order: a block to pack, or a run the same as an earlier one.
struct Item {
    uint64_t virt = 0;
    uint32_t n = 0;
    int64_t copy_of = -1; // a run: the earlier run it is the same as (n unused, size in copy_size)
    uint64_t copy_size = 0;
    bool run_start = false; // the first block of a run: not joined to the entry before it
};

} // namespace

std::vector<Run> RunsFromIndirect(const std::vector<IndirectRun>& entries, uint64_t end, uint64_t data_offset, uint64_t data_size) {
    struct Clip {
        uint64_t a, b;
        int32_t storage;
        uint64_t phys;
    };
    std::vector<Clip> clips;
    const uint64_t lo = data_offset, hi = data_offset + data_size;
    for (size_t i = 0; i < entries.size(); i++) {
        const uint64_t v = entries[i].virt, next = i + 1 < entries.size() ? entries[i + 1].virt : end;
        const uint64_t a = std::max(v, lo), b = std::min(next, hi);
        if (a >= b) continue;
        clips.push_back({a - lo, b - lo, entries[i].storage, entries[i].phys + (a - v)});
    }
    // runs read from the same place of the base game, of the same size, hold the same bytes
    std::map<std::pair<uint64_t, uint64_t>, int> seen;
    for (const auto& c : clips) {
        if (c.storage == 0) seen[{c.phys, c.b - c.a}]++;
    }
    std::map<std::pair<uint64_t, uint64_t>, uint64_t> first;
    std::vector<Run> runs;
    bool can_grow = false; // the last run is new and repeated nowhere: a run like it may join it
    for (const auto& c : clips) {
        const uint64_t base = c.storage == 0 ? c.phys : UINT64_MAX;
        const auto key = std::make_pair(c.phys, c.b - c.a);
        if (c.storage == 0 && seen[key] > 1) {
            // repeated: its own blocks the first time, the same entries the next times
            auto it = first.find(key);
            if (it == first.end()) {
                first[key] = c.a;
                runs.push_back({c.a, c.b - c.a, -1, base});
            } else {
                runs.push_back({c.a, c.b - c.a, int64_t(it->second), base});
            }
            can_grow = false;
            continue;
        }
        // repeated nowhere: goes on with the run before it when that one can grow (fewer
        // partial blocks): new bytes after new bytes, or the base read on from where the run
        // before it stopped (its blocks stay on the 64 KiB of the base)
        const bool joins = can_grow && runs.back().virt + runs.back().size == c.a &&
                           (base == UINT64_MAX ? runs.back().base == UINT64_MAX
                                               : runs.back().base != UINT64_MAX && runs.back().base + runs.back().size == base);
        if (joins) {
            runs.back().size += c.b - c.a;
        } else {
            runs.push_back({c.a, c.b - c.a, -1, base});
            can_grow = true;
        }
    }
    // the data level not covered by the table (it always is in games): one more run
    const uint64_t covered = runs.empty() ? 0 : runs.back().virt + runs.back().size;
    if (covered < data_size) runs.push_back({covered, data_size - covered, -1, UINT64_MAX});
    return runs;
}

bool Pack(uint64_t virt_size, const std::vector<Run>& runs_in, int level, uint32_t min_gain, const lz4pack::ReadFn& read,
          const lz4pack::WriteFn& write, const lz4pack::StepFn& step, const lz4pack::HashFn& hash, const OldBlockFn& old,
          lz4pack::Packed& out, Stats& stats, unsigned threads) {
    out = lz4pack::Packed{};
    out.virt_size = virt_size;
    stats = Stats{};
    threads = std::clamp(threads, 1u, 8u);
    std::vector<Run> runs = runs_in;
    if (runs.empty()) runs.push_back({0, virt_size, -1});

    // what to do, in order
    std::vector<Item> items;
    uint64_t at_virt = 0;
    for (const auto& r : runs) {
        if (r.virt != at_virt || r.size == 0) return false; // runs must follow each other
        if (r.same_as >= 0) {
            items.push_back({r.virt, 0, r.same_as, r.size, true});
        } else {
            uint64_t v = r.virt;
            bool first = true;
            while (v < r.virt + r.size) {
                // the head of the level apart; from the base, up to its next 64 KiB
                uint64_t want = kBlock;
                if (v == 0) want = kHead;
                else if (r.base != UINT64_MAX) want = kBlock - (r.base + (v - r.virt)) % kBlock;
                const uint32_t n = uint32_t(std::min<uint64_t>(want, r.virt + r.size - v));
                items.push_back({v, n, -1, 0, first});
                first = false;
                v += n;
            }
        }
        at_virt += r.size;
    }
    if (at_virt != virt_size) return false;

    static const uint8_t zeros[16] = {};
    uint64_t at = 0;
    const auto pad_to = [&](uint64_t to) {
        while (at < to) {
            const size_t n = size_t(std::min<uint64_t>(sizeof(zeros), to - at));
            if (!write(at, zeros, n)) return false;
            at += n;
        }
        return true;
    };
    std::unordered_map<std::string, Ref> by_hash;   // blocks stored, by the hash of their bytes
    std::map<uint64_t, std::pair<size_t, size_t>> run_entries; // a run: its first entry and past its last
    bool raw_open = false; // the last entry is raw and stored right before `at`: a raw block may join it

    struct Job {
        uint64_t virt = 0;
        uint32_t n = 0;
        bool run_start = false;
        std::string key;
        int64_t same_in_batch = -1;
        int64_t c = -1;
        std::vector<uint8_t> plain, packed, work;
    };
    std::vector<Job> jobs(size_t(threads) * 8);
    for (auto& j : jobs) {
        j.plain.resize(kBlock);
        j.packed.resize(nx_lz4_bound(kBlock));
        j.work.resize(NX_LZ4_WORK_SIZE);
    }
    std::vector<uint8_t> old_bytes;
    uint64_t current_run = UINT64_MAX;

    size_t i = 0;
    while (i < items.size()) {
        if (items[i].copy_of >= 0) {
            // a run the same as an earlier one: its entries again, moved to this place
            if (current_run != UINT64_MAX) {
                run_entries[current_run].second = out.entries.size();
                current_run = UINT64_MAX;
            }
            const auto it = run_entries.find(uint64_t(items[i].copy_of));
            if (it == run_entries.end()) return false;
            const uint64_t shift = items[i].virt - uint64_t(items[i].copy_of);
            const size_t first = out.entries.size();
            for (size_t e = it->second.first; e < it->second.second; e++) {
                Entry copy = out.entries[e];
                copy.virt += shift;
                out.entries.push_back(copy);
            }
            run_entries[items[i].virt] = {first, out.entries.size()};
            stats.runs_again += items[i].copy_size;
            raw_open = false;
            if (step && !step(items[i].virt + items[i].copy_size)) return false;
            i++;
            continue;
        }
        // a batch of blocks, up to the next run copied
        size_t count = 0;
        while (count < jobs.size() && i + count < items.size() && items[i + count].copy_of < 0) {
            const Item& it = items[i + count];
            Job& j = jobs[count];
            j.virt = it.virt;
            j.n = it.n;
            j.run_start = it.run_start;
            j.same_in_batch = -1;
            j.c = -1;
            if (!read(it.virt, j.plain.data(), it.n)) return false;
            uint8_t h[32];
            hash(j.plain.data(), it.n, h);
            j.key.assign(reinterpret_cast<const char*>(h), 32);
            j.key.append(reinterpret_cast<const char*>(&it.n), 4);
            for (size_t k = 0; k < count; k++) {
                if (jobs[k].key == j.key) {
                    j.same_in_batch = int64_t(k);
                    break;
                }
            }
            count++;
        }
        // compressed side by side: the blocks not stored before
        const auto run = [&](size_t first_job) {
            for (size_t k = first_job; k < count; k += threads) {
                Job& j = jobs[k];
                if (j.virt == 0 || j.same_in_batch >= 0 || by_hash.count(j.key)) continue;
                j.c = nx_lz4_compress(j.plain.data(), j.n, j.packed.data(), j.packed.size(), level, j.work.data());
            }
        };
        if (threads > 1 && count > 1) {
            std::vector<std::thread> helpers;
            for (unsigned t = 1; t < threads && t < count; t++) helpers.emplace_back(run, t);
            run(0);
            for (auto& h : helpers) h.join();
        } else {
            run(0);
        }
        for (size_t k = 0; k < count; k++) {
            Job& j = jobs[k];
            // the run this block starts
            if (j.run_start) {
                if (current_run != UINT64_MAX) run_entries[current_run].second = out.entries.size();
                current_run = j.virt;
                run_entries[current_run] = {out.entries.size(), out.entries.size()};
                raw_open = false;
            }
            const auto found = by_hash.find(j.key);
            if (j.virt != 0 && found != by_hash.end()) {
                // stored before: pointed at
                out.entries.push_back({j.virt, found->second.phys, found->second.type, found->second.level, found->second.size});
                stats.blocks_again += j.n;
                raw_open = false;
            } else {
                const uint8_t* bytes = j.packed.data();
                int64_t c = j.c;
                int8_t lvl = int8_t(level);
                if (j.virt != 0 && old && old(j.virt, j.n, old_bytes) && !old_bytes.empty() &&
                    (c <= 0 || Align16(old_bytes.size()) < Align16(uint64_t(c)))) {
                    bytes = old_bytes.data();
                    c = int64_t(old_bytes.size());
                    lvl = 0;
                    stats.old_kept++;
                }
                Ref ref;
                if (j.virt != 0 && c > 0 && uint64_t(c) + min_gain <= j.n) {
                    if (!pad_to(Align16(at))) return false;
                    ref = {at, uint32_t(c), kLz4, lvl};
                    out.entries.push_back({j.virt, at, kLz4, lvl, uint32_t(c)});
                    if (!write(at, bytes, size_t(c))) return false;
                    at += uint64_t(c);
                    raw_open = false;
                } else {
                    // as it is: joined to the raw entry right before it, in the same run
                    if (!(raw_open && !j.run_start && j.virt != kHead)) {
                        if (!pad_to(Align16(at))) return false;
                        out.entries.push_back({j.virt, at, kNone, 0, kNoneSize});
                    }
                    ref = {at, kNoneSize, kNone, 0};
                    if (!write(at, j.plain.data(), j.n)) return false;
                    at += j.n;
                    raw_open = j.virt != 0;
                }
                if (j.virt != 0) by_hash.emplace(j.key, ref);
            }
            if (step && !step(j.virt + j.n)) return false;
        }
        i += count;
    }
    if (current_run != UINT64_MAX) run_entries[current_run].second = out.entries.size();
    out.stored = at;
    out.table_offset = Align16(at);
    return pad_to(out.table_offset);
}

} // namespace nxdock::mergepack
