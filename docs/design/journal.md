# The journal

The journal is the community's record of everything that happened. It is a set of text files that only grow, where each entry is sealed to the one before it. The indexes that screens and search use are derived from the journal and can be rebuilt from it at any time.

The code is in [`core/journal`](../../core/journal).

## Why a journal

A community server runs on ordinary hardware, with nobody on call. Power goes out, and cheap disks claim to have saved data they have not saved. Keeping one append-only record as the single source of truth turns four hard problems into one simple habit:

- **Backup and replication.** Copying the new entries is the backup. A second machine that receives them is the replica.
- **Audit.** Each entry carries the seal of the previous one, so a change in the past shows up as a broken chain, as long as someone keeps a recent seal somewhere else.
- **Recovery.** If an index is lost or damaged, it is deleted and rebuilt from the journal.
- **Upgrades.** A new version that changes an index does not migrate data. It builds the new index from the journal, and the old one stays untouched until the new one is ready.

The price is discipline: every change to the community's data has to become an entry.

## Format

A journal is a folder of segment files. Each segment is a text file named after the number of its first entry, with twenty digits, such as `00000000000000000001.journal`. A new segment starts when the current one reaches 64 MB.

Each line is one entry, with four fields separated by a tab and ended by a newline:

```
<number> <seal of the previous entry> <digest of the content> <content>
```

| Field | Form |
| --- | --- |
| Number | Decimal, starting at 1, with no gaps |
| Seal of the previous entry | 64 lowercase hex characters. Entry 1 carries 64 zeros |
| Digest | SHA-256 of the content bytes, as 64 lowercase hex characters |
| Content | One JSON object on a single line, up to 1 MB |

The seal of an entry is the SHA-256 of its first three fields exactly as written, tabs included and without the content:

```
seal = SHA-256("<number>\t<seal of the previous entry>\t<digest>")
```

Because the seal covers the digest and not the content, a content can be erased without breaking the chain. An erased entry keeps its number, its seals and its digest, and its content becomes a single `-`.

The number of an entry is also its identity. A message, an account or a proposal is known by the number of the entry that created it.

The format can be checked with ordinary tools. This prints the seal of the first entry of a journal:

```bash
head -n 1 00000000000000000001.journal | cut -f 1-3 | tr -d '\n' | shasum -a 256
```

## Writing

There is a single writer. `Append` writes all the entries of one call together and returns only after the disk confirmed the write. An entry that `Append` reported is never lost. An entry that was being written when the power went out may or may not be there afterwards, and the caller never heard that it was saved.

If writing or confirming fails, the journal stops accepting writes until it is opened again. It never tries to confirm the same file a second time, because after a failed confirmation nobody knows what is on the disk.

When a segment file is created, the folder is confirmed too, so the new file name survives a power cut. A lock on the folder keeps a second process from opening the same journal.

## Opening after a power cut

`Open` reads the last segment from its first line and checks every entry: its shape, its digest, its number and its tie to the entry before.

- An unfinished line at the end of the file is cut. That is what a write interrupted halfway leaves behind, and it was never confirmed.
- A last segment with no entries is removed. That is what a power cut right after creating a segment leaves behind.
- A complete line that fails any check is damage. `Open` returns an error that names the file, the entry and the position, and changes nothing. A damaged entry is never repaired by guessing: it comes back from a copy.

`Verify` does the same for the whole journal, across all segments, and is meant to run from time to time in the background.

## What the tests cover

- The last line cut at every single byte: the journal opens, ends at the previous entry and accepts the same entry again, producing the same bytes.
- A tail of zeros, short and very long, which is what a file looks like when its size was saved but its data was not.
- Damage in the middle: a changed content, a changed header, a removed line, swapped lines, a repeated line and a garbage line. Each is reported at the right entry, and the files are left as they were.
- Damage in an older segment and a missing segment, both found by `Verify`.
- A failed confirmation, after which the journal refuses to write.
- A writer killed at a random moment, many times over the same journal, with every confirmed entry still there afterwards.

## What is not proven yet

- **A real power cut.** Killing a program is not the same as cutting the power, because the operating system still writes what the program handed over. Disks also write out of order. The proof is to pull the plug on a real machine with a cheap disk, many times, while it writes. The tool for it is [`cmd/journal-proof`](../../cmd/journal-proof).
- **How strict to be after a power cut.** Today, only an unfinished final line is cut, and any other problem stops the journal. If real disks leave other shapes behind in the part that was never confirmed, this rule will need to change, based on what the power cut proof shows.
- **A failed confirmation followed by a restart.** After a failed confirmation the operating system may still hold in memory data that never reached the disk. Reading the file again would then look fine. The safe answer may be to restart the machine, not only the program.

## Erasing

`Erase` replaces the content of given entries with the erased mark. Every segment that holds one of them is read whole, written again into a new file with those contents replaced, confirmed, and moved over the old file, which the file system does in one step. A file left behind by a power cut in the middle of that is removed the next time the journal opens. The writer, if it was appending to that segment, goes on in the new file. An entry that is already erased is left alone, so the same erasure can be asked again after a failure.

- The tests cover: erasing in old segments and in the one being written, the chain verified afterwards, the writer continuing with the right seal, a repeated erasure changing no byte, a leftover file from an interrupted erasure, and the refusal after a failed write.

## Not built yet

- Shipping new entries to a second machine.
- Marking an entry as lost when there is no copy to restore it from.
