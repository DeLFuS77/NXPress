// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 DeLFuS77
#include "merge_game.hpp"

#include <algorithm>
#include <cstring>

namespace nxdock::mergegame {

namespace {

constexpr size_t kChunk = 4 * 1024 * 1024;

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

// The counter of AES-CTR at a place of an NCA (a multiple of 16), for a section of this FS header.
void CounterAt(const uint8_t* fsh, uint64_t at, uint8_t ctr[16]) {
    for (int i = 0; i < 8; i++) ctr[i] = fsh[0x147 - i];
    const uint64_t block = at >> 4;
    for (int i = 0; i < 8; i++) ctr[8 + i] = uint8_t(block >> (8 * (7 - i)));
}

// Bytes given in order, encrypted with AES-CTR for their places from `at` (a multiple of 16), and
// passed on in pieces of whole blocks.
class CtrOut {
public:
    CtrOut(const OutFn& out, const patchview::CtrFn& ctr, const uint8_t* key, const uint8_t* fsh, uint64_t at)
        : m_out(out), m_ctr(ctr), m_key(key), m_fsh(fsh), m_at(at) {
        m_buf.reserve(kChunk + 0x10000);
    }
    bool Put(const uint8_t* d, size_t n) {
        m_buf.insert(m_buf.end(), d, d + n);
        return m_buf.size() < kChunk || Flush(false);
    }
    bool Zeros(uint64_t n) {
        static const uint8_t zeros[0x1000] = {};
        while (n) {
            const size_t k = size_t(std::min<uint64_t>(n, sizeof(zeros)));
            if (!Put(zeros, k)) return false;
            n -= k;
        }
        return true;
    }
    bool Flush(bool all) {
        const size_t n = all ? m_buf.size() : m_buf.size() & ~size_t(15);
        if (!n) return true;
        uint8_t c[16];
        CounterAt(m_fsh, m_at, c);
        m_ctr(m_key, c, m_buf.data(), n);
        if (!m_out(m_buf.data(), n)) return false;
        m_buf.erase(m_buf.begin(), m_buf.begin() + std::ptrdiff_t(n));
        m_at += n;
        return true;
    }

private:
    const OutFn& m_out;
    const patchview::CtrFn& m_ctr;
    const uint8_t* m_key;
    const uint8_t* m_fsh;
    uint64_t m_at;
    std::vector<uint8_t> m_buf;
};

bool Zeros(const OutFn& out, uint64_t n) {
    static const uint8_t zeros[0x1000] = {};
    while (n) {
        const size_t k = size_t(std::min<uint64_t>(n, sizeof(zeros)));
        if (!out(zeros, k)) return false;
        n -= k;
    }
    return true;
}

} // namespace

bool Merger::Open(Input in, std::string& error) {
    m_in = std::move(in);
    m_measured = false;
    if (m_in.update_header.size() != 0xC00 || m_in.update_index < 0 || m_in.update_index > 3) {
        error = "not a program NCA";
        return false;
    }
    m_base.reset();
    m_base_ptr = m_in.base_section;
    if (!m_base_ptr) {
        m_base = patchview::OpenBase(m_in.base, error);
        if (!m_base) return false;
        m_base_ptr = m_base.get();
    }
    m_patched = patchview::OpenPatched(m_in.update, m_base_ptr, error);
    if (!m_patched) return false;
    if (!m_romfs.Open(m_patched.get(), m_in.update.fsh.data(), m_in.hash, error)) return false;
    {
        const uint8_t* h = m_in.update.fsh.data() + 8;
        const uint32_t layers = Get32(h + 0xC);
        m_data_offset = layers >= 2 && layers <= 7 ? Get64(h + 0x10 + (layers - 2) * 0x18) : 0;
    }
    // the runs of the update, when its data level is not compressed (then the runs are places of
    // the RomFS itself): what it reads more than once from the base is stored once
    m_runs.clear();
    if (!m_romfs.Compressed()) {
        std::vector<patchview::IndirectEntry> ie;
        uint64_t end = 0;
        if (!patchview::IndirectEntries(*m_patched, ie, end)) {
            error = "no indirect table";
            return false;
        }
        std::vector<mergepack::IndirectRun> ir;
        ir.reserve(ie.size());
        for (const auto& e : ie) ir.push_back({e.virt, e.phys, e.storage});
        const uint8_t* h = m_in.update.fsh.data() + 8;
        const uint32_t layers = Get32(h + 0xC);
        if (layers < 2 || layers > 7) {
            error = "not an IVFC";
            return false;
        }
        const uint64_t data_offset = Get64(h + 0x10 + (layers - 2) * 0x18);
        m_runs = mergepack::RunsFromIndirect(ir, end, data_offset, m_romfs.StoredSize());
    }
    return true;
}

bool Merger::Indirect(std::vector<patchview::IndirectEntry>& entries, uint64_t& end) const {
    return m_patched && patchview::IndirectEntries(*m_patched, entries, end);
}

bool Merger::PackOnce(const lz4pack::WriteFn& write, const lz4pack::StepFn& step, lz4pack::Packed& out) {
    const mergepack::OldBlockFn old = m_romfs.Compressed()
        ? mergepack::OldBlockFn([this](uint64_t v, uint32_t n, std::vector<uint8_t>& b) { return m_romfs.StoredBlock(v, n, b); })
        : mergepack::OldBlockFn();
    return mergepack::Pack(m_romfs.Size(), m_runs, m_in.level, m_in.min_gain,
                           [this](uint64_t o, uint8_t* b, size_t n) { return m_romfs.Read(o, b, n); }, write, step, m_in.hash, old, out,
                           m_stats, m_in.threads);
}

bool Merger::Measure(const lz4pack::StepFn& step, std::string& error) {
    m_measured = false;
    lz4pack::BlockHasher hasher(1u << 14, m_in.hash);
    if (!PackOnce([&](uint64_t, const uint8_t* d, size_t n) { hasher.Feed(d, n); return true; }, step, m_packed)) {
        error = m_romfs.Damaged() ? "damaged" : "stopped";
        return false;
    }
    m_table = lz4pack::BuildTable(m_packed.entries, m_romfs.Size());
    hasher.Feed(m_table.data(), m_table.size());
    m_data_hashes = hasher.Finish();
    m_stored = m_packed.table_offset + m_table.size();
    m_levels = mergenca::IvfcLayout(std::max(m_romfs.Size(), m_stored));
    m_levels.back().size = m_stored;
    uint8_t master[32];
    if (!lz4pack::BuildIvfc(m_data_hashes, m_levels, m_in.hash, m_level_bytes, master)) {
        error = "IVFC";
        return false;
    }
    const auto fsh = mergenca::RomfsHeader(m_in.update.fsh.data(), m_levels, master, m_packed.table_offset, m_table.size(),
                                           uint32_t(m_packed.entries.size()));
    if (!mergenca::PlanNca(m_in.update_header.data(), m_in.update_index, m_levels.back().offset + m_stored, fsh, m_in.hash, m_plan)) {
        error = "NCA header";
        return false;
    }
    m_measured = true;
    return true;
}

bool Merger::Write(const OutFn& out, const lz4pack::StepFn& step, std::string& error) {
    if (!m_measured) {
        error = "not measured";
        return false;
    }
    std::vector<uint8_t> head = m_plan.header;
    m_in.encrypt_header(head.data(), head.size());
    if (!out(head.data(), head.size())) {
        error = "write";
        return false;
    }
    uint64_t at = head.size();
    std::vector<uint8_t> buf;
    for (const auto& s : m_plan.sections) {
        if (s.start < at || !Zeros(out, s.start - at)) {
            error = "write";
            return false;
        }
        at = s.start;
        const uint8_t* fsh = m_plan.header.data() + 0x400 + s.index * 0x200;
        if (!s.romfs) {
            // the ExeFS, the logo: as they are; moved, decrypted at their old place and encrypted
            // again at the new one
            const bool moved = s.start != s.old_start;
            const uint8_t enc = fsh[4];
            if (moved && enc != 1 && enc != 3) {
                error = "a section that cannot move";
                return false;
            }
            buf.resize(kChunk);
            for (uint64_t o = s.old_start; o < s.old_end;) {
                const size_t n = size_t(std::min<uint64_t>(buf.size(), s.old_end - o));
                if (!m_in.update_raw(o, buf.data(), n)) {
                    error = "read";
                    return false;
                }
                if (moved && enc == 3) {
                    uint8_t c[16];
                    CounterAt(fsh, o, c);
                    m_in.update.ctr(m_in.update.key.data(), c, buf.data(), n);
                    CounterAt(fsh, o - s.old_start + s.start, c);
                    m_in.update.ctr(m_in.update.key.data(), c, buf.data(), n);
                }
                if (!out(buf.data(), n)) {
                    error = "write";
                    return false;
                }
                o += n;
            }
            at = s.end;
            continue;
        }
        // the RomFS: its hash levels, the data level packed again, its table, zeros to its end
        CtrOut enc(out, m_in.update.ctr, m_in.update.key.data(), fsh, s.start);
        const uint64_t data_at = m_levels.back().offset;
        std::vector<uint8_t> levels(size_t(data_at), 0);
        for (size_t l = 0; l < m_level_bytes.size(); l++) {
            std::memcpy(levels.data() + m_levels[l].offset, m_level_bytes[l].data(), m_level_bytes[l].size());
        }
        if (!enc.Put(levels.data(), levels.size())) {
            error = "write";
            return false;
        }
        lz4pack::BlockHasher check(1u << 14, m_in.hash);
        lz4pack::Packed again;
        bool wrote = true;
        const bool ok = PackOnce([&](uint64_t, const uint8_t* d, size_t n) {
            check.Feed(d, n);
            wrote = enc.Put(d, n);
            return wrote;
        }, step, again);
        if (!ok || !wrote) {
            error = !wrote ? "write" : m_romfs.Damaged() ? "damaged" : "stopped";
            return false;
        }
        check.Feed(m_table.data(), m_table.size());
        // what was written is what the hashes were made of
        if (again.table_offset != m_packed.table_offset || again.entries.size() != m_packed.entries.size() || check.Finish() != m_data_hashes) {
            error = "not the same the second time";
            return false;
        }
        if (!enc.Put(m_table.data(), m_table.size()) || !enc.Zeros(s.end - s.start - data_at - m_stored) || !enc.Flush(true)) {
            error = "write";
            return false;
        }
        at = s.end;
    }
    if (at > m_plan.size || !Zeros(out, m_plan.size - at)) {
        error = "write";
        return false;
    }
    return true;
}

} // namespace nxdock::mergegame
