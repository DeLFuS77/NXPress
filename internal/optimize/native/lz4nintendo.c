// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 DeLFuS77
//
// The LZ4 blocks of the compressed storage of an NCA are made by two compressors of Nintendo's
// tools, told by the level of each block in its table:
//
//   - level 0, the fast compressor of LZ4 before 1.8.2: a table of 13 bits, the position right
//     after a match tried at once, and matches searched only up to 12 bytes before the end. Its
//     hash is of 5 bytes as built for 64 bits, and of 4 bytes as built for 32 bits: games use
//     either (the second is told as NX_LZ4_LEVEL_FAST4);
//   - levels 1 to 16, LZ4HC of LZ4 r1xx (before 1.7): a hash chain searched 1 << (level-1) times,
//     16 at level 5 and 32,768 at level 16, without the analysis of repeated patterns of 1.8, and
//     the second search of a match starting one byte after the first one.
//
// Written after lz4.c and lz4hc.c of LZ4 (BSD 2-Clause License, Copyright Yann Collet), set as
// those tools set them, and after the compressors of the library app (switchfs/lz4nintendo.go),
// which they match byte for byte (tests/lz4nintendo_test.cpp).
#include "lz4nintendo.h"

#include <string.h>

enum {
    MIN_MATCH = 4,
    MF_LIMIT = 12,
    LAST_LITERALS = 5,
    MIN_LENGTH = MF_LIMIT + 1,
    ML_BITS = 4,
    ML_MASK = 15,
    RUN_MASK = 15,
    MAX_DISTANCE = 65535,
    FAST_HASH_LOG = 13,
    HC_HASH_LOG = 15,
    HC_MAX_D = 1 << 16,
    HC_OPTIMAL_ML = (ML_MASK - 1) + MIN_MATCH,
    // the indices of LZ4HC start at 64 KiB, as LZ4HC_init sets them
    HC_PREFIX = 64 << 10,
};

static uint32_t read32(const uint8_t* p) {
    uint32_t v;
    memcpy(&v, p, 4);
    return v;
}

static uint64_t read64(const uint8_t* p) {
    uint64_t v;
    memcpy(&v, p, 8);
    return v;
}

// The output, which stops being written once it is full.
typedef struct {
    uint8_t* dst;
    size_t cap;
    size_t len;
    int full;
} Out;

static void put(Out* o, uint8_t b) {
    if (o->len >= o->cap) {
        o->full = 1;
        return;
    }
    o->dst[o->len++] = b;
}

static void put_bytes(Out* o, const uint8_t* p, size_t n) {
    if (n > o->cap - o->len) {
        o->full = 1;
        return;
    }
    memcpy(o->dst + o->len, p, n);
    o->len += n;
}

static void add_to_token(Out* o, size_t token, uint8_t v) {
    if (token < o->len) {
        o->dst[token] = (uint8_t)(o->dst[token] + v);
    }
}

// How many bytes from in equal those from match, up to limit; match is before in.
static int64_t count(const uint8_t* src, int64_t in, int64_t match, int64_t limit) {
    const int64_t start = in;
    while (in + 8 <= limit) {
        const uint64_t diff = read64(src + in) ^ read64(src + match);
        if (diff) {
            return in - start + (__builtin_ctzll(diff) >> 3);
        }
        in += 8;
        match += 8;
    }
    while (in < limit && src[in] == src[match]) {
        in++;
        match++;
    }
    return in - start;
}

// A token with n literals and the literals; gives the place of the token.
static size_t literals(Out* o, const uint8_t* lit, size_t n) {
    const size_t token = o->len;
    if (n >= RUN_MASK) {
        put(o, RUN_MASK << ML_BITS);
        for (size_t l = n - RUN_MASK;; l -= 255) {
            if (l < 255) {
                put(o, (uint8_t)l);
                break;
            }
            put(o, 255);
        }
    } else {
        put(o, (uint8_t)(n << ML_BITS));
    }
    put_bytes(o, lit, n);
    return token;
}

