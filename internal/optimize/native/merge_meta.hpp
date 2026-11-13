// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 DeLFuS77
#pragma once

// What the console is told about a game merged with its update (25D): one Application of the
// version of the update, no Patch.
//  - Its CNMT is the one of the base game (the same id, attributes and extended header) with the
//    version of the update, the contents of the update (the merged program in place of its program,
//    without its delta fragments) and no required application version, so the console does not
//    ask for the update it already has; the firmware it needs is at least the higher of the two
//    and 12.0.0 (the LZ4 layer of the NCA). A base game Nintendo cut down is marked Compacted
//    (whole only with its update): the merged game is whole, so the mark goes, or the console
//    asks to download the update before the game starts.
//  - Its meta NCA is made from the one of the base game: the same header and key area, one section
//    with a PFS0 holding the .cnmt, hashed in blocks (HierarchicalSha256), as Nintendo makes them.
//  - The record of the database of ncm: its header, extended header and every content, the meta
//    NCA first.
//
// Plain C++: tested on the computer.
#include <array>
#include <cstdint>
#include <functional>
#include <string>
#include <vector>

namespace nxdock::mergemeta {

using HashFn = std::function<void(const uint8_t* data, size_t size, uint8_t out[32])>;

enum : uint8_t { kTypeApplication = 0x80, kTypePatch = 0x81 };
enum : uint8_t { kContentMeta = 0, kContentProgram = 1, kContentDeltaFragment = 6 };

constexpr uint32_t kFirmware12 = 12u << 26; // 12.0.0, as the console writes a version
// The attributes of a CNMT (its header at 0x14): a base game that is whole only with its update.
constexpr uint8_t kAttributeCompacted = 0x04;

// One content of a CNMT: its hash, then the content info of ncm (id, size, attributes, type, id
// offset), 0x38 bytes.
struct Content {
    std::array<uint8_t, 32> hash{};
    std::array<uint8_t, 16> id{};
    uint64_t size = 0;
    uint8_t attributes = 0, type = 0, id_offset = 0;
};

struct Cnmt {
    uint64_t title_id = 0;
    uint32_t version = 0;
    uint8_t type = 0;
    std::array<uint8_t, 0x20> header{}; // as it was, for the fields not set here
    std::vector<uint8_t> extended;      // the extended header
    std::vector<Content> contents;
    std::vector<uint8_t> meta_infos;    // content meta infos (0x10 each)
    std::vector<uint8_t> extended_data; // a Patch's history, a Delta's...
    std::array<uint8_t, 32> digest{};
};

bool ParseCnmt(const uint8_t* data, size_t size, Cnmt& out);
std::vector<uint8_t> WriteCnmt(const Cnmt& c);

// The CNMT of the merged game from the CNMT of the base (Application) and of the update (Patch),
// with the merged program (`program`) in place of the program of the update.
bool MergedCnmt(const Cnmt& base, const Cnmt& update, const Content& program, Cnmt& out);
// The .cnmt in the decrypted meta section (HierarchicalSha256 over a PFS0), from its FS header.
bool CnmtFromSection(const std::vector<uint8_t>& section, const uint8_t* fsh, std::vector<uint8_t>& cnmt);

// A meta NCA for `cnmt`, from the decrypted header (0xC00) of another meta NCA of the game: its
// header (decrypted, its section, FS header, size and hash set) and its one section, not encrypted
// yet (from 0xC00 in the NCA).
bool MetaNca(const uint8_t* model_header, const Cnmt& cnmt, const HashFn& hash, std::vector<uint8_t>& header, std::vector<uint8_t>& section);

// The system version a record of the database of ncm asks for (its extended header, after the id
// of the game or of its update), UINT32_MAX when it does not say.
uint32_t RecordedSystemVersion(const uint8_t* record, size_t size);
// A game merged on the console asks no more of the system than the record of what it replaces
// (an installer such as DBI can have lowered it so the console starts the game), and never less
// than the LZ4 layer needs (12.0.0). Asking for more, the console asks for a system update first.
void KeepRecordedSystem(Cnmt& merged, uint32_t recorded);

// The record of the database of ncm for `cnmt`, its meta NCA (id, size) listed first.
std::vector<uint8_t> DbRecord(const Cnmt& cnmt, const std::array<uint8_t, 16>& meta_id, uint64_t meta_size);

} // namespace nxdock::mergemeta
