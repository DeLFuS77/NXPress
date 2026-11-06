"""Which libzstd version and level re-creates Nintendo's .zs bytes exactly? (one-shot ZSTD_compress)"""
import sys, os, ctypes, random, collections, glob, multiprocessing as mp
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import nup, zstandard as zs

HERE = os.path.dirname(os.path.abspath(__file__))
LIBS = sorted(glob.glob(os.path.join(HERE, "zlibs", "zstd-*.dll")))
LEVELS = list(range(-7, 0)) + list(range(1, 20))

_dlls = None


def libs():
    global _dlls
    if _dlls is None:
        _dlls = []
        for p in LIBS:
            d = ctypes.CDLL(p)
            d.ZSTD_compressBound.restype = ctypes.c_size_t
            d.ZSTD_compressBound.argtypes = [ctypes.c_size_t]
            d.ZSTD_compress.restype = ctypes.c_size_t
            d.ZSTD_compress.argtypes = [ctypes.c_void_p, ctypes.c_size_t, ctypes.c_void_p, ctypes.c_size_t, ctypes.c_int]
            d.ZSTD_isError.restype = ctypes.c_uint
            d.ZSTD_isError.argtypes = [ctypes.c_size_t]
            _dlls.append((os.path.basename(p)[5:-4], d))
    return _dlls


def try_one(args):
    raw, orig, levels = args
    hits = []
    for ver, d in libs():
        cap = d.ZSTD_compressBound(len(raw))
        buf = ctypes.create_string_buffer(cap)
        for L in levels:
            n = d.ZSTD_compress(buf, cap, raw, len(raw), L)
            if d.ZSTD_isError(n):
                continue
            if n == len(orig) and buf.raw[:n] == orig:
                hits.append((ver, L))
    return hits



def analyze(names, res):
    vers = sorted({v for hits in res for v, _ in hits}, key=lambda s: tuple(map(int, s.split("."))))
    print("\nper version: files matched at level 8 / at any level")
    for v in vers:
        l8 = sum(1 for h in res if (v, 8) in h)
        anyl = sum(1 for h in res if any(x == v for x, _ in h))
        print(f"  {v:6} L8 {l8:4}  any {anyl:4}")
    best = max(vers, key=lambda v: sum(1 for h in res if any(x == v for x, _ in h)))
    print("best version", best)
    lv = collections.Counter()
    for name, h in zip(names, res):
        ls = sorted(L for x, L in h if x == best)
        if 8 not in ls:
            lv[(nup.ext_of(name), tuple(ls))] += 1
    print("files of best version not at L8 (ext, levels that match):", lv.most_common(20))


if __name__ == "__main__":
    path = sys.argv[1]
    count = int(sys.argv[2]) if len(sys.argv) > 2 else 200
    maxsize = int(sys.argv[3]) if len(sys.argv) > 3 else 4 << 20
    n = nup.read(path)
    uniq = {}
    for e in n["entries"]:
        if e.path.endswith(".zs") and e.codec == 0 and 16 < e.esize <= maxsize:
            uniq.setdefault((e.off, e.esize), e)
    items = list(uniq.values())
    random.seed(11)
    random.shuffle(items)
    items = items[:count]
    f = open(path, "rb")
    D = zs.ZstdDecompressor()
    jobs = []
    names = []
    for e in items:
        f.seek(e.off)
        c = f.read(e.esize)
        raw = D.decompress(c, max_output_size=max(e.size * 80, 1 << 20))
        jobs.append((raw, c, LEVELS))
        names.append(e.path)
    with mp.Pool(20) as pool:
        res = pool.map(try_one, jobs, chunksize=1)
    tally = collections.Counter()
    none = 0
    by_ext = collections.defaultdict(collections.Counter)
    for name, hits in zip(names, res):
        if not hits:
            none += 1
            by_ext[nup.ext_of(name)]["none"] += 1
        for h in hits:
            tally[h] += 1
        if hits:
            by_ext[nup.ext_of(name)][hits[0]] += 1
    print("files", len(res), "no match", none); analyze(names, res)
    print("matches (version, level):", tally.most_common(40))
    for k, v in sorted(by_ext.items(), key=lambda kv: -sum(kv[1].values()))[:15]:
        print(" ", k, v.most_common(4))

