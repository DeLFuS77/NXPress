/* Lossless coder for DSP-ADPCM sample data (frames of 8 bytes: a header byte with predictor and
 * scale, then 14 nibbles). Context-modelled binary arithmetic coding (range coder as in LZMA):
 * the header by the previous header, each nibble by the scale and predictor of its frame and the
 * two nibbles before it. The bytes come back exactly; nothing about audio is changed. */
#include <stdint.h>
#include <stdlib.h>
#include <string.h>

#define EXPORT __declspec(dllexport)
#define TOP (1u << 24)
#define PBITS 11
#define MOVE 5

typedef struct {
    uint16_t nib[16 * 16 * 16 * 8][16];
    uint16_t hdr[256][256];
} Model;

static Model* model_new(void) {
    Model* m = (Model*)malloc(sizeof(Model));
    if (!m) return 0;
    uint16_t* p = (uint16_t*)m;
    for (size_t i = 0; i < sizeof(Model) / 2; i++) p[i] = 1u << (PBITS - 1);
    return m;
}

typedef struct {
    uint8_t* out;
    size_t cap, pos;
    uint64_t low;
    uint32_t range;
    uint8_t cache;
    uint64_t cache_size;
    int overflow;
} Enc;

static void shift_low(Enc* e) {
    if ((uint32_t)e->low < 0xFF000000u || (uint32_t)(e->low >> 32) != 0) {
        uint8_t temp = e->cache;
        do {
            if (e->pos < e->cap) e->out[e->pos++] = (uint8_t)(temp + (uint8_t)(e->low >> 32));
            else e->overflow = 1;
            temp = 0xFF;
        } while (--e->cache_size != 0);
        e->cache = (uint8_t)((uint32_t)e->low >> 24);
    }
    e->cache_size++;
    e->low = (uint64_t)((uint32_t)e->low << 8);
}

static inline void enc_bit(Enc* e, uint16_t* p, int bit) {
    uint32_t bound = (e->range >> PBITS) * (*p);
    if (!bit) {
        e->range = bound;
        *p += ((1u << PBITS) - *p) >> MOVE;
    } else {
        e->low += bound;
        e->range -= bound;
        *p -= *p >> MOVE;
    }
    while (e->range < TOP) {
        e->range <<= 8;
        shift_low(e);
    }
}

typedef struct {
    const uint8_t* in;
    size_t len, pos;
    uint32_t range, code;
} Dec;

static inline uint8_t next(Dec* d) {
    return d->pos < d->len ? d->in[d->pos++] : 0;
}

static inline int dec_bit(Dec* d, uint16_t* p) {
    uint32_t bound = (d->range >> PBITS) * (*p);
    int bit;
    if (d->code < bound) {
        d->range = bound;
        *p += ((1u << PBITS) - *p) >> MOVE;
        bit = 0;
    } else {
        d->code -= bound;
        d->range -= bound;
        *p -= *p >> MOVE;
        bit = 1;
    }
    while (d->range < TOP) {
        d->range <<= 8;
        d->code = (d->code << 8) | next(d);
    }
    return bit;
}

/* Codes nframes frames of 8 bytes; gives the bytes written, 0 when out is too small. */
EXPORT size_t adpcm_encode(const uint8_t* data, size_t nframes, uint8_t* out, size_t cap) {
    Model* m = model_new();
    if (!m) return 0;
    Enc e = {out, cap, 0, 0, 0xFFFFFFFFu, 0, 1, 0};
    unsigned prevh = 0, n1 = 0, n2 = 0;
    for (size_t f = 0; f < nframes; f++) {
        const uint8_t* fr = data + f * 8;
        unsigned h = fr[0], node = 1;
        for (int i = 7; i >= 0; i--) {
            int b = (h >> i) & 1;
            enc_bit(&e, &m->hdr[prevh][node], b);
            node = node * 2 + b;
        }
        unsigned base = ((h & 15) * 256) * 8 + ((h >> 4) & 7);
        for (int k = 0; k < 14; k++) {
            unsigned nib = (k & 1) ? (fr[1 + k / 2] & 15) : (fr[1 + k / 2] >> 4);
            uint16_t* probs = m->nib[base + (n1 * 16 + n2) * 8];
            unsigned nd = 1;
            for (int i = 3; i >= 0; i--) {
                int b = (nib >> i) & 1;
                enc_bit(&e, &probs[nd], b);
                nd = nd * 2 + b;
            }
            n2 = n1;
            n1 = nib;
        }
        prevh = h;
    }
    for (int i = 0; i < 5; i++) shift_low(&e);
    free(m);
    return e.overflow ? 0 : e.pos;
}

/* Gives back nframes frames of 8 bytes from what adpcm_encode wrote; 1 when it went right. */
EXPORT int adpcm_decode(const uint8_t* in, size_t len, uint8_t* data, size_t nframes) {
    Model* m = model_new();
    if (!m) return 0;
    Dec d = {in, len, 0, 0xFFFFFFFFu, 0};
    for (int i = 0; i < 5; i++) d.code = (d.code << 8) | next(&d);
    unsigned prevh = 0, n1 = 0, n2 = 0;
    for (size_t f = 0; f < nframes; f++) {
        uint8_t* fr = data + f * 8;
        unsigned node = 1;
        for (int i = 0; i < 8; i++) node = node * 2 + dec_bit(&d, &m->hdr[prevh][node]);
        unsigned h = node & 0xFF;
        fr[0] = (uint8_t)h;
        unsigned base = ((h & 15) * 256) * 8 + ((h >> 4) & 7);
        for (int k = 0; k < 14; k++) {
            uint16_t* probs = m->nib[base + (n1 * 16 + n2) * 8];
            unsigned nd = 1;
            for (int i = 0; i < 4; i++) nd = nd * 2 + dec_bit(&d, &probs[nd]);
            unsigned nib = nd & 15;
            if (k & 1) fr[1 + k / 2] |= (uint8_t)nib;
            else fr[1 + k / 2] = (uint8_t)(nib << 4);
            n2 = n1;
            n1 = nib;
        }
        prevh = h;
    }
    free(m);
    return 1;
}
