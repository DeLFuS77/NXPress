"""Writes the message that tells the Discord server about a release, with its notes.

Usage: discord_release.py "<title>" <url of the release> <notes.md> > message.json

The message is an embed with the title linked to the release and the notes of CHANGELOG.md.
Headings become bold lines, notes longer than Discord allows end with a link to all of them,
and no mention in the notes pings anyone.
"""
import json
import re
import sys

# Discord allows 4096 characters in the description of an embed; a margin for how it counts
LIMIT = 4000

title, url, source = sys.argv[1], sys.argv[2], sys.argv[3]
with open(source, encoding="utf-8") as f:
    notes = f.read().strip()

notes = re.sub(r"^#{1,6}[ \t]+(.+?)[ \t]*$", r"**\1**", notes, flags=re.M)

if len(notes) > LIMIT:
    more = f"\n\n… [All the notes on GitHub]({url})"
    room = LIMIT - len(more)
    cut = notes.rfind("\n", 0, room)
    notes = notes[: cut if cut > 0 else room].rstrip() + more

message = {
    "embeds": [{"title": title, "url": url, "description": notes}],
    "allowed_mentions": {"parse": []},
}
# ASCII with \u escapes, the same whatever the encoding of the output
json.dump(message, sys.stdout)
