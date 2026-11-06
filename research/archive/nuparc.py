"""Archive NUP (prototype, NXPress only): keeps the data that games compressed themselves
decompressed, and makes it again byte for byte on restore.

  nuparc.py pack    game.nup  game.nupa      # .zs of Nintendo (zstd 1.3.x) and zlib streams
  nuparc.py restore game.nupa game.nup       # the NUP back, checked by its SHA-256

Layout of a .nupa: magic NUPA\\x01, u64 length of the index, the index (JSON, zstd), then the
skeleton (the NUP without the rebuilt spans, zstd), the payload of the .zs files and the payload of
the zlib streams (their contents in the order of the NUP, zstd 19 with a 128 MiB window).
"""
import sys, os, json, struct, hashlib, ctypes, zlib, time, threading, glob, io
import concurrent.futures as cf
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import nup, zstandard as zs

HERE = os.path.dirname(os.path.abspath(__file__))
# versions tried for a .zs, the usual ones first; levels in the order to try
ZS_VERSIONS = ["1.3.4", "1.3.5", "1.3.8", "1.4.5"]
ZS_LEVELS = [8, 7, 9, 6, 10, 5, 4, 3, 11, 12, 13, 14, 15, 16, 17, 18, 19, 2, 1]
ZLIB_TRIES = [(6, 8), (9, 8), (1, 8), (9, 9), (5, 8), (4, 8), (7, 8), (8, 8), (3, 8), (2, 8)]
THREADS = 20

_libs = {}
_lock = threading.Lock()


def lib(ver):
    with _lock:
        if ver not in _libs:
            d = ctypes.CDLL(os.path.join(HERE, "zlibs", f"zstd-{ver}.dll"))
            d.ZSTD_compressBound.restype = ctypes.c_size_t
            d.ZSTD_compressBound.argtypes = [ctypes.c_size_t]
            d.ZSTD_compress.restype = ctypes.c_size_t
            d.ZSTD_compress.argtypes = [ctypes.c_void_p, ctypes.c_size_t, ctypes.c_char_p, ctypes.c_size_t, ctypes.c_int]
            d.ZSTD_isError.restype = ctypes.c_uint
            d.ZSTD_isError.argtypes = [ctypes.c_size_t]
            _libs[ver] = d
        return _libs[ver]


def zs_make(raw, ver, level):
    d = lib(ver)
    cap = d.ZSTD_compressBound(len(raw))
    buf = ctypes.create_string_buffer(cap)
    n = d.ZSTD_compress(buf, cap, raw, len(raw), level)
    if d.ZSTD_isError(n):
        return None
    return buf.raw[:n]


def zs_recipe(orig):
    """(version, level, raw) that make the .zs bytes again, or None."""
    try:
        raw = zs.ZstdDecompressor().decompress(orig, max_output_size=1 << 31)
    except zs.ZstdError:
        return None
    for ver in ZS_VERSIONS:
        for level in ZS_LEVELS[:3]:
            if zs_make(raw, ver, level) == orig:
                return ver, level, raw
    # the rest of the levels: up to 12 on the two usual versions; the slow ones only on small files
    for ver in ZS_VERSIONS[:2]:
        for level in ZS_LEVELS[3:]:
            if level > 12 and len(raw) > (256 << 10):
                continue
            if zs_make(raw, ver, level) == orig:
                return ver, level, raw
    return None


def zlib_make(raw, level, mem):
    c = zlib.compressobj(level, zlib.DEFLATED, 15, mem)
    return c.compress(raw) + c.flush()


def zlib_scan(data, base):
    """zlib streams in data that zlib makes again: (absolute offset, length, raw length, level, mem)."""
    out = []
    pos = 0
    n = len(data)
    while pos < n - 2:
        i = data.find(b"\x78", pos)
        if i < 0 or i >= n - 2:
            break
        if data[i + 1] in (0x01, 0x5E, 0x9C, 0xDA) and ((data[i] << 8) | data[i + 1]) % 31 == 0:
            d = zlib.decompressobj()
            try:
                chunk = data[i:i + (8 << 20)]
                raw = d.decompress(chunk)
                if d.eof and len(raw) >= 256:
                    clen = len(chunk) - len(d.unused_data)
                    orig = data[i:i + clen]
                    for level, mem in ZLIB_TRIES:
                        if zlib_make(raw, level, mem) == orig:
                            out.append((base + i, clen, len(raw), level, mem))
                            break
                    pos = i + clen
                    continue
            except zlib.error:
                pass
        pos = i + 1
    return out


