# Map

Where everything is, for whoever has to find their way without help.

## The shape of the code

The journal is the truth. The scribe is the only one who writes in it. The indexes are derived from it and can be rebuilt. A screen asks the scribe to write and reads from its own index. A screen never calls another screen and never writes on its own.

```
browser -> screen -> scribe -> journal -> indexes -> screen -> browser
```

Two tests in [`rules_test.go`](rules_test.go) keep this shape: no file has more than 300 lines, and the layers only point one way.

## Folders

| Folder | What is in it |
| --- | --- |
| [`cmd/earthler`](cmd/earthler) | The program a community runs. It opens the journal and the indexes, starts the scribe and mounts the screens |
| [`cmd/journal-proof`](cmd/journal-proof) | A tool that checks the journal against a real power cut, with two machines |
| [`core/journal`](core/journal) | The append-only record, with sealed entries and recovery after a power cut |
| [`core/index`](core/index) | The SQLite files derived from the journal, and how they are rebuilt |
| [`core/event`](core/event) | What goes inside an entry, and the catalog of event types |
| [`core/scribe`](core/scribe) | The single writer: journal first, then the indexes, then the answer |
| [`core/accounts`](core/accounts) | Residents, their browsers and the session cookie |
| [`core/permissions`](core/permissions) | The roles, and the one table that says who may do what |
| [`core/roster`](core/roster) | Who has which role, followed from the journal by every index that needs it |
| [`core/certificate`](core/certificate) | The community's own certificate authority, for HTTPS with no internet |
| [`core/live`](core/live) | The connection that every open page keeps, on which the server sends what just changed |
| [`core/erasure`](core/erasure) | The night's work that takes deleted content out of the journal for good |
| [`core/durable`](core/durable) | Writing a small file so that it is whole or not there after a power cut |
| [`core/replica`](core/replica) | The second machine: pairing, the token, and pulling the encrypted copy over HTTPS |
| [`core/copies`](core/copies) | Encrypted copies of the journal and the secrets, and the way back from one |
| [`core/search`](core/search) | How text becomes search terms, in Portuguese |
| [`core/page`](core/page) | The layout and the base style that every screen is drawn in |
| [`screens/welcome`](screens/welcome) | The only screen without HTTPS: it walks a new device through installing the certificate, one page per kind of device |
| [`screens/signup`](screens/signup) | Creating an account |
| [`screens/chat`](screens/chat) | The open channel |
| [`screens/auditlog`](screens/auditlog) | The log of what administrators and moderators did |
| [`screens/residents`](screens/residents) | The list of residents, where an administrator gives roles |
| [`screens/search`](screens/search) | The search field and its results |
| [`locales`](locales) | Every text people read on a screen, one file per language |
| [`experiments`](experiments) | Disposable programs that test the design on real devices. Not part of the system |
| [`docs/design`](docs/design) | Why each part is the way it is |
| [`docs/glossary.md`](docs/glossary.md) | The words the code uses, and what each one means |
| [`brand`](brand) | The logo. Not under the code license |

## Where the data lives

The server keeps everything under one data folder:

| Folder | What is in it | If it is lost |
| --- | --- | --- |
| `journal/` | The segments of the journal | It comes back from a copy. Nothing else can rebuild it |
| `indexes/` | One SQLite file per area | Deleted and rebuilt from the journal |
| `secrets/` | The key that signs the session cookies, the certificate chain and the copy key | Everyone signs in again, and every device installs a new certificate. The copy key comes back from the paper it was written on |

## How to add something

1. Add the event type and its data to [`core/event/types.go`](core/event/types.go). A type that was once written is never removed and never given another meaning.
2. In the screen's folder, describe its index as an `index.Area`: the tables, and how each event changes them. An `Apply` function ignores what it does not understand and only returns an error when the database fails.
3. Write through the scribe, read from the index.
4. Put every text people will read in [`locales`](locales), never in the code.
5. Write the test that proves it, including what happens after a restart.

## Running it

```bash
go test ./...
```

To work on the screens, on your own computer and with no certificate:

```bash
go run ./cmd/earthler -dev 127.0.0.1:8080 -community "My community" -name mine
```

Without `-dev`, the server answers on port 80 with the welcome page and on port 443 with the system, and prints the address that new devices open:

```bash
go run ./cmd/earthler -community "My community" -name mine
```

To write an encrypted copy of the data into a folder, to print the key that opens it, and to bring the data back on another machine:

```bash
go run ./cmd/earthler -copy /path/to/disk
```

```bash
go run ./cmd/earthler -print-copy-key
```

```bash
go run ./cmd/earthler -restore /path/to/disk -copy-key "THE-KEY-WRITTEN-DOWN"
```

With `-copies /path/to/disk`, the server makes the copy every night by itself. To read a whole copy with the key and know that it can be restored:

```bash
go run ./cmd/earthler -check /path/to/disk -copy-key "THE-KEY-WRITTEN-DOWN"
```

To keep a second machine, ask the main machine for a pairing code while it runs, and give it to the second machine with the fingerprint that comes with it:

```bash
go run ./cmd/earthler -pair
```

```bash
go run ./cmd/earthler -follow https://ADDRESS-OF-THE-MAIN-MACHINE -pair-code THE-CODE -fingerprint THE-FINGERPRINT
```

`-machines` lists the second machines that are paired, and `-unpair NUMBER` takes one away. To take over, turn the first machine off and restore the second machine's folder with the copy key, as above.

It is in early development and must not be used to run a real community.
