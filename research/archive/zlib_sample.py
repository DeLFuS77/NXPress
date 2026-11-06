"""Sample windows across a whole NUP: share of bytes in zlib streams that zlib re-creates exactly,
and what zstd makes of their content. Estimates the size of an 'archive' NUP."""
import sys, os, zlib, random, multiprocessing as mp
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import nup, zstandard as zs

WIN = 24 << 20


def scan(args):
    path, off, size = args
    f = open(path, "rb")
    f.seek(off)
    data = f.read(size)
    pos = 0
    zl_bytes = 0; exact_bytes = 0; raws = []; other = bytearray()
    last = 0
    while pos < len(data) - 2:
        i = data.find(b"\x78", pos)
        if i < 0 or i >= len(data) - 2:
            break
        if data[i + 1] in (0x01, 0x5E, 0x9C, 0xDA) and ((data[i] << 8) | data[i + 1]) % 31 == 0:
            d = zlib.decompressobj()
            try:
                chunk = data[i:i + (4 << 20)]
                raw = d.decompress(chunk)
                if d.eof and len(raw) >= 64:
                    clen = len(chunk) - len(d.unused_data)
                    orig = data[i:i + clen]
                    ok = False
                    for lvl, mem in ((6, 8), (9, 8), (1, 8), (9, 9), (5, 8), (4, 8), (7, 8), (8, 8), (3, 8), (2, 8)):
                        c = zlib.compressobj(lvl, zlib.DEFLATED, 15, mem)
                        if c.compress(raw) + c.flush() == orig:
                            ok = True; break
                    other += data[last:i]
                    if ok:
                        exact_bytes += clen; raws.append(raw)
                    else:
                        other += orig
                    zl_bytes += clen
                    pos = last = i + clen
                    continue
            except zlib.error:
                pass
        pos = i + 1
    other += data[last:]
    p = zs.ZstdCompressionParameters.from_level(19, window_log=27, enable_ldm=True)
    raw_all = b"".join(raws)
    z_raw = len(zs.ZstdCompressor(compression_params=p).compress(raw_all)) if raw_all else 0
    z_other = len(zs.ZstdCompressor(level=19).compress(bytes(other))) if other else 0
    return len(data), zl_bytes, exact_bytes, len(raw_all), z_raw, len(other), z_other


if __name__ == "__main__":
    path = sys.argv[1]
    nwin = int(sys.argv[2]) if len(sys.argv) > 2 else 40
    n = nup.read(path)
    uniq = {}
    for e in n["entries"]:
        if e.codec == 0 and e.size > 0:
            uniq.setdefault((e.off, e.esize), e)
    ext_bytes = {}
    for e in uniq.values():
        ext_bytes[nup.ext_of(e.path)] = ext_bytes.get(nup.ext_of(e.path), 0) + e.size
    total = sum(e.size for e in uniq.values())
    stored_total = n["file_size"] - n["blob_off"]
    # windows chosen uniformly over the bytes of raw-stored files
    random.seed(9)
    items = list(uniq.values())
    cum = []
    acc = 0
    for e in items:
        acc += e.size; cum.append(acc)
    import bisect
    jobs = []
    for _ in range(nwin):
        r = random.randrange(total)
        k = bisect.bisect_right(cum, r)
        e = items[k]
        start = max(0, min(e.size - WIN, r - (cum[k] - e.size)))
        jobs.append((path, e.off + start, min(WIN, e.size)))
    with mp.Pool(20) as pool:
        res = pool.map(scan, jobs, chunksize=1)
    S = [sum(r[i] for r in res) for i in range(7)]
    data, zl, exact, raw, zraw, other, zother = S
    print(f"raw-stored bytes in NUP {total/1e9:.2f} GB (of {stored_total/1e9:.2f} GB stored)")
    print(f"sampled {data/1e6:.0f} MB in {nwin} windows: zlib {100*zl/data:.1f}%  re-created exactly {100*exact/data:.1f}%")
    print(f"  exact zlib {exact/1e6:.0f} MB -> content {raw/1e6:.0f} MB -> zstd19 {zraw/1e6:.0f} MB")
    print(f"  rest {other/1e6:.0f} MB -> zstd19 {zother/1e6:.0f} MB")
    ratio = (zraw + zother) / data
    print(f"  archive estimate: {100*ratio:.1f}% of the sampled bytes -> raw-stored part {total*ratio/1e9:.2f} GB "
          f"(was {total/1e9:.2f}); whole NUP ~{(stored_total - total + total*ratio)/1e9:.1f} GB of {stored_total/1e9:.1f}")
