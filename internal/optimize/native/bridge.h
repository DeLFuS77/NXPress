// The C side of "Optimize for install": what Go calls of the code of NXDock (patch_view, merge_game,
// merge_meta), and what that code calls of Go (reads, AES, SHA-256, writes, progress). This file
// and bridge.cpp belong to NXPress; the rest of this folder is copied from NXDock
// (tools/nxdocksync).
#ifndef NXPRESS_OPTIMIZE_BRIDGE_H
#define NXPRESS_OPTIMIZE_BRIDGE_H

#include <stddef.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

// One section of an NCA: which NCA it is (told back to nxoGoRead), its key, its FS header and
// where it is in the NCA.
typedef struct {
    int which;
    uint8_t key[16];
    uint8_t fsh[0x200];
    uint64_t start, size;
} nxo_section;

// The program NCA of a game merged with its update.
typedef struct nxo_merger nxo_merger;

typedef struct {
    uint64_t romfs_size;   // the RomFS as the game reads it
    uint64_t stored_size;  // its data level as stored, with its table
    uint64_t nca_size;     // the new NCA (after measure)
    uint64_t runs_again;   // bytes stored once for runs read again from the base
    uint64_t blocks_again; // bytes stored once for blocks the same as another
    uint64_t old_kept;     // blocks where the LZ4 of Nintendo was the smaller
    uint64_t damaged;      // blocks of the base or the update found damaged
} nxo_info;

// The phases told to nxoGoStep.
enum { NXO_MEASURE = 1, NXO_WRITE = 2 };

// `handle` is given back to every call of Go. Every function giving int gives 1 when it worked,
// 0 with the reason in err.
nxo_merger* nxo_merger_new(uintptr_t handle);
int nxo_merger_open(nxo_merger* m, const nxo_section* base, const nxo_section* update, int update_index,
                    const uint8_t* update_header, int level, unsigned threads, char* err, size_t errlen);
int nxo_merger_measure(nxo_merger* m, char* err, size_t errlen);
int nxo_merger_write(nxo_merger* m, char* err, size_t errlen);
void nxo_merger_info(nxo_merger* m, nxo_info* out);
void nxo_merger_free(nxo_merger* m);

// A RomFS as the game reads it: of a section alone (update NULL), or of an update over its base.
// Each block is checked against the IVFC above it the first time it is read.
typedef struct nxo_romfs nxo_romfs;
nxo_romfs* nxo_romfs_open(uintptr_t handle, const nxo_section* base, const nxo_section* update, char* err, size_t errlen);
uint64_t nxo_romfs_size(nxo_romfs* r);
uint64_t nxo_romfs_stored(nxo_romfs* r);
int nxo_romfs_compressed(nxo_romfs* r);
int nxo_romfs_read(nxo_romfs* r, uint64_t offset, uint8_t* buf, size_t size);
uint64_t nxo_romfs_damaged(nxo_romfs* r);
void nxo_romfs_free(nxo_romfs* r);

// One content of a CNMT.
typedef struct {
    uint8_t id[16];
    uint8_t hash[32];
    uint64_t size;
    uint8_t type, id_offset;
} nxo_content;

// The meta NCA of the merged game, from the meta NCAs of the base and the update (their headers
// decrypted, 0xC00 bytes, and their section decrypted) and the merged program: its header
// (decrypted, 0xC00 bytes into out_header), its section not encrypted yet (from 0xC00 in the NCA,
// to be freed with nxo_free), and the contents of its CNMT.
int nxo_merged_meta(uintptr_t handle, const uint8_t* base_header, const uint8_t* base_section, size_t base_len,
                    const uint8_t* update_header, const uint8_t* update_section, size_t update_len, const nxo_content* program,
                    uint8_t* out_header, uint8_t** out_section, size_t* out_len, nxo_content** contents, size_t* count,
                    uint64_t* title_id, uint32_t* version, uint32_t* base_version, char* err, size_t errlen);
void nxo_free(void* p);

// The CNMT of a meta section: its title ID, type and version, and its contents (nxo_free).
int nxo_read_cnmt(const uint8_t* header, const uint8_t* section, size_t len, uint64_t* title_id, uint8_t* type,
                  uint32_t* version, nxo_content** contents, size_t* count, char* err, size_t errlen);

// What Go gives (exported in callbacks.go).
extern int nxoGoRead(uintptr_t handle, int which, uint64_t offset, uint8_t* buf, size_t size);
extern void nxoGoCtr(uint8_t* key, uint8_t* ctr, uint8_t* data, size_t size);
extern void nxoGoSha256(uint8_t* data, size_t size, uint8_t* out);
extern void nxoGoHeader(uintptr_t handle, uint8_t* header, size_t size);
extern int nxoGoOut(uintptr_t handle, uint8_t* data, size_t size);
extern int nxoGoStep(uintptr_t handle, int phase, uint64_t done);

#ifdef __cplusplus
}
#endif

#endif
