// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 DeLFuS77
#include "merge_pack.hpp"

#include "lz4nintendo.h"
#include "work_thread.hpp"

#include <algorithm>
#include <cstring>
#include <deque>
#include <functional>
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

// One thing to do, in order: a new run, cut into blocks as it is read, or a run the same as an
// earlier one.
struct Item {
    uint64_t virt = 0;
    int64_t copy_of = -1; // a run the same as the earlier run at copy_of; -1: a new run
    uint64_t size = 0;
    uint64_t base = UINT64_MAX; // a new run read from the base game: from where
};

// The blocks of a new run, planned as it is read. Where the run holds a whole block known before,
// at any byte and not only where blocks are cut, that block is cut out to point at the one
// stored, and the bytes before it are a block of their own; past it, blocks are cut as before
// (from the base, up to its next 64 KiB again).
//
// Spots that come with the content find those places wherever they are: a byte where a gear hash
// of the 64 bytes up to it has its top 12 bits clear, one in 4096 on average (about 16 in a
// block). A whole block stored is known by its first spot (its anchor), 8 words from across it
// and its hash. A later spot with the hash of an anchor is where that block may be held again,
// which the words and then the hash of all of it there have to bear out. Only what comes after a
// block can point at it, so it is all done in the one read, about two blocks ahead of the cuts.
class Planner {
public:
    using SeenFn = std::function<bool(const std::string& key)>;

    Planner(const lz4pack::ReadFn& read, const lz4pack::HashFn& hash) : m_read(read), m_hash(hash) {
        uint64_t s = 0x6A09E667F3BCC908ull;
        for (auto& g : m_gear) {
            s += 0x9E3779B97F4A7C15ull;
            uint64_t z = s;
            z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9ull;
            z = (z ^ (z >> 27)) * 0x94D049BB133111EBull;
            g = z ^ (z >> 31);
        }
    }

    void Start(uint64_t virt, uint64_t size, uint64_t base) {
        m_virt = m_pos = m_scan = virt;
        m_end = virt + size;
        m_base = base;
        m_h = 0;
        m_spots.clear();
        m_last_spot = UINT64_MAX;
        m_pending = UINT64_MAX;
        if (m_buf_virt > virt || m_buf_virt + m_buf.size() < virt) {
            m_buf.clear();
            m_buf_virt = virt;
        }
    }
    bool Done() const { return m_pos >= m_end; }

    // The next block of the run: its place and size, its bytes in plain and their key (hash and
    // size); moved when it is a block known before found away from where blocks are cut. A whole
    // block new to seen is known from here on.
    bool Next(uint64_t& virt, uint32_t& n, uint8_t* plain, std::string& key, bool& moved, const SeenFn& seen) {
        const uint64_t v = m_pos;
        uint64_t want = kBlock;
        if (v == 0) want = kHead;
        else if (m_base != UINT64_MAX) want = kBlock - (m_base + (v - m_virt)) % kBlock;
        const uint32_t w = uint32_t(std::min<uint64_t>(want, m_end - v));
        n = w;
        moved = false;
        if (m_pending == v) {
            n = kBlock;
            key = m_pending_key;
            moved = true;
            m_pending = UINT64_MAX;
        } else if (v != 0 && m_end - v >= kBlock) {
            const uint64_t limit = std::min<uint64_t>(m_end, v + w + kBlock);
            if (!Fill(limit)) return false;
            Scan(limit);
            while (!m_spots.empty() && m_spots.front().pos < v + kWindow - 1) m_spots.pop_front();
            uint64_t best = UINT64_MAX;
            size_t best_known = 0;
            for (auto& spot : m_spots) {
                if (spot.pos >= limit) break;
                const auto it = m_by_spot.find(spot.h);
                if (it == m_by_spot.end()) continue;
                const auto& list = it->second;
                while (spot.checked < list.size()) {
                    const Known& k = m_known[list[spot.checked]];
                    // where the block would start: before v or past the run, never again
                    if (spot.pos < v + k.anchor || spot.pos - k.anchor + kBlock > m_end) {
                        spot.checked++;
                        continue;
                    }
                    const uint64_t p = spot.pos - k.anchor;
                    if (p >= v + w) break; // for a later block
                    spot.checked++;
                    if (p < best && Words(p, k) && Key(p) == k.key) {
                        best = p;
                        best_known = list[spot.checked - 1];
                    }
                }
            }
            // right where the block is cut anyway, its hash finds it as it always did
            if (best != UINT64_MAX && !(best == v && w == kBlock)) {
                if (best == v) {
                    n = kBlock;
                    key = m_known[best_known].key;
                    moved = true;
                } else {
                    n = uint32_t(best - v);
                    m_pending = best;
                    m_pending_key = m_known[best_known].key;
                }
            }
        }
        if (!Fill(v + n)) return false;
        std::memcpy(plain, m_buf.data() + (v - m_buf_virt), n);
        if (!moved) key = Key(v, n);
        if (!moved && n == kBlock && v != 0 && m_end - v >= kBlock && !seen(key)) Learn(v, key);
        virt = v;
        m_pos = v + n;
        return true;
    }

private:
    static constexpr uint64_t kWindow = 64;  // the bytes a spot depends on
    static constexpr int kSpotShift = 52;    // a spot: the top 12 bits of the hash clear
    static constexpr size_t kPerSpot = 256;  // blocks kept for one anchor (files often start alike)
    static constexpr int kWords = 8;         // words from across a block, checked before its hash
    static constexpr uint64_t kKeep = 8u << 20;

