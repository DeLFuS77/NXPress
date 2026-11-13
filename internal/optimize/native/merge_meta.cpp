// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 DeLFuS77
#include "merge_meta.hpp"

#include <algorithm>
#include <cstdio>
#include <cstring>

namespace nxdock::mergemeta {

namespace {

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
void Put16(uint8_t* p, uint16_t v) { std::memcpy(p, &v, 2); }
void Put32(uint8_t* p, uint32_t v) { std::memcpy(p, &v, 4); }
void Put64(uint8_t* p, uint64_t v) { std::memcpy(p, &v, 8); }

uint64_t Align(uint64_t v, uint64_t a) { return (v + a - 1) / a * a; }

void PutInfo(uint8_t* p, const std::array<uint8_t, 16>& id, uint64_t size, uint8_t attributes, uint8_t type, uint8_t id_offset) {
    std::memcpy(p, id.data(), 16);
    Put32(p + 0x10, uint32_t(size));
    p[0x14] = uint8_t(size >> 32);
    p[0x15] = attributes;
    p[0x16] = type;
    p[0x17] = id_offset;
}

} // namespace

bool ParseCnmt(const uint8_t* d, size_t n, Cnmt& out) {
    if (n < 0x20 + 0x20) return false;
    out = Cnmt{};
    std::memcpy(out.header.data(), d, 0x20);
    out.title_id = Get64(d);
    out.version = Get32(d + 8);
    out.type = d[0xC];
    const uint16_t ext = Get16(d + 0xE), count = Get16(d + 0x10), metas = Get16(d + 0x12);
    size_t at = 0x20;
    if (at + ext > n) return false;
    out.extended.assign(d + at, d + at + ext);
    at += ext;
    if (at + size_t(count) * 0x38 > n) return false;
    for (uint16_t i = 0; i < count; i++, at += 0x38) {
        Content c;
        std::memcpy(c.hash.data(), d + at, 32);
        const uint8_t* info = d + at + 0x20;
        std::memcpy(c.id.data(), info, 16);
        c.size = uint64_t(Get32(info + 0x10)) | uint64_t(info[0x14]) << 32;
        c.attributes = info[0x15];
        c.type = info[0x16];
        c.id_offset = info[0x17];
        out.contents.push_back(c);
    }
    if (at + size_t(metas) * 0x10 > n) return false;
    out.meta_infos.assign(d + at, d + at + size_t(metas) * 0x10);
    at += size_t(metas) * 0x10;
    // what is left before the digest: the extended data (a Patch says how much it has)
    if (n < at + 0x20) return false;
    out.extended_data.assign(d + at, d + n - 0x20);
    std::memcpy(out.digest.data(), d + n - 0x20, 0x20);
    return true;
}

std::vector<uint8_t> WriteCnmt(const Cnmt& c) {
    std::vector<uint8_t> o(0x20);
    std::memcpy(o.data(), c.header.data(), 0x20);
    Put64(o.data(), c.title_id);
    Put32(o.data() + 8, c.version);
    o[0xC] = c.type;
    Put16(o.data() + 0xE, uint16_t(c.extended.size()));
    Put16(o.data() + 0x10, uint16_t(c.contents.size()));
    Put16(o.data() + 0x12, uint16_t(c.meta_infos.size() / 0x10));
    o.insert(o.end(), c.extended.begin(), c.extended.end());
    for (const auto& k : c.contents) {
        const size_t at = o.size();
        o.resize(at + 0x38);
        std::memcpy(o.data() + at, k.hash.data(), 32);
        PutInfo(o.data() + at + 0x20, k.id, k.size, k.attributes, k.type, k.id_offset);
    }
    o.insert(o.end(), c.meta_infos.begin(), c.meta_infos.end());
    o.insert(o.end(), c.extended_data.begin(), c.extended_data.end());
    o.insert(o.end(), c.digest.begin(), c.digest.end());
    return o;
}

bool MergedCnmt(const Cnmt& base, const Cnmt& update, const Content& program, Cnmt& out) {
    if (base.type != kTypeApplication || update.type != kTypePatch || base.extended.size() < 0x10 || update.extended.size() < 0x10) return false;
    if (Get64(update.extended.data()) != base.title_id) return false; // the update of another game
    out = base;
    out.version = update.version;
    // the firmware: the higher of the two, and the one of the LZ4 layer
    const uint32_t need = std::max({Get32(base.extended.data() + 8), Get32(update.extended.data() + 8), kFirmware12});
    Put32(out.extended.data() + 8, need);
    Put32(out.extended.data() + 0xC, 0); // no required application version
    // whole now: a base Nintendo cut down (Compacted) no longer needs its update
    out.header[0x14] &= uint8_t(~kAttributeCompacted);
    // the download firmware of the header: the higher of the two
    Put32(out.header.data() + 0x18, std::max(Get32(base.header.data() + 0x18), Get32(update.header.data() + 0x18)));
    out.contents.clear();
    bool have_program = false;
    for (const auto& c : update.contents) {
        if (c.type == kContentDeltaFragment) continue;
        if (c.type == kContentProgram && c.id_offset == program.id_offset) {
            out.contents.push_back(program);
            have_program = true;
        } else {
            out.contents.push_back(c);
        }
    }
    out.extended_data.clear();
    return have_program;
}

bool CnmtFromSection(const std::vector<uint8_t>& s, const uint8_t* fsh, std::vector<uint8_t>& cnmt) {
    if (fsh[3] != 2) return false; // HierarchicalSha256
    const uint64_t po = Get64(fsh + 0x40), ps = Get64(fsh + 0x48);
    if (po > s.size() || ps > s.size() - po || ps < 0x10) return false;
    const uint8_t* pfs = s.data() + po;
    if (std::memcmp(pfs, "PFS0", 4) != 0) return false;
    const uint32_t files = Get32(pfs + 4), strings = Get32(pfs + 8);
    const uint64_t head = 0x10 + uint64_t(files) * 0x18 + strings;
    if (head > ps) return false;
    for (uint32_t i = 0; i < files; i++) {
        const uint8_t* e = pfs + 0x10 + i * 0x18;
        const uint64_t name_at = 0x10 + uint64_t(files) * 0x18 + Get32(e + 0x10);
        if (name_at >= head) return false;
        const std::string name(reinterpret_cast<const char*>(pfs + name_at), strnlen(reinterpret_cast<const char*>(pfs + name_at), size_t(head - name_at)));
        if (name.size() <= 5 || name.compare(name.size() - 5, 5, ".cnmt") != 0) continue;
        const uint64_t off = Get64(e), n = Get64(e + 8);
        if (off > ps - head || n > ps - head - off) return false;
        const uint64_t at = head + off;
        cnmt.assign(pfs + at, pfs + at + n);
        return true;
    }
    return false;
}

bool MetaNca(const uint8_t* model, const Cnmt& c, const HashFn& hash, std::vector<uint8_t>& header, std::vector<uint8_t>& section) {
    if (std::memcmp(model + 0x200, "NCA3", 4) != 0 || model[0x205] != 1) return false;
    const uint8_t* mfsh = model + 0x400;
    if (mfsh[3] != 2) return false;
    uint32_t block = Get32(mfsh + 0x28);
    if (block < 0x200 || block > 0x100000) block = 0x1000;
    // the PFS0: one file, its name padded so the head is a whole number of 0x20
    char name[64];
    std::snprintf(name, sizeof(name), "%s_%016llx.cnmt", c.type == kTypeApplication ? "Application" : "Patch", (unsigned long long)c.title_id);
    const std::vector<uint8_t> cnmt = WriteCnmt(c);
    const size_t name_len = std::strlen(name) + 1;
    const size_t strings = size_t(Align(0x10 + 0x18 + name_len, 0x20) - (0x10 + 0x18));
    std::vector<uint8_t> pfs(0x10 + 0x18 + strings, 0);
    std::memcpy(pfs.data(), "PFS0", 4);
    Put32(pfs.data() + 4, 1);
    Put32(pfs.data() + 8, uint32_t(strings));
    Put64(pfs.data() + 0x10, 0);
    Put64(pfs.data() + 0x18, cnmt.size());
    Put32(pfs.data() + 0x20, 0);
    std::memcpy(pfs.data() + 0x28, name, name_len);
    pfs.insert(pfs.end(), cnmt.begin(), cnmt.end());
    // the hash table, then the PFS0 on 0x200, the section a whole number of 0x200
    const uint64_t blocks = (pfs.size() + block - 1) / block;
    const uint64_t hash_size = blocks * 32, pfs_at = Align(hash_size, 0x200);
    section.assign(size_t(Align(pfs_at + pfs.size(), 0x200)), 0);
    std::memcpy(section.data() + pfs_at, pfs.data(), pfs.size());
    for (uint64_t b = 0; b < blocks; b++) {
        const uint64_t from = b * block, n = std::min<uint64_t>(block, pfs.size() - from);
        hash(pfs.data() + from, size_t(n), section.data() + b * 32);
    }
    header.assign(model, model + 0xC00);
    uint8_t* fsh = header.data() + 0x400;
    hash(section.data(), size_t(hash_size), fsh + 8); // the master hash
    Put32(fsh + 0x28, block);
    Put32(fsh + 0x2C, 2);
    Put64(fsh + 0x30, 0);
    Put64(fsh + 0x38, hash_size);
    Put64(fsh + 0x40, pfs_at);
    Put64(fsh + 0x48, pfs.size());
    // one section, from 0xC00
    for (int i = 1; i < 4; i++) {
        std::memset(header.data() + 0x240 + i * 0x10, 0, 0x10);
        std::memset(header.data() + 0x280 + i * 0x20, 0, 0x20);
        std::memset(header.data() + 0x400 + i * 0x200, 0, 0x200);
    }
    Put32(header.data() + 0x240, 0xC00 / 0x200);
    Put32(header.data() + 0x244, uint32_t((0xC00 + section.size()) / 0x200));
    Put64(header.data() + 0x208, 0xC00 + section.size());
    hash(fsh, 0x200, header.data() + 0x280);
    return true;
}

uint32_t RecordedSystemVersion(const uint8_t* r, size_t n) {
    if (n < 8) return UINT32_MAX;
    const uint16_t ext = Get16(r);
    if (ext < 0xC || n < 8 + size_t(ext)) return UINT32_MAX;
    return Get32(r + 8 + 8);
}

void KeepRecordedSystem(Cnmt& m, uint32_t recorded) {
    if (m.extended.size() < 0xC) return;
    const uint32_t asks = Get32(m.extended.data() + 8);
    Put32(m.extended.data() + 8, std::max(std::min(asks, recorded), kFirmware12));
}

std::vector<uint8_t> DbRecord(const Cnmt& c, const std::array<uint8_t, 16>& meta_id, uint64_t meta_size) {
    std::vector<uint8_t> o(8, 0);
    Put16(o.data(), uint16_t(c.extended.size()));
    Put16(o.data() + 2, uint16_t(c.contents.size() + 1));
    Put16(o.data() + 4, uint16_t(c.meta_infos.size() / 0x10));
    o[6] = c.header[0x14]; // attributes
    o.insert(o.end(), c.extended.begin(), c.extended.end());
    auto put = [&](const std::array<uint8_t, 16>& id, uint64_t size, uint8_t attributes, uint8_t type, uint8_t id_offset) {
        const size_t at = o.size();
        o.resize(at + 0x18);
        PutInfo(o.data() + at, id, size, attributes, type, id_offset);
    };
    put(meta_id, meta_size, 0, kContentMeta, 0);
    for (const auto& k : c.contents) put(k.id, k.size, k.attributes, k.type, k.id_offset);
    o.insert(o.end(), c.meta_infos.begin(), c.meta_infos.end());
    return o;
}

} // namespace nxdock::mergemeta
