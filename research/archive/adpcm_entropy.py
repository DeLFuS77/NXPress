"""How far below 4 bits/sample can DSP-ADPCM (BWAV codec 1) be coded losslessly? Empirical
conditional entropies of the nibbles, as a context-modelling arithmetic coder would reach."""
import sys, os, struct, random
import numpy as np
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import nup, zstandard as zs


def HC(ctx, sym, nsym):
    j = np.bincount(ctx * nsym + sym)
    c = np.bincount(ctx)
    return H(j) - H(c)


def H(counts):
    c = counts[counts > 0].astype(np.float64)
    p = c / c.sum()
    return -(c * np.log2(p)).sum()  # total bits


path = sys.argv[1]
budget = int(sys.argv[2]) if len(sys.argv) > 2 else 300_000_000
n = nup.read(path)
uniq = {}
for e in n["entries"]:
    if e.path.endswith(".bwav"):
        uniq.setdefault((e.off, e.esize), e)
items = list(uniq.values())
random.seed(3)
random.shuffle(items)
f = open(path, "rb")
D = zs.ZstdDecompressor()
frames_all = []
tot = 0
zstd_stored = 0
for e in items:
    if tot > budget:
        break
    f.seek(e.off)
    b = f.read(e.esize)
    if e.codec == 1:
        b = D.decompress(b, max_output_size=e.size)
    if b[:4] != b"BWAV":
        continue
    ch = struct.unpack_from("<H", b, 0x0E)[0]
    for c in range(ch):
        ci = 0x10 + c * 0x4C
        codec, = struct.unpack_from("<H", b, ci)
        if codec != 1:
            continue
        nsamp, = struct.unpack_from("<I", b, ci + 0x08)
        off, = struct.unpack_from("<I", b, ci + 0x30)
        nfr = (nsamp + 13) // 14
        data = b[off:off + nfr * 8]
        if len(data) < 8:
            continue
        frames_all.append(np.frombuffer(data[: len(data) // 8 * 8], dtype=np.uint8).reshape(-1, 8))
    tot += e.size
    zstd_stored += e.esize
fr = np.concatenate(frames_all)
N = fr.shape[0]
hdr = fr[:, 0]
nib = np.empty((N, 14), dtype=np.uint8)
nib[:, 0::2] = fr[:, 1:] >> 4
nib[:, 1::2] = fr[:, 1:] & 15
scale = hdr & 15
raw_bits = N * 64
print(f"sampled {tot/1e6:.0f} MB of bwav ({len(frames_all)} channels), {N} frames; stored now {zstd_stored/1e6:.0f} MB")
# headers
hb0 = H(np.bincount(hdr, minlength=256))
prev_h = np.concatenate([[0], hdr[:-1]])
hb1 = HC(prev_h.astype(np.int64), hdr, 256)
flat = nib.reshape(-1)
b0 = H(np.bincount(flat, minlength=16))
sc = np.repeat(scale, 14)
b1 = HC(sc.astype(np.int64), flat, 16)
prev = np.concatenate([[8], flat[:-1]])
b2 = HC(sc.astype(np.int64) * 16 + prev, flat, 16)
prev2 = np.concatenate([[8, 8], flat[:-2]])
b3 = HC((sc.astype(np.int64) * 16 + prev) * 16 + prev2, flat, 16)
pos = np.tile(np.arange(14), N)
b4 = HC(((sc.astype(np.int64) * 16 + prev) * 16 + prev2) * 14 + pos, flat, 16)
ph = np.repeat(hdr >> 4, 14).astype(np.int64)
b5 = HC((((sc.astype(np.int64) * 16 + prev) * 16 + prev2) * 14 + pos) * 8 + (ph & 7), flat, 16)
for name, hb, nb in [("order0", hb0, b0), ("ctx scale", hb1, b1), ("scale+prev", hb1, b2),
                     ("scale+prev2", hb1, b3), ("scale+prev2+pos", hb1, b4), ("+predictor", hb1, b5)]:
    bits = hb + nb
    print(f"  {name:18} {bits/8/1e6:8.1f} MB  = {100*bits/raw_bits:5.1f}% of raw  (header {hb/N:.2f} b/frame, nibble {nb/(N*14):.3f} b)")
print(f"  zstd today (per file): {100*zstd_stored/tot:.1f}% of raw")
