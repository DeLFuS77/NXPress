"""Code the DSP-ADPCM frames of every BWAV of a NUP with adpcm.dll, check they come back byte for
byte, and compare the size with how the NUP stores the BWAV files today."""
import sys, os, struct, ctypes, time, concurrent.futures as cf
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import nup, zstandard as zs

HERE = os.path.dirname(os.path.abspath(__file__))
dll = ctypes.CDLL(os.path.join(HERE, "adpcm.dll"))
dll.adpcm_encode.restype = ctypes.c_size_t
dll.adpcm_encode.argtypes = [ctypes.c_char_p, ctypes.c_size_t, ctypes.c_void_p, ctypes.c_size_t]
dll.adpcm_decode.restype = ctypes.c_int
dll.adpcm_decode.argtypes = [ctypes.c_char_p, ctypes.c_size_t, ctypes.c_void_p, ctypes.c_size_t]

path = sys.argv[1]
streams_n = int(sys.argv[2]) if len(sys.argv) > 2 else 16
budget = int(sys.argv[3]) if len(sys.argv) > 3 else 1 << 62
n = nup.read(path)
uniq = {}
for e in n["entries"]:
    if e.path.endswith(".bwav"):
        uniq.setdefault((e.off, e.esize), e)
items = sorted(uniq.values(), key=lambda e: e.off)
f = open(path, "rb")
D = zs.ZstdDecompressor()
frames_parts, rest_parts = [], []
stored_now = 0
t0 = time.time()
got = 0
for e in items:
    if got > budget:
        break
    f.seek(e.off)
    b = f.read(e.esize)
    got += e.size
    stored_now += e.esize
    if e.codec == 1:
        b = D.decompress(b, max_output_size=e.size)
    regions = []
    if b[:4] == b"BWAV":
        ch = struct.unpack_from("<H", b, 0x0E)[0]
        for c in range(ch):
            ci = 0x10 + c * 0x4C
            if struct.unpack_from("<H", b, ci)[0] != 1:
                continue
            nsamp, = struct.unpack_from("<I", b, ci + 0x08)
            off, = struct.unpack_from("<I", b, ci + 0x30)
            ln = min((nsamp + 13) // 14 * 8, len(b) - off) // 8 * 8
            if ln > 0:
                regions.append((off, ln))
    regions.sort()
    pos = 0
    for off, ln in regions:
        if off < pos:
            continue
        rest_parts.append(b[pos:off]); frames_parts.append(b[off:off + ln]); pos = off + ln
    rest_parts.append(b[pos:])
frames = b"".join(frames_parts)
rest = b"".join(rest_parts)
print(f"{len(items)} BWAV, {(len(frames)+len(rest))/1e6:.0f} MB: ADPCM frames {len(frames)/1e6:.0f} MB, the rest {len(rest)/1e6:.1f} MB; "
      f"stored today {stored_now/1e6:.0f} MB  (read in {time.time()-t0:.0f}s)")

# streams of about equal size, cut at frame boundaries
nfr = len(frames) // 8
cuts = [round(i * nfr / streams_n) * 8 for i in range(streams_n + 1)]


def run(i):
    chunk = frames[cuts[i]:cuts[i + 1]]
    k = len(chunk) // 8
    out = ctypes.create_string_buffer(len(chunk) + 4096)
    t = time.perf_counter()
    size = dll.adpcm_encode(chunk, k, out, len(out))
    te = time.perf_counter() - t
    enc = out.raw[:size]
    back = ctypes.create_string_buffer(len(chunk))
    t = time.perf_counter()
    ok = dll.adpcm_decode(enc, len(enc), back, k) == 1 and back.raw == chunk
    td = time.perf_counter() - t
    return size, ok, te, td, len(chunk)


t = time.time()
with cf.ThreadPoolExecutor(streams_n) as ex:
    res = list(ex.map(run, range(streams_n)))
enc = sum(r[0] for r in res)
ok = all(r[1] for r in res)
speed_e = sum(r[4] for r in res) / sum(r[2] for r in res) / 1e6
speed_d = sum(r[4] for r in res) / sum(r[3] for r in res) / 1e6
rest_z = len(zs.ZstdCompressor(level=19, threads=16).compress(rest))
frames_z = len(zs.ZstdCompressor(level=19, threads=16).compress(frames))
print(f"coded frames {enc/1e6:.0f} MB = {100*enc/len(frames):.1f}% of them (zstd 19 solid: {100*frames_z/len(frames):.1f}%), all back byte for byte: {ok}")
print(f"speed per core: coding {speed_e:.0f} MB/s, decoding {speed_d:.0f} MB/s; wall {time.time()-t:.0f}s on {streams_n} threads")
total = enc + rest_z
print(f"BWAV in an archive: {total/1e6:.0f} MB vs {stored_now/1e6:.0f} MB in the NUP today: {100*(1-total/stored_now):.1f}% smaller, {(stored_now-total)/1e6:.0f} MB less")