size_t nx_lz4_bound(size_t n) {
    return n + n / 255 + 16;
}

int nx_lz4_level_known(int level) {
    return level == 0 || level == NX_LZ4_LEVEL_FAST4 || (level >= 1 && level <= 16);
}

// The hash of the fast compressor on a table of 13 bits: of 5 bytes, or of 4 when wide is 0.
static uint32_t fast_hash(const uint8_t* src, int64_t n, int64_t i, int wide) {
    if (!wide) {
        return (uint32_t)(read32(src + i) * 2654435761u) >> (32 - FAST_HASH_LOG);
    }
    uint64_t v;
    if (i + 8 <= n) {
        v = read64(src + i);
    } else {
        uint8_t tail[8] = {0};
        memcpy(tail, src + i, (size_t)(n - i));
        v = read64(tail);
    }
    return (uint32_t)(((v << 24) * 889523592379ull) >> (64 - FAST_HASH_LOG));
}

// LZ4_compress_generic of LZ4 before 1.8.2, with no dictionary, with the hash of 5 bytes when
// wide is set.
static void compress_fast(const uint8_t* src, int64_t n, Out* o, int32_t* table, int wide) {
    memset(table, 0, sizeof(int32_t) << FAST_HASH_LOG);
    int64_t anchor = 0, ip = 0;
    // matches are searched up to 12 bytes before the end (one byte less than LZ4 1.8.2 and later)
    const int64_t mflimit = n - MF_LIMIT;
    const int64_t match_limit = n - LAST_LITERALS;
    if (n < MIN_LENGTH) {
        literals(o, src, (size_t)n);
        return;
    }
    table[fast_hash(src, n, ip, wide)] = (int32_t)ip;
    ip++;
    uint32_t forward_h = fast_hash(src, n, ip, wide);
    for (;;) {
        int64_t match;
        int64_t forward_ip = ip;
        int64_t step = 1;
        int64_t search_match_nb = 1 << 6;
        for (;;) {
            const uint32_t h = forward_h;
            const int64_t current = forward_ip;
            const int64_t match_index = table[h];
            ip = forward_ip;
            forward_ip += step;
            step = search_match_nb >> 6;
            search_match_nb++;
            if (forward_ip > mflimit) {
                literals(o, src + anchor, (size_t)(n - anchor));
                return;
            }
            match = match_index;
            forward_h = fast_hash(src, n, forward_ip, wide);
            table[h] = (int32_t)current;
            if (match_index + MAX_DISTANCE < current) {
                continue;
            }
            if (match < ip && read32(src + match) == read32(src + ip)) {
                break;
            }
        }
        // catch up
        while (ip > anchor && match > 0 && src[ip - 1] == src[match - 1]) {
            ip--;
            match--;
        }
        size_t token = literals(o, src + anchor, (size_t)(ip - anchor));
        for (;;) {
            const int64_t offset = ip - match;
            put(o, (uint8_t)offset);
            put(o, (uint8_t)(offset >> 8));
            int64_t match_code = count(src, ip + MIN_MATCH, match + MIN_MATCH, match_limit);
            ip += match_code + MIN_MATCH;
            if (match_code >= ML_MASK) {
                add_to_token(o, token, ML_MASK);
                match_code -= ML_MASK;
                for (; match_code >= 255; match_code -= 255) {
                    put(o, 255);
                }
                put(o, (uint8_t)match_code);
            } else {
                add_to_token(o, token, (uint8_t)match_code);
            }
            anchor = ip;
            if (ip > mflimit) {
                literals(o, src + anchor, (size_t)(n - anchor));
                return;
            }
            table[fast_hash(src, n, ip - 2, wide)] = (int32_t)(ip - 2);
            // the position right after the match
            const uint32_t h = fast_hash(src, n, ip, wide);
            const int64_t match_index = table[h];
            table[h] = (int32_t)ip;
            if (match_index + MAX_DISTANCE >= ip && match_index < ip && read32(src + match_index) == read32(src + ip)) {
                match = match_index;
                token = o->len;
                put(o, 0);
                continue;
            }
            break;
        }
        ip++;
        forward_h = fast_hash(src, n, ip, wide);
    }
}

