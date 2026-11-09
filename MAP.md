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
| [`core/search`](core/search) | How text becomes search terms, in Portuguese |
| [`core/page`](core/page) | The layout and the base style that every screen is drawn in |
| [`screens/signup`](screens/signup) | Creating an account |
| [`screens/chat`](screens/chat) | The open channel |
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
| `secrets/` | The key that signs the session cookies | Everyone has to sign in again |

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

```bash
go run ./cmd/earthler -community "My community"
```

The server answers on port 8080 over plain HTTP. It is in early development and must not be used to run a real community.