    struct Spot {
        uint64_t pos = 0, h = 0;
        uint32_t checked = 0; // the blocks with this anchor already tried here
    };
    struct Known {
        std::string key;
        uint32_t anchor = 0;
        uint32_t words[kWords] = {};
    };

    // the run read on up to `to`, keeping what is from the block being cut on
    bool Fill(uint64_t to) {
        to = std::min(to, m_end);
        if (m_buf_virt + m_buf.size() >= to) return true;
        if (m_pos - m_buf_virt > kKeep) {
            m_buf.erase(m_buf.begin(), m_buf.begin() + ptrdiff_t(m_pos - m_buf_virt));
            m_buf_virt = m_pos;
        }
        const uint64_t from = m_buf_virt + m_buf.size();
        const uint64_t n = std::max<uint64_t>(to - from, std::min<uint64_t>(1u << 20, m_end - from));
        m_buf.resize(size_t(m_buf.size() + n));
        return m_read(from, m_buf.data() + (from - m_buf_virt), size_t(n));
    }
    // the spots up to `to`; where the data repeats every few bytes, one in a block's worth
    void Scan(uint64_t to) {
        for (; m_scan < to; m_scan++) {
            m_h = (m_h << 1) + m_gear[m_buf[size_t(m_scan - m_buf_virt)]];
            if (m_scan - m_virt + 1 < kWindow || (m_h >> kSpotShift) != 0) continue;
            if (m_last_spot != UINT64_MAX && m_h == m_last_h && m_scan - m_last_spot < kBlock) continue;
            m_spots.push_back({m_scan, m_h, 0});
            m_last_spot = m_scan;
            m_last_h = m_h;
        }
    }
    std::string Key(uint64_t at, uint32_t n = kBlock) const {
        uint8_t h[32];
        m_hash(m_buf.data() + (at - m_buf_virt), n, h);
        std::string key(reinterpret_cast<const char*>(h), 32);
        key.append(reinterpret_cast<const char*>(&n), 4);
        return key;
    }
    uint32_t Word(uint64_t at, int i) const {
        uint32_t word;
        std::memcpy(&word, m_buf.data() + (at - m_buf_virt) + i * (kBlock / kWords) + kBlock / kWords / 2, 4);
        return word;
    }
    bool Words(uint64_t at, const Known& k) const {
        for (int i = 0; i < kWords; i++) {
            if (Word(at, i) != k.words[i]) return false;
        }
        return true;
    }
    // a whole block stored at `at`: known by its first spot, when it has one
    void Learn(uint64_t at, const std::string& key) {
        for (const auto& spot : m_spots) {
            if (spot.pos < at + kWindow - 1) continue;
            if (spot.pos >= at + kBlock) return;
            auto& list = m_by_spot[spot.h];
            if (list.size() >= kPerSpot) return;
            Known k;
            k.key = key;
            k.anchor = uint32_t(spot.pos - at);
            for (int i = 0; i < kWords; i++) k.words[i] = Word(at, i);
            list.push_back(uint32_t(m_known.size()));
            m_known.push_back(std::move(k));
            return;
        }
    }