def scan_extent(args):
    path, off, size = args
    with open(path, "rb") as f:
        f.seek(off)
        data = f.read(size)
    return zlib_scan(data, off)


def pack(src, dst):
    t0 = time.time()
    n = nup.read(src)
    total = os.path.getsize(src)
    uniq = {}
    for e in n["entries"]:
        if e.codec == 0 and e.size > 0:
            uniq.setdefault(e.off, e)
    zs_items = sorted((e for e in uniq.values() if e.path.endswith(".zs") and e.size > 16), key=lambda e: e.off)
    other = sorted((e for e in uniq.values() if not e.path.endswith(".zs") and e.size >= 4096
                    and nup.ext_of(e.path) not in (".bwav", ".webm")), key=lambda e: e.off)
    f = open(src, "rb")

    # .zs: recipe of each file, on threads (ctypes and zstandard let go of the GIL)
    def zs_job(e):
        with open(src, "rb") as g:
            g.seek(e.off)
            r = zs_recipe(g.read(e.size))
        return (e.off, e.size, len(r[2]), r[0], r[1]) if r else None

    zs_recs = []
    with cf.ThreadPoolExecutor(THREADS) as ex:
        for k, r in enumerate(ex.map(zs_job, zs_items, chunksize=16)):
            if r:
                zs_recs.append(r)
            if k % 5000 == 4999:
                print(f"  .zs {k+1}/{len(zs_items)} ({time.time()-t0:.0f}s)", flush=True)
    t1 = time.time()
    print(f".zs: {len(zs_recs)} of {len(zs_items)} files made again exactly ({time.time()-t0:.0f}s)")

    # zlib streams inside the other files, in processes (the scan is Python)
    import multiprocessing as mp
    jobs = []
    for e in other:
        for s in range(0, e.size, 64 << 20):  # pieces of 64 MiB with 8 MiB more, streams found once
            jobs.append((src, e.off + s, min(e.size - s, (64 << 20) + (8 << 20))))
    zl_recs = []
    seen_end = 0
    with mp.Pool(THREADS) as pool:
        for recs in pool.imap(scan_extent, jobs, chunksize=1):
            for r in recs:
                if r[0] >= seen_end:
                    zl_recs.append(r); seen_end = r[0] + r[1]
    zl_recs.sort()
    # keep streams that do not overlap one another or a .zs file
    spans = sorted([(o, l, "zs", i) for i, (o, l, *_ ) in enumerate(zs_recs)] +
                   [(o, l, "zl", i) for i, (o, l, *_ ) in enumerate(zl_recs)])
    keep, end = [], 0
    for s in spans:
        if s[0] >= end:
            keep.append(s); end = s[0] + s[1]
    print(f"zlib: {len(zl_recs)} streams, {sum(r[1] for r in zl_recs)/1e6:.0f} MB made again exactly ({time.time()-t1:.0f}s)")

    # skeleton: the NUP without the spans kept
    cpar = zs.ZstdCompressionParameters.from_level(19, window_log=27, enable_ldm=True, threads=THREADS)
    out = open(dst, "wb")
    out.write(b"NUPA\x01" + b"\0" * 8)
    sections = {}

    def section(name, writer_fn, params):
        start = out.tell()
        cctx = zs.ZstdCompressor(compression_params=params)
        with cctx.stream_writer(out, closefd=False) as w:
            writer_fn(w)
        sections[name] = (start, out.tell() - start)

    def skeleton(w):
        pos = 0
        for (o, l, _, _) in keep:
            f.seek(pos)
            left = o - pos
            while left:
                b = f.read(min(left, 16 << 20)); w.write(b); left -= len(b)
            pos = o + l
        f.seek(pos)
        while True:
            b = f.read(16 << 20)
            if not b:
                break
            w.write(b)

    def payload(kind):
        def run(w):
            D = zs.ZstdDecompressor()
            for (o, l, k, i) in keep:
                if k != kind:
                    continue
                f.seek(o)
                b = f.read(l)
                w.write(D.decompress(b, max_output_size=1 << 31) if kind == "zs" else zlib.decompress(b))
        return run

    skel_params = zs.ZstdCompressionParameters.from_level(9, window_log=27, enable_ldm=True, threads=THREADS)
    section("skeleton", skeleton, skel_params)
    t2 = time.time()
    section("zs", payload("zs"), cpar)
    section("zl", payload("zl"), cpar)
    sha = hashlib.sha256()
    f.seek(0)
    while True:
        b = f.read(64 << 20)
        if not b:
            break
        sha.update(b)
    index = {"size": total, "sha256": sha.hexdigest(), "sections": sections,
             "spans": [[o, l, k, (zs_recs[i][2:] if k == "zs" else zl_recs[i][2:])] for (o, l, k, i) in keep]}
    blob = zs.ZstdCompressor(level=19).compress(json.dumps(index).encode())
    at = out.tell()
    out.write(blob)
    out.seek(5)
    out.write(struct.pack("<Q", at))  # where the index is
    out.close()
    size = os.path.getsize(dst)
    zs_bytes = sum(l for (o, l, k, i) in keep if k == "zs")
    zl_bytes = sum(l for (o, l, k, i) in keep if k == "zl")
    print(f"sections MB: skeleton {sections['skeleton'][1]/1e6:.1f}, .zs {sections['zs'][1]/1e6:.1f} (were {zs_bytes/1e6:.1f}),"
          f" zlib {sections['zl'][1]/1e6:.1f} (were {zl_bytes/1e6:.1f}), index {len(blob)/1e6:.2f}")
    print(f"NUP {total/1e9:.3f} GB -> archive {size/1e9:.3f} GB ({100*(1-size/total):.1f}% smaller) in {time.time()-t0:.0f}s")


