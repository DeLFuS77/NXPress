// The C side of "Optimize for install" (bridge.h): the code of NXDock run with the reads, the AES,
// the SHA-256 and the writes of Go.
#include "bridge.h"

#include "merge_game.hpp"
#include "merge_meta.hpp"
#include "patch_view.hpp"

#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <memory>
#include <new>
#include <string>

using namespace nxdock;

namespace {

void SetError(char* err, size_t errlen, const std::string& what) {
    if (!err || !errlen) return;
    std::snprintf(err, errlen, "%s", what.c_str());
}

void Sha256(const uint8_t* data, size_t size, uint8_t out[32]) {
    nxoGoSha256(const_cast<uint8_t*>(data), size, out);
}

void Ctr(const uint8_t key[16], const uint8_t ctr[16], uint8_t* data, size_t size) {
    nxoGoCtr(const_cast<uint8_t*>(key), const_cast<uint8_t*>(ctr), data, size);
}

patchview::NcaSection Section(uintptr_t handle, const nxo_section& s) {
    patchview::NcaSection out;
    const int which = s.which;
    out.raw = [handle, which](uint64_t offset, uint8_t* buf, size_t size) { return nxoGoRead(handle, which, offset, buf, size) != 0; };
    out.ctr = Ctr;
    std::memcpy(out.key.data(), s.key, 16);
    std::memcpy(out.fsh.data(), s.fsh, 0x200);
    out.start = s.start;
    out.size = s.size;
    return out;
}

void Put(const mergemeta::Content& c, nxo_content& out) {
    std::memcpy(out.id, c.id.data(), 16);
    std::memcpy(out.hash, c.hash.data(), 32);
    out.size = c.size;
    out.type = c.type;
    out.id_offset = c.id_offset;
}

bool Contents(const mergemeta::Cnmt& c, nxo_content** contents, size_t* count) {
    *count = c.contents.size();
    *contents = static_cast<nxo_content*>(std::calloc(c.contents.size() + 1, sizeof(nxo_content)));
    if (!*contents) return false;
    for (size_t i = 0; i < c.contents.size(); i++) Put(c.contents[i], (*contents)[i]);
    return true;
}

// The CNMT in a decrypted meta section.
bool ReadCnmt(const uint8_t* header, const uint8_t* section, size_t len, mergemeta::Cnmt& out, std::string& error) {
    if (std::memcmp(header + 0x200, "NCA3", 4) != 0) {
        error = "not a meta NCA";
        return false;
    }
    std::vector<uint8_t> s(section, section + len), raw;
    if (!mergemeta::CnmtFromSection(s, header + 0x400, raw)) {
        error = "no CNMT in the meta NCA";
        return false;
    }
    if (!mergemeta::ParseCnmt(raw.data(), raw.size(), out)) {
        error = "a CNMT not read";
        return false;
    }
    return true;
}

} // namespace

struct nxo_merger {
    uintptr_t handle = 0;
    mergegame::Merger merger;
};

struct nxo_romfs {
    std::unique_ptr<patchview::Section> base, patched;
    patchview::Romfs romfs;
};

