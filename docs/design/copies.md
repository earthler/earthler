# Copies

A community server is one machine with one disk, and a dead disk takes everything with it. The copies are how the community survives that. The code is in [`core/copies`](../../core/copies).

## What a copy holds

Only what cannot be rebuilt: the segments of the [journal](journal.md) and the secrets folder, which has the key that signs the session cookies, the certificate chain and the copy key itself. The indexes stay out, because they are rebuilt from the journal. The downloaded collection stays out too, because it comes back from a pen drive or from another community.

## The copy key

Every copy is encrypted with one key, made when the community is born and kept in the secrets folder. The person who looks after the machine writes it down, in groups of four letters and digits, because a copy on a dead machine can only be read back with the key on paper. The text has no letter that looks like a digit, and typing a 0 for an O or a 1 for an I is forgiven.

Today the key is shown by the `-print-copy-key` option of the program. The setup screens, when they exist, show it once on the day the community is born, as the architecture asks.

## The format

Every file of a copy, the manifest included, has the same shape:

```
<32 random bytes of salt> <block 0> <block 1> ... <last block>
```

From the copy key and the salt, a key for that file alone is derived with HKDF over SHA-256. Each block holds up to 4 MB of content and is sealed with AES-256-GCM, with the number of the block as the unique number, and with the name of the file in the copy and a last-block mark as additional data. So a block cannot be changed, moved, repeated or taken from another file, and a file cannot be cut short: the last block says it is the last. A file is refused as a whole when any of that fails, or when the key is not the one.

The files inside a copy have random names. The one file with a fixed name is the manifest, which lists each path of the data folder with its SHA-256 digest, its size and the random name that holds it. The manifest is encrypted like the rest.

## Keeping a copy up to date

`Copy` brings a folder up to date with the data folder: each source file is read, its digest compared with the manifest, and only the files that changed are written, under new random names. Then a new manifest is written, and whatever the manifest no longer names is removed. Each file is written to a temporary name, confirmed to the disk and moved into place, so a power cut leaves the old file or the new one, never half of one.

The last segment of the journal changes every day, so it is copied every day. An erased segment has a new digest and is copied again, which is how an erasure reaches the copies.

A copy goes into a folder that is empty or already holds a copy. A folder that holds other things, such as the root of a pen drive, is refused before anything is written, and a folder that already holds a copy keeps whatever else is in it: only the copy's own files, named at random, are ever removed.

The program makes a copy with `-copy <folder>`, or every night into the folder given with `-copies`, after the night's erasure, so a copy never carries a text that was deleted before it.

## Restoring

`Restore` writes every file of a copy into an empty data folder, checking each one against the manifest as it goes, and refuses to continue at the first file that does not match. The journal opens as usual afterwards, cutting an unfinished line at its end if the copy caught the journal in the middle of a write, and the indexes are rebuilt from it. The erasure runs before any screen is served, so a deletion that was waiting for the night is done on the restored machine as well.

`Check` reads a copy whole without writing anything. It is the restore test that the architecture asks for once a month: a copy that was never checked does not count.

## Not built yet

- A second machine that pulls the new entries over HTTPS and keeps them encrypted, and the pairing of that machine.
- The copies screen, with the destinations, the age of the last copy and the restore test.
- Showing the key as words on the day the community is born, in the setup.
- A copy that is older than the journal cannot be told from a copy that was rolled back on purpose. The seal that the community writes down each week is what shows that.
