// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 DeLFuS77
#pragma once

// The compressed storage of an NCA section (firmware 12 and later), written as Nintendo writes it
// (24A, "Compress games"). Seen in games Nintendo ships compressed:
//  - only the data level of the RomFS (the last level of its IVFC) is compressed; the hash levels
//    stay before it, and they hash the data level as it is stored (the blocks and the table), not
//    as it reads decompressed: the FS checks what it reads from the NCA, then decompresses it
//    (Atmosphère: CreateStorageByRawStorage). So they are made again (BuildIvfc), and the master
//    hash in the FS header with them;
//  - the stored data level is: the first 0x200 bytes as they are (an entry of kind "none" whose
//    stored size is 0xFFFFFFFF), then blocks of at most 64 KiB, each LZ4 or as it is, at places
//    aligned to 0x10, then the table, right after the last block (0x10 aligned);
//  - the table is a bucket tree of 0x4000-byte nodes: a first node with how many sets and where
//    each starts, then one node per set of up to 682 entries of 0x18 bytes: the offset of the
//    block in the decompressed data, its place after the start of the stored data level, its
//    kind (0 none, 1 zeros, 3 LZ4), the level it was made at and its stored size;
//  - the FS header keeps the place and size of the table (after the start of the data level) and
//    its BKTR header at 0x178; the size of the data level becomes what is stored (blocks + table).
// The FS takes compressed blocks of at most 64 KiB stored (block_size_max), so blocks stay at
// 64 KiB.
// Atmosphère: fssystem_compressed_storage.hpp, fssystem_bucket_tree.cpp,
// fssystem_nca_file_system_driver.cpp.
//
// Plain C++: tested on the computer.
#include <cstddef>
#include <cstdint>
#include <functional>
#include <vector>

namespace nxdock::lz4pack {

constexpr uint32_t kBlock = 0x10000;     // the blocks, as Nintendo's
constexpr uint32_t kHead = 0x200;        // the start, kept as it is
constexpr uint32_t kNode = 0x4000;       // a node of the table
constexpr uint32_t kPerSet = (kNode - 0x10) / 0x18; // 682
constexpr uint8_t kNone = 0, kZeros = 1, kLz4 = 3;
constexpr uint32_t kNoneSize = 0xFFFFFFFF;

struct Entry {
    uint64_t virt = 0, phys = 0;
    uint8_t type = kNone;
    int8_t level = 0;
    uint32_t size = kNoneSize; // stored bytes of an LZ4 block
};

struct Packed {
    std::vector<Entry> entries;
    uint64_t virt_size = 0;
    uint64_t stored = 0;      // stored bytes of the blocks, the last one included
    uint64_t table_offset = 0; // after the start of the data level: stored, aligned to 0x10
};

using ReadFn = std::function<bool(uint64_t offset, uint8_t* buf, size_t size)>;     // the plain data level
using WriteFn = std::function<bool(uint64_t offset, const uint8_t* data, size_t size)>; // its stored form, in order
using StepFn = std::function<bool(uint64_t done)>;                                    // false: stop

// Packs a data level of virt_size bytes. Blocks that do not get at least min_gain bytes smaller
// stay as they are; next to each other they make one entry. Padding between stored blocks is
// written as zeros, so the stored form is written in one go, from 0 to stored. The blocks are
// read and written in order, and compressed on `threads` threads: the same bytes with any number.
bool Pack(uint64_t virt_size, int level, uint32_t min_gain, const ReadFn& read, const WriteFn& write, const StepFn& step, Packed& out,
          unsigned threads = 1);

// The table of the entries, as the FS reads it; its size is a whole number of nodes.
std::vector<uint8_t> BuildTable(const std::vector<Entry>& entries, uint64_t virt_end);
// And read back (for the checks): false when it is not a table of count entries.
bool ParseTable(const uint8_t* table, size_t size, uint32_t count, std::vector<Entry>& out, uint64_t& virt_end);

// The plain bytes of [offset, offset + size) from the stored form: what the FS gives.
bool Unpack(const std::vector<Entry>& entries, uint64_t virt_end, const ReadFn& stored, uint64_t offset, uint8_t* out, size_t size);

// What packing would save, from a sample of the blocks (one in every `every`): the share of the
// bytes saved, from 0 to 1.
double EstimateGain(uint64_t virt_size, int level, uint32_t every, const ReadFn& read);

// A data level Nintendo packed already, packed again at `level`: the share of its stored bytes
// that would be saved, from a sample of its LZ4 blocks (one in every `every`); a block never comes
// out bigger than it was. `stored` reads the stored data level.
double EstimateRepack(const std::vector<Entry>& entries, uint64_t virt_end, int level, uint32_t every, const ReadFn& stored);
// The share of the LZ4 blocks of a table made at a given level: Nintendo's fast LZ4 (level 0)
// leaves room, its HC levels (5, 16) do not.
double ShareAtLevel(const std::vector<Entry>& entries, int level);

// The IVFC of a RomFS, made again for a new data level.
using HashFn = std::function<void(const uint8_t* data, size_t size, uint8_t out[32])>;

// One SHA-256 per block of a level, the last block padded with zeros to the block size, as the
// FS checks it. Fed in order.
class BlockHasher {
public:
    BlockHasher(uint32_t block_size, HashFn hash);
    void Feed(const uint8_t* data, size_t size);
    // the hashes, 32 bytes each; the hasher is empty again
    std::vector<uint8_t> Finish();
    uint64_t Fed() const { return m_fed; }

private:
    uint32_t m_size;
    HashFn m_hash;
    std::vector<uint8_t> m_block, m_out;
    size_t m_fill = 0;
    uint64_t m_fed = 0;
};

struct IvfcLevel {
    uint64_t offset = 0, size = 0;
    uint32_t log2 = 14; // the size of its blocks
};

// The levels above the data level and the master hash, from the hashes of the data level: level i
// holds the hashes of the blocks of level i + 1, and the master hash is the hash of level 0, each
// hashed in blocks of its own size. `levels` are all of them, the data level last. As in the games
// Nintendo compresses, each level above the data keeps the size it is given (the one it had, made
// for the data decompressed) with zeros after its hashes, and is never less than its hashes
// rounded up to a whole block; only the blocks holding its hashes are hashed in the level above,
// whose entries for the zeros stay zero (as Nintendo's). The sizes are set here. `out` gets the content of each level above
// the data level. False when a level would not fit before the next one.
bool BuildIvfc(std::vector<uint8_t> data_hashes, std::vector<IvfcLevel>& levels, const HashFn& hash,
               std::vector<std::vector<uint8_t>>& out, uint8_t master[32]);

} // namespace nxdock::lz4pack
