// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 DeLFuS77
#include "patch_view.hpp"

#include <algorithm>
#include <cstring>

namespace nxdock::patchview {
namespace {

constexpr uint32_t kNode = 0x4000;
constexpr uint32_t kBktr = 0x52544B42; // "BKTR"

uint16_t Get16(const uint8_t* p) {
    uint16_t v;
    std::memcpy(&v, p, 2);
    return v;
}
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
bool Zero(const uint8_t* p, size_t n) {
    for (size_t i = 0; i < n; i++) {
        if (p[i]) return false;
    }
    return true;
}

// The counter of a section: its upper half from the FS header (big endian), the lower half the
// place in the NCA in blocks of 16 bytes.
void Counter(const uint8_t* fsh, uint64_t at, uint8_t ctr[16]) {
    for (int i = 0; i < 8; i++) ctr[i] = fsh[0x147 - i];
    const uint64_t block = at >> 4;
    for (int i = 0; i < 8; i++) ctr[8 + i] = uint8_t(block >> (8 * (7 - i)));
}
void SetGeneration(uint8_t ctr[16], uint32_t generation) {
    for (int i = 0; i < 4; i++) ctr[4 + i] = uint8_t(generation >> (8 * (3 - i)));
}

// The entries of a bucket tree of entry_size bytes each: a first node with how many sets and where
// the tree ends, then a node per set (the trees of games have one level of sets).
bool BucketEntries(const std::vector<uint8_t>& table, size_t entry_size, uint32_t count, std::vector<const uint8_t*>& out, uint64_t& end) {
    out.clear();
    const size_t per_set = (kNode - 0x10) / entry_size;
    if (count == 0 || table.size() < kNode) return false;
    const size_t sets = (count + per_set - 1) / per_set;
    if (sets > (kNode - 0x10) / 8 || table.size() / kNode < 1 + sets || Get32(table.data() + 4) != sets) return false;
    end = Get64(table.data() + 8);
    for (size_t s = 0; s < sets; s++) {
        const uint8_t* node = table.data() + (1 + s) * kNode;
        const uint32_t n = Get32(node + 4);
        if (n == 0 || n > per_set) return false;
        for (uint32_t k = 0; k < n; k++) out.push_back(node + 0x10 + k * entry_size);
    }
    return out.size() == count;
}

// AES-CTR (or not encrypted): read at its place.
class Plain : public Section {
public:
    explicit Plain(const NcaSection& s) : m_s(s) {}
    bool Read(uint64_t offset, uint8_t* buf, size_t size) override {
        if (offset > m_s.size || size > m_s.size - offset) return false;
        const uint8_t enc = m_s.fsh[4];
        if (enc == 1) return m_s.raw(m_s.start + offset, buf, size);
        const uint64_t at = m_s.start + offset, from = at & ~uint64_t(15);
        m_work.resize(size_t(at - from) + size);
        if (!m_s.raw(from, m_work.data(), m_work.size())) return false;
        uint8_t ctr[16];
        Counter(m_s.fsh.data(), from, ctr);
        m_s.ctr(m_s.key.data(), ctr, m_work.data(), m_work.size());
        std::memcpy(buf, m_work.data() + (at - from), size);
        return true;
    }
    uint64_t Size() const override { return m_s.size; }

private:
    NcaSection m_s;
    std::vector<uint8_t> m_work;
};

// A base game cut down to what its update does not replace: the pieces it holds, one after the
// other from its physical offset, each still encrypted at its place in the whole section, and
// zeros for the rest.
class Sparse : public Section {
public:
    bool Open(const NcaSection& s, std::string& error) {
        m_s = s;
        const uint8_t* f = s.fsh.data();
        const uint64_t table_offset = Get64(f + 0x148), table_size = Get64(f + 0x150);
        const uint32_t count = Get32(f + 0x160);
        m_physical = Get64(f + 0x168);
        const uint16_t generation = Get16(f + 0x170);
        m_size = s.size;
        if (count == 0 || table_size == 0) return true; // nothing stored: all of it is in the update
        if (Get32(f + 0x158) != kBktr || table_size > 64 * 1024 * 1024) {
            error = "sparse table";
            return false;
        }
        std::vector<uint8_t> table(static_cast<size_t>(table_size));
        const uint64_t at = m_physical + table_offset;
        const uint64_t from = at & ~uint64_t(15);
        std::vector<uint8_t> work(size_t(at - from) + table.size());
        if (!s.raw(from, work.data(), work.size())) {
            error = "sparse table read";
            return false;
        }
        uint8_t ctr[16];
        Counter(f, from, ctr);
        SetGeneration(ctr, uint32_t(generation) << 16);
        s.ctr(s.key.data(), ctr, work.data(), work.size());
        std::memcpy(table.data(), work.data() + (at - from), table.size());
        std::vector<const uint8_t*> raw;
        uint64_t end = 0;
        if (!BucketEntries(table, 0x14, count, raw, end)) {
            error = "sparse table entries";
            return false;
        }
        for (size_t i = 0; i < raw.size(); i++) {
            Piece p{Get64(raw[i]), Get64(raw[i] + 8), i + 1 < raw.size() ? Get64(raw[i + 1]) : end, Get32(raw[i] + 16)};
            if (p.storage > 1 || p.end <= p.virt) {
                error = "sparse entry";
                return false;
            }
            m_pieces.push_back(p);
        }
        return true;
    }
    bool Read(uint64_t offset, uint8_t* buf, size_t size) override {
        if (offset > m_size || size > m_size - offset) return false;
        std::memset(buf, 0, size);
        const uint64_t end = offset + size;
        auto it = std::upper_bound(m_pieces.begin(), m_pieces.end(), offset, [](uint64_t o, const Piece& p) { return o < p.virt; });
        if (it != m_pieces.begin()) --it;
        for (; it != m_pieces.end() && it->virt < end; ++it) {
            if (it->storage != 0) continue;
            const uint64_t a = std::max(offset, it->virt), b = std::min(end, it->end);
            if (a >= b) continue;
            // the counter of the place in the whole section, the bytes where the piece is stored
            const uint64_t place = m_s.start + a, skip = place & 15;
            const uint64_t file = m_physical + it->phys + (a - it->virt);
            m_work.resize(size_t(skip + (b - a)));
            if (file < skip || !m_s.raw(file - skip, m_work.data(), m_work.size())) return false;
            uint8_t ctr[16];
            Counter(m_s.fsh.data(), place - skip, ctr);
            m_s.ctr(m_s.key.data(), ctr, m_work.data(), m_work.size());
            std::memcpy(buf + (a - offset), m_work.data() + skip, size_t(b - a));
        }
        return true;
    }
    uint64_t Size() const override { return m_size; }

private:
    struct Piece {
        uint64_t virt, phys, end;
        uint32_t storage; // 0 stored, 1 zeros
    };
    NcaSection m_s;
    uint64_t m_physical = 0, m_size = 0;
    std::vector<Piece> m_pieces;
    std::vector<uint8_t> m_work;
};

// The RomFS section of an update: AES-CTR with the generation of each run in its counter (the
// subsection table); past the runs (its tables), the counter of the section.
class CtrEx : public Section {
public:
    bool Open(const NcaSection& s, std::string& error) {
        m_s = s;
        m_plain = std::make_unique<Plain>(s);
        const uint8_t* f = s.fsh.data();
        const uint64_t table_offset = Get64(f + 0x120), table_size = Get64(f + 0x128);
        const uint32_t count = Get32(f + 0x138);
        if (Get32(f + 0x130) != kBktr || table_size < kNode || table_size > 64 * 1024 * 1024 || table_offset > s.size || table_size > s.size - table_offset) {
            error = "subsection table";
            return false;
        }
        std::vector<uint8_t> table(static_cast<size_t>(table_size));
        if (!m_plain->Read(table_offset, table.data(), table.size())) {
            error = "subsection table read";
            return false;
        }
        std::vector<const uint8_t*> raw;
        if (!BucketEntries(table, 0x10, count, raw, m_end)) {
            error = "subsection entries";
            return false;
        }
        for (const uint8_t* e : raw) m_runs.push_back({Get64(e), Get32(e + 12)});
        for (size_t i = 0; i < m_runs.size(); i++) {
            const uint64_t next = i + 1 < m_runs.size() ? m_runs[i + 1].offset : m_end;
            if ((i == 0 && m_runs[0].offset != 0) || next <= m_runs[i].offset) {
                error = "subsections do not follow each other";
                return false;
            }
        }
        return true;
    }
    bool Read(uint64_t offset, uint8_t* buf, size_t size) override {
        if (offset > m_s.size || size > m_s.size - offset) return false;
        while (size) {
            if (offset >= m_end) return m_plain->Read(offset, buf, size);
            auto it = std::upper_bound(m_runs.begin(), m_runs.end(), offset, [](uint64_t o, const Run& r) { return o < r.offset; }) - 1;
            const uint64_t next = it + 1 != m_runs.end() ? (it + 1)->offset : m_end;
            const size_t n = size_t(std::min<uint64_t>(size, next - offset));
            const uint64_t at = m_s.start + offset, from = at & ~uint64_t(15);
            m_work.resize(size_t(at - from) + n);
            if (!m_s.raw(from, m_work.data(), m_work.size())) return false;
            uint8_t ctr[16];
            Counter(m_s.fsh.data(), from, ctr);
            SetGeneration(ctr, it->generation);
            m_s.ctr(m_s.key.data(), ctr, m_work.data(), m_work.size());
            std::memcpy(buf, m_work.data() + (at - from), n);
            buf += n;
            offset += n;
            size -= n;
        }
        return true;
    }
    uint64_t Size() const override { return m_s.size; }

private:
    struct Run {
        uint64_t offset;
        uint32_t generation;
    };
    NcaSection m_s;
    std::unique_ptr<Plain> m_plain;
    std::vector<Run> m_runs;
    uint64_t m_end = 0;
    std::vector<uint8_t> m_work;
};

// The patched section: each run from the update (storage 1) or from the base game (storage 0).
class Patched : public Section {
public:
    bool Open(const NcaSection& update, Section* base, std::string& error) {
        m_base = base;
        auto ex = std::make_unique<CtrEx>();
        if (!ex->Open(update, error)) return false;
        m_update = std::move(ex);
        const uint8_t* f = update.fsh.data();
        const uint64_t table_offset = Get64(f + 0x100), table_size = Get64(f + 0x108);
        const uint32_t count = Get32(f + 0x118);
        if (Get32(f + 0x110) != kBktr || table_size < kNode || table_size > 256 * 1024 * 1024) {
            error = "indirect table";
            return false;
        }
        std::vector<uint8_t> table(static_cast<size_t>(table_size));
        if (!m_update->Read(table_offset, table.data(), table.size())) {
            error = "indirect table read";
            return false;
        }
        std::vector<const uint8_t*> raw;
        if (!BucketEntries(table, 0x14, count, raw, m_end)) {
            error = "indirect entries";
            return false;
        }
        for (size_t i = 0; i < raw.size(); i++) {
            IndirectEntry e{Get64(raw[i]), Get64(raw[i] + 8), int32_t(Get32(raw[i] + 16))};
            const uint64_t next = i + 1 < raw.size() ? Get64(raw[i + 1]) : m_end;
            if ((i == 0 && e.virt != 0) || next <= e.virt || (e.storage != 0 && e.storage != 1)) {
                error = "indirect entry";
                return false;
            }
            m_entries.push_back(e);
        }
        if (!m_base && std::any_of(m_entries.begin(), m_entries.end(), [](const IndirectEntry& e) { return e.storage == 0; })) {
            error = "no base game";
            return false;
        }
        return true;
    }
    bool Read(uint64_t offset, uint8_t* buf, size_t size) override {
        if (offset > m_end || size > m_end - offset) return false;
        while (size) {
            auto it = std::upper_bound(m_entries.begin(), m_entries.end(), offset, [](uint64_t o, const IndirectEntry& e) { return o < e.virt; }) - 1;
            const uint64_t next = it + 1 != m_entries.end() ? (it + 1)->virt : m_end;
            const size_t n = size_t(std::min<uint64_t>(size, next - offset));
            Section* from = it->storage == 1 ? m_update.get() : m_base;
            if (!from->Read(it->phys + (offset - it->virt), buf, n)) return false;
            buf += n;
            offset += n;
            size -= n;
        }
        return true;
    }
    uint64_t Size() const override { return m_end; }
    const std::vector<IndirectEntry>* Indirect() const override { return &m_entries; }

private:
    Section* m_base = nullptr;
    std::unique_ptr<CtrEx> m_update;
    std::vector<IndirectEntry> m_entries;
    uint64_t m_end = 0;
};

} // namespace

bool IsSparse(const uint8_t* fsh) { return Get16(fsh + 0x170) != 0 || !Zero(fsh + 0x150, 8); }

bool IsPatch(const uint8_t* fsh) { return !Zero(fsh + 0x100, 0x40); }

std::unique_ptr<Section> OpenBase(const NcaSection& s, std::string& error) {
    if (IsSparse(s.fsh.data())) {
        const uint8_t enc = s.fsh[4];
        if (enc != 3 && enc != 5) {
            error = "sparse section not AES-CTR";
            return nullptr;
        }
        auto sp = std::make_unique<Sparse>();
        if (!sp->Open(s, error)) return nullptr;
        return sp;
    }
    const uint8_t enc = s.fsh[4];
    if (enc != 1 && enc != 3 && enc != 5) {
        error = "base section encryption";
        return nullptr;
    }
    return std::make_unique<Plain>(s);
}

std::unique_ptr<Section> OpenPatched(const NcaSection& update, Section* base, std::string& error) {
    const uint8_t enc = update.fsh[4];
    if (!IsPatch(update.fsh.data()) || (enc != 4 && enc != 6)) {
        error = "not an update section";
        return nullptr;
    }
    auto p = std::make_unique<Patched>();
    if (!p->Open(update, base, error)) return nullptr;
    return p;
}

bool IndirectEntries(Section& patched, std::vector<IndirectEntry>& out, uint64_t& end) {
    const auto* entries = patched.Indirect();
    if (!entries) return false;
    out = *entries;
    end = patched.Size();
    return true;
}

bool Romfs::Open(Section* section, const uint8_t* fsh, lz4pack::HashFn hash, std::string& error) {
    m_section = section;
    m_hash = std::move(hash);
    const uint8_t* h = fsh + 8;
    if (fsh[3] != 3 || Get32(h) != 0x43465649) { // IVFC
        error = "not an IVFC RomFS";
        return false;
    }
    const uint32_t layers = Get32(h + 0xC);
    if (layers < 3 || layers > 7) {
        error = "IVFC levels";
        return false;
    }
    const uint8_t* data = h + 0x10 + (layers - 2) * 0x18;
    const uint8_t* above = h + 0x10 + (layers - 3) * 0x18;
    m_data_offset = Get64(data);
    m_data_size = Get64(data + 8);
    const uint32_t log2 = Get32(data + 0x10);
    if (log2 < 9 || log2 > 24 || m_data_offset > section->Size() || m_data_size > section->Size() - m_data_offset) {
        error = "IVFC data level";
        return false;
    }
    m_block = 1u << log2;
    const uint64_t blocks = (m_data_size + m_block - 1) / m_block;
    const uint64_t above_size = Get64(above + 8);
    if (above_size < blocks * 32 || above_size > 512ull * 1024 * 1024) {
        error = "IVFC hash level";
        return false;
    }
    m_hashes.resize(size_t(blocks * 32));
    if (!section->Read(Get64(above), m_hashes.data(), m_hashes.size())) {
        error = "IVFC hash level read";
        return false;
    }
    m_checked.assign(size_t((blocks + 7) / 8), 0);
    m_size = m_data_size;
    m_entries.clear();
    // the compressed storage, its table in the data level
    const uint64_t table_offset = Get64(fsh + 0x178), table_size = Get64(fsh + 0x180);
    if (table_size) {
        const uint32_t count = Get32(fsh + 0x190);
        if (Get32(fsh + 0x188) != kBktr || table_size > 256 * 1024 * 1024 || table_offset > m_data_size || table_size > m_data_size - table_offset) {
            error = "compression table";
            return false;
        }
        std::vector<uint8_t> table(static_cast<size_t>(table_size));
        if (!ReadStored(table_offset, table.data(), table.size()) || !lz4pack::ParseTable(table.data(), table.size(), count, m_entries, m_virt_end)) {
            error = m_damaged ? "damaged" : "compression table entries";
            return false;
        }
        m_size = m_virt_end;
    }
    return true;
}

bool Romfs::ReadStored(uint64_t offset, uint8_t* buf, size_t size) {
    if (offset > m_data_size || size > m_data_size - offset) return false;
    if (!m_section->Read(m_data_offset + offset, buf, size)) return false;
    // each block not checked yet, against its hash: whole in what was read, or read whole
    std::vector<uint8_t> block;
    for (uint64_t b = offset / m_block; b * m_block < offset + size; b++) {
        if (m_checked[size_t(b / 8)] & (1u << (b % 8))) continue;
        const uint64_t from = b * m_block, n = std::min<uint64_t>(m_block, m_data_size - from);
        block.assign(m_block, 0);
        if (from >= offset && from + n <= offset + size) {
            std::memcpy(block.data(), buf + (from - offset), size_t(n));
        } else if (!m_section->Read(m_data_offset + from, block.data(), size_t(n))) {
            return false;
        }
        uint8_t d[32];
        m_hash(block.data(), block.size(), d);
        if (std::memcmp(d, m_hashes.data() + b * 32, 32)) {
            m_damaged++;
            return false;
        }
        m_checked[size_t(b / 8)] |= uint8_t(1u << (b % 8));
    }
    return true;
}

bool Romfs::StoredBlock(uint64_t virt, uint32_t size, std::vector<uint8_t>& out) {
    auto it = std::lower_bound(m_entries.begin(), m_entries.end(), virt, [](const lz4pack::Entry& e, uint64_t v) { return e.virt < v; });
    if (it == m_entries.end() || it->virt != virt || it->type != lz4pack::kLz4) return false;
    const uint64_t next = it + 1 != m_entries.end() ? (it + 1)->virt : m_virt_end;
    if (next - virt != size || it->size > lz4pack::kBlock) return false;
    out.resize(it->size);
    return ReadStored(it->phys, out.data(), out.size());
}

bool Romfs::Read(uint64_t offset, uint8_t* buf, size_t size) {
    if (offset > m_size || size > m_size - offset) return false;
    if (m_entries.empty()) return ReadStored(offset, buf, size);
    return lz4pack::Unpack(m_entries, m_virt_end, [this](uint64_t o, uint8_t* b, size_t n) { return ReadStored(o, b, n); }, offset, buf, size);
}

} // namespace nxdock::patchview
