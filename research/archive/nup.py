"""Minimal read-only NUP v1 reader for measurements (SPEC 1.0-draft)."""
import struct, os

KINDS = {0: "base", 1: "update", 2: "addon", 3: "addon_update", 4: "system"}


class Entry:
    __slots__ = ("path", "size", "off", "esize", "sha", "codec", "pkg", "kind", "version")

    def __init__(self, **kw):
        for k, v in kw.items():
            setattr(self, k, v)


def read(path):
    f = open(path, "rb")
    sb = f.read(0x80)
    assert sb[:4] == b"NUP\x01", "not a NUP"
    toc_off, toc_size = struct.unpack_from("<QQ", sb, 8)
    blob_off, file_size, pkg_count = struct.unpack_from("<QQI", sb, 0x38)
    f.seek(toc_off)
    toc = f.read(toc_size)
    magic, pc, dir_count, file_count, sp_off, sp_size = struct.unpack_from("<IIQQQQ", toc, 0)
    p = 0x30
    pkgs = []
    for i in range(pc):
        tid, root, exe, rom, ver, kind, flags = struct.unpack_from("<QIIIIBB", toc, p)
        meta_off, meta_size = struct.unpack_from("<QI", toc, p + 0x20)
        pkgs.append(dict(tid=tid, root=root, exefs=exe, romfs=rom, version=ver, kind=kind, flags=flags,
                         meta_off=meta_off, meta_size=meta_size))
        p += 0x40
    dirs_off = p
    files_off = dirs_off + dir_count * 0x18
    child_off = files_off + file_count * 0x50
    pool = toc[sp_off:sp_off + sp_size]

    def dname(i):
        no, nl, fc, cc = struct.unpack_from("<IIII", toc, dirs_off + i * 0x18)
        return pool[no:no + nl].decode("utf-8", "replace"), fc, cc

    def fentry(i):
        b = files_off + i * 0x50
        no, nl, size, eo, es = struct.unpack_from("<IIQQQ", toc, b)
        sha = toc[b + 0x20:b + 0x40]
        codec = toc[b + 0x40]
        return pool[no:no + nl].decode("utf-8", "replace"), size, eo, es, sha, codec

    entries = []
    for pi, pk in enumerate(pkgs):
        stack = [(pk["root"], "")]
        seen = set()
        while stack:
            d, prefix = stack.pop()
            if d in seen:
                continue
            seen.add(d)
            name, fc, cc = dname(d)
            for s in range(fc, fc + cc):
                (nid,) = struct.unpack_from("<I", toc, child_off + s * 4)
                if nid < dir_count:
                    cn, _, _ = dname(nid)
                    stack.append((nid, prefix + cn + "/"))
                else:
                    fn, size, eo, es, sha, codec = fentry(nid - dir_count)
                    entries.append(Entry(path=prefix + fn, size=size, off=eo, esize=es, sha=sha, codec=codec,
                                         pkg=pi, kind=KINDS.get(pk["kind"], str(pk["kind"])), version=pk["version"]))
    f.close()
    return dict(pkgs=pkgs, entries=entries, blob_off=blob_off, file_size=file_size, toc_size=toc_size)


def ext_of(path):
    base = path.rsplit("/", 1)[-1]
    parts = base.split(".")
    if len(parts) == 1:
        return "(none)"
    if parts[-1] == "zs" and len(parts) > 2:
        return "." + parts[-2] + ".zs"
    return "." + parts[-1]