// The hash chain of LZ4HC.
typedef struct {
    const uint8_t* src;
    int64_t n;
    uint32_t* hash_table;
    uint16_t* chain;
    uint32_t next_to_update;
    int64_t attempts;
    // set if a search would read outside the block: it cannot be one of Nintendo's
    int broken;
} Hc;

static uint32_t hc_hash(const Hc* s, uint32_t idx) {
    return (read32(s->src + (idx - HC_PREFIX)) * 2654435761u) >> (32 - HC_HASH_LOG);
}

static void hc_insert(Hc* s, uint32_t target) {
    for (uint32_t idx = s->next_to_update; idx < target; idx++) {
        const uint32_t h = hc_hash(s, idx);
        uint32_t delta = idx - s->hash_table[h];
        if (delta > MAX_DISTANCE) {
            delta = MAX_DISTANCE;
        }
        s->chain[idx & (HC_MAX_D - 1)] = (uint16_t)delta;
        s->hash_table[h] = idx;
    }
    s->next_to_update = target;
}

// LZ4HC_InsertAndGetWiderMatch: from ip, a match longer than longest that may start back down to
// i_low. Gives its length, and sets where it is and where it starts (-1 when none is longer).
static int64_t hc_wider_match(Hc* s, int64_t ip, int64_t i_low, int64_t i_high, int64_t longest, int64_t* match_pos, int64_t* start_pos) {
    const uint8_t* src = s->src;
    const uint32_t ip_idx = (uint32_t)(ip + HC_PREFIX);
    uint32_t low_limit = HC_PREFIX;
    if ((uint32_t)HC_PREFIX + (64u << 10) <= ip_idx) {
        low_limit = ip_idx - MAX_DISTANCE;
    }
    const int64_t delta = ip - i_low;
    *match_pos = -1;
    *start_pos = -1;
    if (ip + MIN_MATCH > s->n || i_low + longest >= s->n || i_low + longest < 0) {
        s->broken = 1;
        return longest;
    }
    hc_insert(s, ip_idx);
    uint32_t match_index = s->hash_table[hc_hash(s, ip_idx)];
    const uint32_t head = read32(src + ip);
    uint8_t last = src[i_low + longest];
    for (int64_t nb = s->attempts; match_index >= low_limit && nb > 0; nb--) {
        const int64_t m = (int64_t)match_index - HC_PREFIX;
        const int64_t probe = m - delta + longest;
        if (probe < 0 || probe >= s->n) {
            s->broken = 1;
            return longest;
        }
        if (last == src[probe] && read32(src + m) == head) {
            int64_t mlt = MIN_MATCH + count(src, ip + MIN_MATCH, m + MIN_MATCH, i_high);
            int64_t back = 0;
            while (ip + back > i_low && m + back > 0 && src[ip + back - 1] == src[m + back - 1]) {
                back--;
            }
            mlt -= back;
            if (mlt > longest) {
                longest = mlt;
                *match_pos = m + back;
                *start_pos = ip + back;
                if (i_low + longest >= s->n) {
                    s->broken = 1;
                    return longest;
                }
                last = src[i_low + longest];
            }
        }
        match_index -= s->chain[(uint16_t)match_index];
    }
    return longest;
}

