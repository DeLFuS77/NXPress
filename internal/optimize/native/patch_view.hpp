// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 DeLFuS77
#pragma once

// The RomFS of a game with its update, read as the console reads it (25B, "Merge and compress"):
//  - the base game: its RomFS section, AES-CTR, or cut down to what its update does not replace
//    (sparse storage: the pieces it holds, one after the other, and zeros for the rest, each piece
//    decrypted with the counter of its place in the whole section);
//  - the update: its RomFS section is AES-CTR-Ex, a counter for each run of it (its subsection
//    table), and the indirect table says, for each run of the patched section, whether it comes
//    from the update or from the base game, and from where;
//  - over the patched section, the IVFC of the update (which hashes it as it is stored) and, when
//    the update has it, the compressed storage (romfs_lz4.hpp).
// Atmosphère: fssystem_nca_file_system_driver.cpp (CreateSparseStorage, CreatePatchMetaStorage,
// CreateIndirectStorage, CreateAesCtrExStorage), fssystem_indirect_storage.hpp,
// fssystem_sparse_storage.hpp, fssystem_aes_ctr_counter_extended_storage.hpp.
//
// Plain C++: the encrypted bytes of each NCA and AES-CTR are given to it, so it is tested on the
// computer against the library app, with real games.
#include "romfs_lz4.hpp"

#include <array>
#include <cstddef>
#include <cstdint>
#include <functional>
#include <memory>
#include <string>
#include <vector>

namespace nxdock::patchview {

// The encrypted bytes of an NCA, at their place in it.
using RawRead = std::function<bool(uint64_t offset, uint8_t* buf, size_t size)>;
// AES-128-CTR of `size` bytes in place, from the start of a 16-byte block with this counter.
using CtrFn = std::function<void(const uint8_t key[16], const uint8_t ctr[16], uint8_t* data, size_t size)>;

// What the patched section is made of, for the merge: each run of the patched section, from the
// update (storage 1) or from a place of the base game (storage 0).
struct IndirectEntry {
    uint64_t virt = 0, phys = 0;
    int32_t storage = 0;
};

// A section of an NCA as it is read: decrypted, at places from its start.
class Section {
public:
    virtual ~Section() = default;
    virtual bool Read(uint64_t offset, uint8_t* buf, size_t size) = 0;
    virtual uint64_t Size() const = 0;
    // the patched section of an update: its indirect entries (null for any other)
    virtual const std::vector<IndirectEntry>* Indirect() const { return nullptr; }
};

// One section of an NCA: where it is, its FS header (0x200 bytes) and the key of its sections.
struct NcaSection {
    RawRead raw;
    CtrFn ctr;
    std::array<uint8_t, 16> key{};
    std::array<uint8_t, 0x200> fsh{};
    uint64_t start = 0, size = 0; // in the NCA
};

// What a section of the base game is: as it is, or sparse.
bool IsSparse(const uint8_t* fsh);
// The RomFS section of an update: an indirect table and AES-CTR-Ex.
bool IsPatch(const uint8_t* fsh);

// The base game: AES-CTR (or not encrypted), or sparse. Null, with `error` set, when it cannot be
// read.
std::unique_ptr<Section> OpenBase(const NcaSection& s, std::string& error);
// The update over its base: the patched section, as stored (before its IVFC). `base` stays the
// caller's and must outlive what is returned.
std::unique_ptr<Section> OpenPatched(const NcaSection& update, Section* base, std::string& error);

// What the patched section is made of, for the merge (IndirectEntries below).
bool IndirectEntries(Section& patched, std::vector<IndirectEntry>& out, uint64_t& end);

// The RomFS a section holds, as the game reads it: its IVFC data level, decompressed through its
// table when it has one. Each block of the data level read is checked against the IVFC level
// above it, as the FS checks it, the first time it is read.
class Romfs {
public:
    // `section` stays the caller's. `hash` makes the SHA-256 of a block.
    bool Open(Section* section, const uint8_t* fsh, lz4pack::HashFn hash, std::string& error);
    bool Read(uint64_t offset, uint8_t* buf, size_t size);
    uint64_t Size() const { return m_size; }
    // the data level as stored, and where it is in the section
    uint64_t StoredSize() const { return m_data_size; }
    bool Compressed() const { return !m_entries.empty(); }
    const std::vector<lz4pack::Entry>& Entries() const { return m_entries; }
    // Nintendo's LZ4 of the block [virt, virt + size) when it has one exactly there: its stored
    // bytes, checked (mergepack::OldBlockFn)
    bool StoredBlock(uint64_t virt, uint32_t size, std::vector<uint8_t>& out);
    // blocks of the data level found damaged (or of the wrong base game) so far
    uint64_t Damaged() const { return m_damaged; }

private:
    bool ReadStored(uint64_t offset, uint8_t* buf, size_t size);
    Section* m_section = nullptr;
    lz4pack::HashFn m_hash;
    uint64_t m_data_offset = 0, m_data_size = 0, m_size = 0, m_virt_end = 0;
    uint32_t m_block = 0;
    std::vector<uint8_t> m_hashes;  // the level above the data level
    std::vector<uint8_t> m_checked; // a bit per block of the data level
    std::vector<lz4pack::Entry> m_entries;
    uint64_t m_damaged = 0;
};

} // namespace nxdock::patchview
