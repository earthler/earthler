# Journal proof

A tool that checks the [journal](../../docs/design/journal.md) against a real power cut.

## What it checks

The journal promises that an entry it confirmed is never lost. The tests in `core/journal` kill the writing program many times, but killing a program is not the same as cutting the power: the operating system still writes what the program handed over. Cheap disks also claim to have saved data they have not saved, and write out of order.

The only real proof is to pull the plug on a real machine while it writes, many times, and see whether every confirmed entry is still there.

## How it works

It needs two machines on the same network.

- The **collector** runs on a machine that stays on. It remembers the highest entry the other machine confirmed.
- The **writer** runs on the machine under test. It appends to a journal without stopping, and tells the collector about each entry only after the disk confirmed it.

After a power cut, the checker opens the journal the same way the system does, runs the full check and asks the collector what was confirmed. Everything up to that entry has to be there, with the same seal.

## Run

Build it for the machine under test. For a Linux server with an Intel or AMD processor:

```bash
GOOS=linux GOARCH=amd64 go build -o journal-proof ./cmd/journal-proof
```

On the machine that stays on:

```bash
go run ./cmd/journal-proof collect
```

On the machine under test, with the address of the machine that stays on:

```bash
./journal-proof write -collector 192.168.1.50:7070
```

Then, for each round:

1. Wait a few seconds while it writes, and pull the plug. Do not shut the machine down: cut the power.
2. Plug it back in and wait for it to start.
3. Run the same `write` command again. It checks the journal first, tells the collector the verdict, and goes back to writing.

The collector prints one `RESULT PASS` or `RESULT FAIL` line per round and keeps them in `journal-proof.log`. The acceptance target of the design is 50 rounds with no failure, on a cheap disk.

To check one last time without writing again:

```bash
./journal-proof check -collector 192.168.1.50:7070
```

## Reading a result

- `PASS` says where the journal ends, how many entries were confirmed, and how many were written but never confirmed. That last number is fine at any value: those entries were never promised.
- `FAIL` with "the journal did not open" means the power cut left something other than an unfinished last line. The message names the entry and the position. Keep the data folder: it shows what a real disk leaves behind, and the rule for opening after a power cut may need to change because of it.
- `FAIL` with "the journal ends at entry" means the disk confirmed a write it had not saved. That is the disk lying, and no program can fix it. It says that disk must not be used.

## Notes

- Use the kind of disk a community would use, and the file system the design proposes for data, ext4.
- Keep the data folder between rounds, so the journal grows across power cuts.
- The tool listens and connects in plain text on the local network, and writes only test data.