def restore(src, dst):
    t0 = time.time()
    f = open(src, "rb")
    assert f.read(5) == b"NUPA\x01"
    (at,) = struct.unpack("<Q", f.read(8))
    f.seek(at)
    index = json.loads(zs.ZstdDecompressor().decompress(f.read(), max_output_size=1 << 30))
    D = zs.ZstdDecompressor()

    def reader(name):
        start, length = index["sections"][name]
        f2 = open(src, "rb")
        f2.seek(start)
        # a decompressor of its own: two readers on one share its context
        return zs.ZstdDecompressor().stream_reader(io.BufferedReader(_Limited(f2, length), 1 << 20), read_across_frames=True)

    skel, zsr, zlr = reader("skeleton"), reader("zs"), reader("zl")

    def take(r, n):
        parts = []
        while n:
            b = r.read(min(n, 16 << 20))
            if not b:
                raise IOError("archive ends early")
            parts.append(b); n -= len(b)
        return b"".join(parts)

    def make(span, raw):
        o, l, k, rec = span
        if k == "zs":
            out = zs_make(raw, rec[1], rec[2])
        else:
            out = zlib_make(raw, rec[1], rec[2])
        if out is None or len(out) != l:
            raise IOError(f"span at {o} is not made again")
        return out

    sha = hashlib.sha256()
    out = open(dst, "wb")
    pos = 0
    pending = []
    with cf.ThreadPoolExecutor(THREADS) as ex:
        def flush(limit):
            while len(pending) > limit:
                gap, fut = pending.pop(0)
                out.write(gap); sha.update(gap)
                b = fut.result(); out.write(b); sha.update(b)
        for span in index["spans"]:
            o, l, k, rec = span
            gap = take(skel, o - pos)
            raw = take(zsr if k == "zs" else zlr, rec[0])
            pending.append((gap, ex.submit(make, span, raw)))
            pos = o + l
            flush(256)
        flush(0)
    while True:
        b = skel.read(16 << 20)
        if not b:
            break
        out.write(b); sha.update(b)
    out.close()
    ok = sha.hexdigest() == index["sha256"] and os.path.getsize(dst) == index["size"]
    print(f"restored {index['size']/1e9:.3f} GB in {time.time()-t0:.0f}s: SHA-256 {'matches' if ok else 'DOES NOT MATCH'}")
    return ok


class _Limited(io.RawIOBase):
    def __init__(self, f, n):
        self.f, self.n = f, n

    def readable(self):
        return True

    def readinto(self, b):
        if self.n <= 0:
            return 0
        k = self.f.readinto(memoryview(b)[:min(len(b), self.n)])
        self.n -= k
        return k


if __name__ == "__main__":
    cmd, a, b = sys.argv[1:4]
    if cmd == "pack":
        pack(a, b)
    else:
        sys.exit(0 if restore(a, b) else 1)
