# Rebuild (`--rebuild`)

`nxpress --rebuild FILE` writes a package again as a clean NSP, next to it or in `-o`
(`FILE (rebuilt).nsp`; with `-C`, an NSZ). It takes any package of games, updates or DLC: NSP,
NSZ, XCI or XCZ, with one title or several.

It is meant for packages another tool left: NCA rewritten after they were named, a CNMT that
does not match its contents, tickets an installer does not expect. DBI often installs them as
they are; other installers may not. Rebuild writes them the way a package from the eShop is laid
out, without touching the data of the game.

## What it makes right

For every NCA but the meta NCA:

- **Standard crypto.** An NCA with a rights ID gets its title key, from its ticket, into slot 2 of
  its key area, encrypted with the key area key of an application of its key generation. The
  rights ID is cleared and the key area index set to the one of applications. The key of its
  sections and its key generation stay the same, so the sections are not encrypted again.
- **The hashes of its FS headers** in the NCA header are made right.
- **Its size** in the header is the size of the NCA.
- **Distribution type: downloaded.** An NCA marked for a game card is marked as downloaded.
- **Its name** is its content ID: the first 16 bytes of the SHA-256 of the whole NCA as it is now.

For each title:

- **A new CNMT**, with the header and extended header it had, and each content with its new ID,
  SHA-256 and size. Delta fragments are left out: an installed game never uses them.
- **A new meta NCA**, made from the one it had (same key area and crypto): content type Meta,
  downloaded, one section with a PFS0 holding `Application_<id>.cnmt` (or `Patch_`,
  `AddOnContent_`...), hashed in blocks.

For the package:

- **A new PFS0** with the NCA only, the meta NCA last. Tickets, certificates and any other file
  (XML, images) are left out: with standard crypto no ticket is needed.

## What stays as it is

- The data of every section, byte for byte: no compression, no LZ4, no other key.
- The signatures of the NCA headers. A rewritten header is no longer signed by Nintendo, and
  Rebuild does not hide it: **the rebuilt package needs sigpatches**, as the one it came from.
  An NCA that was signed and is only renamed or moved to standard crypto loses its signature too.

## How it is checked

Before the new file gets its name it is opened again from the disk:

- every file in it is an NCA, every content each CNMT lists is there, and every NCA is listed by
  a CNMT;
- no NCA has a rights ID, and the hash of every FS header is right;
- every NCA matches its content ID, and its hash trees are checked where it can be told
  (the check of `-V`): none is MODIFIED and none is checked only in part.

If anything does not match, nothing is written and the original stays as it is. The original is
never deleted (`--rm-source` does not go with `--rebuild`).

## What it needs

- `prod.keys` with the header key, the key area keys and, for packages with tickets, the title
  key encryption keys.
- A build of NXPress with cgo (the releases are).