static void hc_encode(Out* o, const uint8_t* src, int64_t* ip, int64_t* anchor, int64_t ml, int64_t ref) {
    const size_t token = literals(o, src + *anchor, (size_t)(*ip - *anchor));
    const int64_t offset = *ip - ref;
    put(o, (uint8_t)offset);
    put(o, (uint8_t)(offset >> 8));
    int64_t l = ml - MIN_MATCH;
    if (l >= ML_MASK) {
        add_to_token(o, token, ML_MASK);
        l -= ML_MASK;
        for (; l >= 510; l -= 510) {
            put(o, 255);
            put(o, 255);
        }
        if (l >= 255) {
            l -= 255;
            put(o, 255);
        }
        put(o, (uint8_t)l);
    } else {
        add_to_token(o, token, (uint8_t)l);
    }
    *ip += ml;
    *anchor = *ip;
}

static int64_t min64(int64_t a, int64_t b) {
    return a < b ? a : b;
}

// LZ4HC_compress_generic of LZ4 r1xx, whose levels are all of a hash chain, searching it
// attempts times. Gives 0, or -1 when a search would read outside the block.
static int compress_hc(const uint8_t* src, int64_t n, Out* o, void* work, int64_t attempts) {
    Hc s;
    s.src = src;
    s.n = n;
    s.hash_table = (uint32_t*)work;
    s.chain = (uint16_t*)((uint8_t*)work + ((size_t)4 << HC_HASH_LOG));
    s.next_to_update = HC_PREFIX;
    s.attempts = attempts;
    s.broken = 0;
    memset(s.hash_table, 0, (size_t)4 << HC_HASH_LOG);
    memset(s.chain, 0xFF, (size_t)2 * HC_MAX_D);

    int64_t ip = 0, anchor = 0;
    const int64_t mflimit = n - MF_LIMIT;
    const int64_t match_limit = n - LAST_LITERALS;
    if (n >= MIN_LENGTH) {
        int64_t ml, ml2 = 0, ml3 = 0, ml0;
        int64_t ref, start2 = 0, ref2 = 0, start3 = 0, ref3 = 0, start0, ref0;
        int64_t unused;
        while (ip < mflimit) {
            if (s.broken || o->full) {
                return s.broken ? -1 : 0;
            }
            ml = hc_wider_match(&s, ip, ip, match_limit, MIN_MATCH - 1, &ref, &unused);
            if (ml < MIN_MATCH) {
                ip++;
                continue;
            }
            start0 = ip;
            ref0 = ref;
            ml0 = ml;
        search2:
            if (s.broken || o->full) {
                return s.broken ? -1 : 0;
            }
            if (ip + ml < mflimit) {
                // r1xx: the wider match may start one byte after ip at the earliest
                ml2 = hc_wider_match(&s, ip + ml - 2, ip + 1, match_limit, ml, &ref2, &start2);
            } else {
                ml2 = ml;
            }
            if (ml2 == ml) {
                hc_encode(o, src, &ip, &anchor, ml, ref);
                continue;
            }
            if (start0 < ip && start2 < ip + ml0) {
                ip = start0;
                ref = ref0;
                ml = ml0;
            }
            if (start2 - ip < 3) {
                ml = ml2;
                ip = start2;
                ref = ref2;
                goto search2;
            }
        search3:
            if (s.broken || o->full) {
                return s.broken ? -1 : 0;
            }
            if (start2 - ip < HC_OPTIMAL_ML) {
                int64_t new_ml = min64(ml, HC_OPTIMAL_ML);
                if (ip + new_ml > start2 + ml2 - MIN_MATCH) {
                    new_ml = start2 - ip + ml2 - MIN_MATCH;
                }
                const int64_t correction = new_ml - (start2 - ip);
                if (correction > 0) {
                    start2 += correction;
                    ref2 += correction;
                    ml2 -= correction;
                }
            }
            if (start2 + ml2 < mflimit) {
                ml3 = hc_wider_match(&s, start2 + ml2 - 3, start2, match_limit, ml2, &ref3, &start3);
            } else {
                ml3 = ml2;
            }
            if (ml3 == ml2) {
                if (start2 < ip + ml) {
                    ml = start2 - ip;
                }
                hc_encode(o, src, &ip, &anchor, ml, ref);
                ip = start2;
                hc_encode(o, src, &ip, &anchor, ml2, ref2);
                continue;
            }
            if (start3 < ip + ml + 3) {
                if (start3 >= ip + ml) {
                    if (start2 < ip + ml) {
                        const int64_t correction = ip + ml - start2;
                        start2 += correction;
                        ref2 += correction;
                        ml2 -= correction;
                        if (ml2 < MIN_MATCH) {
                            start2 = start3;
                            ref2 = ref3;
                            ml2 = ml3;
                        }
                    }
                    hc_encode(o, src, &ip, &anchor, ml, ref);
                    ip = start3;
                    ref = ref3;
                    ml = ml3;
                    start0 = start2;
                    ref0 = ref2;
                    ml0 = ml2;
                    goto search2;
                }
                start2 = start3;
                ref2 = ref3;
                ml2 = ml3;
                goto search3;
            }
            if (start2 < ip + ml) {
                if (start2 - ip < ML_MASK) {
                    ml = min64(ml, HC_OPTIMAL_ML);
                    if (ip + ml > start2 + ml2 - MIN_MATCH) {
                        ml = start2 - ip + ml2 - MIN_MATCH;
                    }
                    const int64_t correction = ml - (start2 - ip);
                    if (correction > 0) {
                        start2 += correction;
                        ref2 += correction;
                        ml2 -= correction;
                    }
                } else {
                    ml = start2 - ip;
                }
            }
            hc_encode(o, src, &ip, &anchor, ml, ref);
            ip = start2;
            ref = ref2;
            ml = ml2;
            start2 = start3;
            ref2 = ref3;
            ml2 = ml3;
            goto search3;
        }
    }
    if (s.broken) {
        return -1;
    }
    literals(o, src + anchor, (size_t)(n - anchor));
    return 0;
}

