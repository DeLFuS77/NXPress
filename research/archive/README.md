# Archive transforms (research prototypes, not part of NXPress)

Prototypes for an "archive" mode: data that a game compressed itself is kept decompressed in the
archive and made again byte for byte on restore. No reader but these tools understands the
result: it is for keeping games, and is restored to a normal NUP before playing. Measured on
real games on 06/11/2026; nothing here is wired into NXPress.

## What is made again exactly

| Data | How it is made again | Found on |
|---|---|---|
| `.zs` files of Nintendo | libzstd **1.3.0–1.3.4** (most) or **1.3.5–1.4.5**, one-shot `ZSTD_compress`, level 8 (a few 5–10) | 298 of 300 random files of a Switch 2 game; 1.4.7 and later almost never match |
| zlib streams | zlib level 6, memLevel 8 (the defaults) | 400 of 400 in a Switch 2 game of another publisher; 160,923 streams (1.25 GB) of five of its files |
| DSP-ADPCM audio (`.bwav` codec 1) | not compressed by the game: coded losslessly by `adpcm.c` | every frame of 1.6 GB of `.bwav` came back byte for byte |

## Gains measured

- `.zs`: their content with zstd 19 is 11.6% smaller than Nintendo's files one by one, 14.4–15.1%
  solid by type.
- zlib: 1,255 MB of streams → 994 MB (−21%) as zstd 19 of their content; on one very redundant file
  −68 to −73%; over the whole game the zlib streams are only 7.5% of the bytes.
- DSP-ADPCM: 78.7% of its size with `adpcm.c` (zstd 19 solid 87.2%; a NUP stores it at 95.6%), so
  `.bwav` files take 17.8% less than in a NUP.

## Files

- `nup.py`: a minimal NUP reader (SPEC 1.0-draft).
- `nuparc.py`: `pack game.nup game.nupa` / `restore game.nupa game.nup`, with `.zs` and zlib.
  The restore is checked by the SHA-256 of the whole NUP.
- `zs_versions.py`: finds which version and level of libzstd made each `.zs`.
- `adpcm.c`: the DSP-ADPCM coder (range coder as in LZMA, contexts: previous header; scale,
  predictor and the two previous nibbles). `adpcm_test.py` codes every `.bwav` of a NUP and checks
  it. `adpcm_entropy.py` gives the entropy bounds.
- `zlib_sample.py`: estimates the share of zlib streams and their gain over a whole NUP.

## Building the libraries

The `.zs` recipes need libzstd of each version as a DLL: the official sources from
`https://github.com/facebook/zstd/archive/refs/tags/v<version>.tar.gz`, built with zig:

```
zig cc -O2 -shared -DZSTD_DLL_EXPORT=1 -DZSTD_DISABLE_ASM -DZSTD_MULTITHREAD=0 -Ilib -Ilib/common \
  -o zlibs/zstd-<version>.dll lib/common/*.c lib/compress/*.c lib/decompress/*.c
zig cc -O3 -shared -o adpcm.dll adpcm.c
```

The Python scripts need `zstandard` (and `numpy` for `adpcm_entropy.py`).

## For NXPress, if it goes further

- The old libzstd compressors would be vendored in C with their symbols renamed, as they clash
  with libzstd 1.5.7; only the compressor of 1.3.4 and 1.3.5 is needed.
- The same transforms fit inside NCZ (as NCZSECT3 does for Nintendo's LZ4): `.zs` files and zlib
  streams inside the RomFS of an NCA, so they would reach NSZ libraries of Switch 1 too.