    const lz4pack::ReadFn& m_read;
    const lz4pack::HashFn& m_hash;
    uint64_t m_gear[256];
    uint64_t m_virt = 0, m_end = 0, m_pos = 0, m_base = UINT64_MAX;
    std::vector<uint8_t> m_buf;
    uint64_t m_buf_virt = 0;
    uint64_t m_scan = 0, m_h = 0, m_last_spot = UINT64_MAX, m_last_h = 0;
    std::deque<Spot> m_spots;
    std::vector<Known> m_known;
    std::unordered_map<uint64_t, std::vector<uint32_t>> m_by_spot;
    uint64_t m_pending = UINT64_MAX;
    std::string m_pending_key;
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
        items.push_back({r.virt, r.same_as, r.size, r.base});
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
        bool moved = false;
        bool skip = false; // stored before, or the head: not compressed
        std::string key;
        int64_t c = -1;
        std::vector<uint8_t> plain, packed, work;
    };
    // Two batches: the next one is read and planned while the cores compress the one before
    // (reading is mostly waiting on the storage, compressing is the cores), then that one is
    // written in order. What the cores look at is only their batch: which blocks to skip is
    // decided as it is planned.
    struct Batch {
        std::vector<Job> jobs;
        size_t count = 0;
        std::vector<std::thread> workers;
    };
    Batch sets[2];
    for (auto& b : sets) {
        b.jobs.resize(size_t(threads) * 8);
        for (auto& j : b.jobs) {
            j.plain.resize(kBlock);
            j.packed.resize(nx_lz4_bound(kBlock));
            j.work.resize(NX_LZ4_WORK_SIZE);
        }
    }
    std::vector<uint8_t> old_bytes;
    uint64_t current_run = UINT64_MAX;
    Planner plan(read, hash);
    bool in_run = false, first_block = false;
    size_t i = 0;

    // the next blocks of the new runs into b, up to a run copied or the end; `other` is the batch
    // being compressed meanwhile, whose blocks are stored before these
    const auto plan_batch = [&](Batch& b, const Batch* other) {
        b.count = 0;
        const auto known = [&](const std::string& key, size_t upto) {
            if (by_hash.count(key)) return true;
            if (other) {
                for (size_t k = 0; k < other->count; k++) {
                    if (other->jobs[k].virt != 0 && other->jobs[k].key == key) return true;
                }
            }
            for (size_t k = 0; k < upto; k++) {
                if (b.jobs[k].virt != 0 && b.jobs[k].key == key) return true;
            }
            return false;
        };
        const Planner::SeenFn seen = [&](const std::string& key) { return known(key, b.count); };
        while (b.count < b.jobs.size()) {
            if (!in_run) {
                if (i >= items.size() || items[i].copy_of >= 0) break;
                plan.Start(items[i].virt, items[i].size, items[i].base);
                i++;
                in_run = true;
                first_block = true;
            }
            Job& j = b.jobs[b.count];
            if (!plan.Next(j.virt, j.n, j.plain.data(), j.key, j.moved, seen)) return false;
            j.run_start = first_block;
            first_block = false;
            j.skip = j.virt == 0 || known(j.key, b.count);
            j.c = -1;
            b.count++;
            if (plan.Done()) in_run = false;
        }
        return true;
    };
    const auto start = [&](Batch& b) {
        const auto run = [&b, level, threads](size_t first) {
            for (size_t k = first; k < b.count; k += threads) {
                Job& j = b.jobs[k];
                if (j.skip) continue;
                j.c = nx_lz4_compress(j.plain.data(), j.n, j.packed.data(), j.packed.size(), level, j.work.data());
            }
        };
        if (threads > 1 && b.count > 1) {
            for (unsigned t = 0; t < threads && t < b.count; t++) {
                b.workers.emplace_back([run, t] {
                    nxdock::WorkThread(int(t)); // below the screen, each on its core
                    run(t);
                });
            }
        } else {
            run(0);
        }
    };
    const auto join = [](Batch& b) {
        for (auto& w : b.workers) w.join();
        b.workers.clear();
    };
    // a batch compressed, written in order
    const auto emit = [&](Batch& b) {
        for (size_t k = 0; k < b.count; k++) {
            Job& j = b.jobs[k];
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
                if (j.moved) stats.moved_again += j.n;
                raw_open = false;
            } else if (j.moved || (j.skip && j.virt != 0)) {
                return false; // a block known before is always stored before it
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
        return true;
    };
    // a run the same as an earlier one: its entries again, moved to this place
    const auto copy_run = [&](const Item& item) {
        if (current_run != UINT64_MAX) {
            run_entries[current_run].second = out.entries.size();
            current_run = UINT64_MAX;
        }
        const auto it = run_entries.find(uint64_t(item.copy_of));
        if (it == run_entries.end()) return false;
        const uint64_t shift = item.virt - uint64_t(item.copy_of);
        const size_t first = out.entries.size();
        for (size_t e = it->second.first; e < it->second.second; e++) {
            Entry copy = out.entries[e];
            copy.virt += shift;
            out.entries.push_back(copy);
        }
        run_entries[item.virt] = {first, out.entries.size()};
        stats.runs_again += item.size;
        raw_open = false;
        return !step || step(item.virt + item.size);
    };

    int cur = 0;
    bool ok = true;
    for (;;) {
        // runs copied come when nothing is planned ahead of them
        if (!in_run && sets[cur].count == 0) {
            while (i < items.size() && items[i].copy_of >= 0) {
                if (!copy_run(items[i])) return false;
                i++;
            }
            if (i >= items.size()) break;
            if (!plan_batch(sets[cur], nullptr)) return false;
            if (sets[cur].count == 0) return false;
            start(sets[cur]);
        }
        Batch& a = sets[cur];
        Batch& b = sets[cur ^ 1];
        // the next batch read while `a` compresses
        ok = plan_batch(b, &a);
        join(a);
        if (!ok || !emit(a)) {
            join(b);
            return false;
        }
        a.count = 0;
        if (b.count > 0) {
            start(b);
            cur ^= 1;
        }
    }
    if (current_run != UINT64_MAX) run_entries[current_run].second = out.entries.size();
    out.stored = at;
    out.table_offset = Align16(at);
    return pad_to(out.table_offset);
}

} // namespace nxdock::mergepack