int64_t nx_lz4_compress(const uint8_t* src, size_t n, uint8_t* dst, size_t cap, int level, void* work) {
    if (!nx_lz4_level_known(level) || n > ((size_t)1 << 30)) {
        return -1;
    }
    Out o = {dst, cap, 0, 0};
    if (level == 0 || level == NX_LZ4_LEVEL_FAST4) {
        compress_fast(src, (int64_t)n, &o, (int32_t*)work, level == 0);
    } else if (compress_hc(src, (int64_t)n, &o, work, (int64_t)1 << (level - 1)) < 0) {
        return -1;
    }
    if (o.full) {
        return -1;
    }
    return (int64_t)o.len;
}

int nx_lz4_decompress(const uint8_t* src, size_t n, uint8_t* dst, size_t size) {
    size_t i = 0, out = 0;
    while (i < n) {
        const uint8_t token = src[i++];
        size_t lit = token >> 4;
        if (lit == 15) {
            for (;;) {
                if (i >= n) {
                    return -1;
                }
                const uint8_t b = src[i++];
                lit += b;
                if (b != 255) {
                    break;
                }
            }
        }
        if (lit > n - i || lit > size - out) {
            return -1;
        }
        memcpy(dst + out, src + i, lit);
        out += lit;
        i += lit;
        if (i == n) {
            break;
        }
        if (n - i < 2) {
            return -1;
        }
        const size_t offset = (size_t)src[i] | (size_t)src[i + 1] << 8;
        i += 2;
        size_t ml = (size_t)(token & 15) + MIN_MATCH;
        if ((token & 15) == 15) {
            for (;;) {
                if (i >= n) {
                    return -1;
                }
                const uint8_t b = src[i++];
                ml += b;
                if (b != 255) {
                    break;
                }
            }
        }
        if (offset == 0 || offset > out || ml > size - out) {
            return -1;
        }
        const size_t start = out - offset;
        for (size_t k = 0; k < ml; k++) {
            dst[out + k] = dst[start + k];
        }
        out += ml;
    }
    return out == size ? 0 : -1;
}