extern "C" {

nxo_merger* nxo_merger_new(uintptr_t handle) {
    auto* m = new (std::nothrow) nxo_merger;
    if (m) m->handle = handle;
    return m;
}

int nxo_merger_open(nxo_merger* m, const nxo_section* base, const nxo_section* update, int update_index, const uint8_t* update_header,
                    int level, unsigned threads, char* err, size_t errlen) {
    try {
        mergegame::Input in;
        in.base = Section(m->handle, *base);
        in.update = Section(m->handle, *update);
        in.update_index = update_index;
        in.update_header.assign(update_header, update_header + 0xC00);
        in.update_raw = in.update.raw;
        in.hash = Sha256;
        const uintptr_t handle = m->handle;
        in.encrypt_header = [handle](uint8_t* header, size_t size) { nxoGoHeader(handle, header, size); };
        in.level = level;
        in.threads = threads;
        std::string error;
        if (!m->merger.Open(std::move(in), error)) {
            SetError(err, errlen, error);
            return 0;
        }
        return 1;
    } catch (const std::exception& e) {
        SetError(err, errlen, e.what());
        return 0;
    }
}

int nxo_merger_measure(nxo_merger* m, char* err, size_t errlen) {
    try {
        const uintptr_t handle = m->handle;
        std::string error;
        if (!m->merger.Measure([handle](uint64_t done) { return nxoGoStep(handle, NXO_MEASURE, done) != 0; }, error)) {
            SetError(err, errlen, error);
            return 0;
        }
        return 1;
    } catch (const std::exception& e) {
        SetError(err, errlen, e.what());
        return 0;
    }
}

int nxo_merger_write(nxo_merger* m, char* err, size_t errlen) {
    try {
        const uintptr_t handle = m->handle;
        std::string error;
        const bool ok = m->merger.Write([handle](const uint8_t* d, size_t n) { return nxoGoOut(handle, const_cast<uint8_t*>(d), n) != 0; },
                                        [handle](uint64_t done) { return nxoGoStep(handle, NXO_WRITE, done) != 0; }, error);
        if (!ok) {
            SetError(err, errlen, error);
            return 0;
        }
        return 1;
    } catch (const std::exception& e) {
        SetError(err, errlen, e.what());
        return 0;
    }
}

void nxo_merger_info(nxo_merger* m, nxo_info* out) {
    out->romfs_size = m->merger.RomfsSize();
    out->stored_size = m->merger.StoredSize();
    out->nca_size = m->merger.NcaSize();
    out->runs_again = m->merger.Stats().runs_again;
    out->blocks_again = m->merger.Stats().blocks_again;
    out->old_kept = m->merger.Stats().old_kept;
    out->damaged = m->merger.Damaged();
}

void nxo_merger_free(nxo_merger* m) { delete m; }

nxo_romfs* nxo_romfs_open(uintptr_t handle, const nxo_section* base, const nxo_section* update, char* err, size_t errlen) {
    try {
        auto r = std::make_unique<nxo_romfs>();
        std::string error;
        r->base = patchview::OpenBase(Section(handle, *base), error);
        patchview::Section* top = r->base.get();
        const uint8_t* fsh = base->fsh;
        if (top && update) {
            r->patched = patchview::OpenPatched(Section(handle, *update), top, error);
            top = r->patched.get();
            fsh = update->fsh;
        }
        if (!top || !r->romfs.Open(top, fsh, Sha256, error)) {
            SetError(err, errlen, error);
            return nullptr;
        }
        return r.release();
    } catch (const std::exception& e) {
        SetError(err, errlen, e.what());
        return nullptr;
    }
}

uint64_t nxo_romfs_size(nxo_romfs* r) { return r->romfs.Size(); }
uint64_t nxo_romfs_stored(nxo_romfs* r) { return r->romfs.StoredSize(); }
int nxo_romfs_compressed(nxo_romfs* r) { return r->romfs.Compressed() ? 1 : 0; }
int nxo_romfs_read(nxo_romfs* r, uint64_t offset, uint8_t* buf, size_t size) {
    try {
        return r->romfs.Read(offset, buf, size) ? 1 : 0;
    } catch (...) {
        return 0;
    }
}
uint64_t nxo_romfs_damaged(nxo_romfs* r) { return r->romfs.Damaged(); }
void nxo_romfs_free(nxo_romfs* r) { delete r; }

int nxo_merged_meta(uintptr_t, const uint8_t* base_header, const uint8_t* base_section, size_t base_len, const uint8_t* update_header,
                    const uint8_t* update_section, size_t update_len, const nxo_content* program, uint8_t* out_header, uint8_t** out_section,
                    size_t* out_len, nxo_content** contents, size_t* count, uint64_t* title_id, uint32_t* version, uint32_t* base_version,
                    char* err, size_t errlen) {
    try {
        std::string error;
        mergemeta::Cnmt bc, uc, mc;
        if (!ReadCnmt(base_header, base_section, base_len, bc, error) || !ReadCnmt(update_header, update_section, update_len, uc, error)) {
            SetError(err, errlen, error);
            return 0;
        }
        mergemeta::Content prog;
        bool found = false;
        for (const auto& c : uc.contents) {
            if (c.type == mergemeta::kContentProgram && c.id_offset == program->id_offset) {
                prog = c;
                found = true;
            }
        }
        if (!found) {
            SetError(err, errlen, "the update has no program");
            return 0;
        }
        std::memcpy(prog.hash.data(), program->hash, 32);
        std::memcpy(prog.id.data(), program->id, 16);
        prog.size = program->size;
        if (!mergemeta::MergedCnmt(bc, uc, prog, mc)) {
            SetError(err, errlen, "not a game and its update");
            return 0;
        }
        std::vector<uint8_t> header, section;
        if (!mergemeta::MetaNca(base_header, mc, Sha256, header, section) || header.size() != 0xC00) {
            SetError(err, errlen, "the meta NCA not made");
            return 0;
        }
        std::memcpy(out_header, header.data(), 0xC00);
        *out_section = static_cast<uint8_t*>(std::malloc(section.size() ? section.size() : 1));
        if (!*out_section || !Contents(mc, contents, count)) {
            std::free(*out_section);
            *out_section = nullptr;
            SetError(err, errlen, "out of memory");
            return 0;
        }
        std::memcpy(*out_section, section.data(), section.size());
        *out_len = section.size();
        *title_id = mc.title_id;
        *version = mc.version;
        *base_version = bc.version;
        return 1;
    } catch (const std::exception& e) {
        SetError(err, errlen, e.what());
        return 0;
    }
}

int nxo_read_cnmt(const uint8_t* header, const uint8_t* section, size_t len, uint64_t* title_id, uint8_t* type, uint32_t* version,
                  nxo_content** contents, size_t* count, char* err, size_t errlen) {
    try {
        std::string error;
        mergemeta::Cnmt c;
        if (!ReadCnmt(header, section, len, c, error)) {
            SetError(err, errlen, error);
            return 0;
        }
        if (!Contents(c, contents, count)) {
            SetError(err, errlen, "out of memory");
            return 0;
        }
        *title_id = c.title_id;
        *type = c.type;
        *version = c.version;
        return 1;
    } catch (const std::exception& e) {
        SetError(err, errlen, e.what());
        return 0;
    }
}

void nxo_free(void* p) { std::free(p); }

} // extern "C"
