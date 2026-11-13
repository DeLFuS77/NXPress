// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 DeLFuS77
#include "romfs_lz4.hpp"

#include "lz4nintendo.h"

#include <algorithm>
#include <cstring>
#include <thread>

namespace nxdock::lz4pack {
namespace {

void Put32(uint8_t* p, uint32_t v) { std::memcpy(p, &v, 4); }
void Put64(uint8_t* p, uint64_t v) { std::memcpy(p, &v, 8); }
uint32_t Get32(const uint8_t* p) {
    uint32_t v;
    std::memcpy(&v, p, 4);
    return v;
}
uint64_t Get64(const uint8_t* p) {
    uint64_t v;
    std::memcpy(&v, p, 8);
    return v;
}
uint64_t Align16(uint64_t v) { return (v + 15) & ~uint64_t(15); }

} // namespace

bool Pack(uint64_t virt_size, int level, uint32_t min_gain, const ReadFn& read, const WriteFn& write, const StepFn& step, Packed& out,
          unsigned threads) {
    out = Packed{};
    out.virt_size = virt_size;
    threads = std::clamp(threads, 1u, 8u);
    // a batch of blocks: read in order, compressed side by side, written in order
    struct Job {
        uint64_t virt = 0;
        uint32_t n = 0;
        int64_t c = -1;
        std::vector<uint8_t> plain, packed, work;
    };
    // two batches: the next one is read while the one before is compressed (reading is mostly
    // waiting on the storage, compressing is the cores), then written in order
    std::vector<Job> sets[2];
    for (auto& jobs : sets) {
        jobs.resize(size_t(threads) * 8);
        for (auto& j : jobs) {
            j.plain.resize(kBlock);
            j.packed.resize(nx_lz4_bound(kBlock));
            j.work.resize(NX_LZ4_WORK_SIZE);
        }
    }
    static const uint8_t zeros[16] = {};
    uint64_t at = 0; // stored bytes written
    const auto pad_to = [&](uint64_t to) {
        while (at < to) {
            const size_t n = size_t(std::min<uint64_t>(sizeof(zeros), to - at));
            if (!write(at, zeros, n)) return false;
            at += n;
        }
        return true;
    };
    uint64_t virt = 0;
    // the next blocks, in order: the head stays as it is, then blocks of 64 KiB
    const auto fill = [&](std::vector<Job>& jobs, size_t& count) {
        count = 0;
        while (count < jobs.size() && virt < virt_size) {
            Job& j = jobs[count++];
            j.virt = virt;
            j.n = uint32_t(std::min<uint64_t>(virt == 0 ? kHead : kBlock, virt_size - virt));
            if (!read(virt, j.plain.data(), j.n)) return false;
            virt += j.n;
        }
        return true;
    };
    size_t counts[2] = {0, 0};
    int cur = 0;
    if (!fill(sets[0], counts[0])) return false;
    while (counts[cur] > 0) {
        std::vector<Job>& jobs = sets[cur];
        const size_t count = counts[cur];
        const auto run = [&jobs, count, threads, level](size_t first) {
            for (size_t i = first; i < count; i += threads) {
                Job& j = jobs[i];
                j.c = j.virt == 0 ? -1 : nx_lz4_compress(j.plain.data(), j.n, j.packed.data(), j.packed.size(), level, j.work.data());
            }
        };
        bool read_ok;
        if (threads > 1 && count > 1) {
            std::vector<std::thread> workers;
            for (unsigned t = 0; t < threads && t < count; t++) workers.emplace_back(run, t);
            read_ok = fill(sets[cur ^ 1], counts[cur ^ 1]);
            for (auto& w : workers) w.join();
        } else {
            run(0);
            read_ok = fill(sets[cur ^ 1], counts[cur ^ 1]);
        }
        if (!read_ok) return false;
        for (size_t i = 0; i < count; i++) {
            const Job& j = jobs[i];
            const bool lz4 = j.c > 0 && uint64_t(j.c) + min_gain <= j.n;
            if (lz4) {
                if (!pad_to(Align16(at))) return false;
                out.entries.push_back({j.virt, at, kLz4, int8_t(level), uint32_t(j.c)});
                if (!write(at, j.packed.data(), size_t(j.c))) return false;
                at += uint64_t(j.c);
            } else {
                // as it is: joined to a stored entry right before it
                const bool join = !out.entries.empty() && out.entries.back().type == kNone && j.virt != kHead;
                if (!join) {
                    if (!pad_to(Align16(at))) return false;
                    out.entries.push_back({j.virt, at, kNone, 0, kNoneSize});
                }
                if (!write(at, j.plain.data(), j.n)) return false;
                at += j.n;
            }
            if (step && !step(j.virt + j.n)) return false;
        }
        cur ^= 1;
    }
    out.stored = at;
    out.table_offset = Align16(at);
    return pad_to(out.table_offset);
}

std::vector<uint8_t> BuildTable(const std::vector<Entry>& entries, uint64_t virt_end) {
    const size_t sets = (entries.size() + kPerSet - 1) / kPerSet;
    std::vector<uint8_t> t((1 + sets) * kNode, 0);
    // the first node: how many sets, where the tree ends, and where each set starts
    Put32(t.data() + 0, 0);
    Put32(t.data() + 4, uint32_t(sets));
    Put64(t.data() + 8, virt_end);
    for (size_t s = 0; s < sets; s++) Put64(t.data() + 0x10 + s * 8, entries[s * kPerSet].virt);
    for (size_t s = 0; s < sets; s++) {
        uint8_t* node = t.data() + (1 + s) * kNode;
        const size_t first = s * kPerSet, count = std::min<size_t>(kPerSet, entries.size() - first);
        const uint64_t end = first + count < entries.size() ? entries[first + count].virt : virt_end;
        Put32(node + 0, uint32_t(s));
        Put32(node + 4, uint32_t(count));
        Put64(node + 8, end);
        for (size_t k = 0; k < count; k++) {
            const Entry& e = entries[first + k];
            uint8_t* p = node + 0x10 + k * 0x18;
            Put64(p, e.virt);
            Put64(p + 8, e.phys);
            p[16] = e.type;
            p[17] = uint8_t(e.level);
            Put32(p + 20, e.size);
        }
    }
    return t;
}

bool ParseTable(const uint8_t* table, size_t size, uint32_t count, std::vector<Entry>& out, uint64_t& virt_end) {
    out.clear();
    if (count == 0 || size < kNode) return false;
    const size_t sets = (count + kPerSet - 1) / kPerSet;
    if (size / kNode < 1 + sets || Get32(table + 4) != sets) return false;
    virt_end = Get64(table + 8);
    for (size_t s = 0; s < sets; s++) {
        const uint8_t* node = table + (1 + s) * kNode;
        const uint32_t n = Get32(node + 4);
        if (Get32(node) != s || n > kPerSet) return false;
        for (uint32_t k = 0; k < n; k++) {
            const uint8_t* p = node + 0x10 + k * 0x18;
            Entry e;
            e.virt = Get64(p);
            e.phys = Get64(p + 8);
            e.type = p[16];
            e.level = int8_t(p[17]);
            e.size = Get32(p + 20);
            if (k == 0 && Get64(table + 0x10 + s * 8) != e.virt) return false;
            // the entries never go down, so each one ends where the next starts
            if (!out.empty() && e.virt < out.back().virt) return false;
            out.push_back(e);
        }
    }
    if (out.size() != count || virt_end < out.back().virt) return false;
    // an LZ4 block is one block of the data at most, so Unpack never takes more memory
    for (size_t i = 0; i < out.size(); i++) {
        if (out[i].type != kLz4) continue;
        const uint64_t end = i + 1 < out.size() ? out[i + 1].virt : virt_end;
        if (end - out[i].virt > kBlock || out[i].size > nx_lz4_bound(kBlock)) return false;
    }
    return true;
}

bool Unpack(const std::vector<Entry>& entries, uint64_t virt_end, const ReadFn& stored, uint64_t offset, uint8_t* out, size_t size) {
    std::vector<uint8_t> packed, plain;
    while (size > 0) {
        // the entry holding offset
        auto it = std::upper_bound(entries.begin(), entries.end(), offset, [](uint64_t o, const Entry& e) { return o < e.virt; });
        if (it == entries.begin()) return false;
        const Entry& e = *(it - 1);
        const uint64_t end = it == entries.end() ? virt_end : it->virt;
        if (offset >= end) return false;
        const size_t n = size_t(std::min<uint64_t>(size, end - offset));
        if (e.type == kNone) {
            if (!stored(e.phys + (offset - e.virt), out, n)) return false;
        } else if (e.type == kZeros) {
            std::memset(out, 0, n);
        } else if (e.type == kLz4) {
            packed.resize(e.size);
            plain.resize(size_t(end - e.virt));
            if (!stored(e.phys, packed.data(), packed.size()) || nx_lz4_decompress(packed.data(), packed.size(), plain.data(), plain.size()) != 0) return false;
            std::memcpy(out, plain.data() + (offset - e.virt), n);
        } else {
            return false;
        }
        out += n;
        offset += n;
        size -= n;
    }
    return true;
}

double EstimateGain(uint64_t virt_size, int level, uint32_t every, const ReadFn& read) {
    std::vector<uint8_t> plain(kBlock), packed(nx_lz4_bound(kBlock)), work(NX_LZ4_WORK_SIZE);
    uint64_t in = 0, kept = 0;
    const uint64_t blocks = (virt_size + kBlock - 1) / kBlock;
    for (uint64_t b = 0; b < blocks; b += std::max<uint32_t>(1, every)) {
        const uint64_t virt = b * kBlock;
        const uint32_t n = uint32_t(std::min<uint64_t>(kBlock, virt_size - virt));
        if (!read(virt, plain.data(), n)) return 0;
        const int64_t c = nx_lz4_compress(plain.data(), n, packed.data(), packed.size(), level, work.data());
        in += n;
        kept += c > 0 && uint64_t(c) < n ? uint64_t(Align16(uint64_t(c))) : n;
    }
    return in ? 1.0 - double(kept) / double(in) : 0;
}

double EstimateRepack(const std::vector<Entry>& entries, uint64_t virt_end, int level, uint32_t every, const ReadFn& stored) {
    std::vector<uint8_t> packed, plain, again(nx_lz4_bound(kBlock)), work(NX_LZ4_WORK_SIZE);
    uint64_t before = 0, after = 0;
    uint32_t seen = 0;
    for (size_t i = 0; i < entries.size(); i++) {
        const Entry& e = entries[i];
        if (e.type != kLz4 || seen++ % std::max<uint32_t>(1, every)) continue;
        const uint64_t end = i + 1 < entries.size() ? entries[i + 1].virt : virt_end;
        if (end <= e.virt || end - e.virt > kBlock) continue;
        packed.resize(e.size);
        plain.resize(size_t(end - e.virt));
        if (!stored(e.phys, packed.data(), packed.size()) || nx_lz4_decompress(packed.data(), packed.size(), plain.data(), plain.size()) != 0) return 0;
        const int64_t c = nx_lz4_compress(plain.data(), plain.size(), again.data(), again.size(), level, work.data());
        const uint64_t was = Align16(e.size);
        before += was;
        after += c > 0 ? std::min<uint64_t>(Align16(uint64_t(c)), was) : was;
    }
    return before ? 1.0 - double(after) / double(before) : 0;
}

double ShareAtLevel(const std::vector<Entry>& entries, int level) {
    uint64_t lz4 = 0, at = 0;
    for (const auto& e : entries) {
        if (e.type != kLz4) continue;
        lz4++;
        if (e.level == level) at++;
    }
    return lz4 ? double(at) / double(lz4) : 0;
}

BlockHasher::BlockHasher(uint32_t block_size, HashFn hash) : m_size(block_size), m_hash(std::move(hash)), m_block(block_size) {}

void BlockHasher::Feed(const uint8_t* data, size_t size) {
    m_fed += size;
    while (size) {
        const size_t n = std::min(size, size_t(m_size) - m_fill);
        std::memcpy(m_block.data() + m_fill, data, n);
        m_fill += n;
        data += n;
        size -= n;
        if (m_fill == m_size) {
            uint8_t h[32];
            m_hash(m_block.data(), m_size, h);
            m_out.insert(m_out.end(), h, h + 32);
            m_fill = 0;
        }
    }
}

std::vector<uint8_t> BlockHasher::Finish() {
    if (m_fill) {
        // the last block, padded with zeros
        std::memset(m_block.data() + m_fill, 0, m_size - m_fill);
        uint8_t h[32];
        m_hash(m_block.data(), m_size, h);
        m_out.insert(m_out.end(), h, h + 32);
        m_fill = 0;
    }
    m_fed = 0;
    return std::move(m_out);
}

bool BuildIvfc(std::vector<uint8_t> data_hashes, std::vector<IvfcLevel>& levels, const HashFn& hash,
               std::vector<std::vector<uint8_t>>& out, uint8_t master[32]) {
    if (levels.size() < 2) return false;
    const size_t data = levels.size() - 1;
    out.assign(data, {});
    std::vector<uint8_t> below = std::move(data_hashes);
    for (size_t i = data; i-- > 0;) {
        // level i holds the hashes of level i + 1; its own blocks are hashed for level i - 1
        // as Nintendo's: a level keeps the size it had (made for the data decompressed), padded
        // with zeros, and is at least its hashes rounded up to a whole block
        // rounded up to a whole block; only its hashes are hashed for the level above, the zeros
        // after them are not (their entries there stay zero)
        const uint64_t block = 1ull << levels[i].log2;
        const size_t used = below.size();
        levels[i].size = std::max<uint64_t>(levels[i].size, (used + block - 1) / block * block);
        if (levels[i].offset + levels[i].size > levels[i + 1].offset) return false;
        below.resize(size_t(levels[i].size), 0);
        out[i] = below;
        BlockHasher h(1u << levels[i].log2, hash);
        h.Feed(out[i].data(), used);
        below = h.Finish();
    }
    // level 0 is one block: its hash is the master hash
    if (below.size() != 32) return false;
    std::memcpy(master, below.data(), 32);
    return true;
}

} // namespace nxdock::lz4pack
