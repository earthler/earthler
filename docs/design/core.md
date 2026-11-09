# The core

How a change travels from a browser to the disk and back to a screen. The [journal](journal.md) has its own note.

## The path of a write

Every change to the community's data takes the same path:

1. A screen checks the request: who is asking, whether it came from one of the system's own pages, and whether the content is acceptable. A refused request writes nothing.
2. The screen hands an event to the scribe.
3. The scribe writes the event in the journal and waits for the disk to confirm it.
4. The scribe applies the new entries to every index, one transaction per index.
5. Only then does the screen get its answer.

Because the answer comes after the indexes, a person who wrote something always sees it on the next page.

The scribe is a single flow of the program, so entries get their numbers in one place and never collide. It does not wait on purpose to form batches: while the disk confirms one batch, the next requests pile up and become the next batch.

If the journal refuses a write, or an index fails after the journal accepted the entries, the scribe stops and the system stays read-only until it is restarted. On restart, each index is brought level with the journal before any screen is served. That same step is how the system comes back after a power cut.

## Events

An entry carries one event: its type, the server clock, the account and the device that acted, a random salt and the data of that type.

- **Type.** A noun and a verb in the past, such as `message.created`. Types are written in journals forever, so a type that was once used is never removed and never given another meaning. The catalog is [`core/event/types.go`](../../core/event/types.go).
- **Time.** For showing to people. The order of events is the order of the entries, so a wrong clock cannot scramble the journal.
- **Account and device.** The entry numbers of who acted. Zero means the system.
- **Salt.** Random bytes that go away with the content when an entry is erased. Without it, the digest that stays in the journal could be used to confirm a guess of what the erased text said.

The field names are whole words on purpose. Single letters would save a few bytes per entry, and whole words can be read by whoever opens the file with nothing but a text editor.

## Indexes

An index is one SQLite file for one area: accounts, chat, search. Each area declares its tables and one function that says how an event changes them.

- The function ignores events it does not understand, and only fails when the database fails. One strange entry must never be able to stop an index, and with it the whole system, from starting.
- Each index remembers the number and the seal of the last entry it applied. When that does not match the journal, the index is deleted and rebuilt from entry 1. That covers an index that is ahead of a journal that lost its unconfirmed tail, and a journal that was restored from a copy.
- An index made with another schema is rebuilt too, which is how an upgrade changes an index without migrating anything.
- Each index keeps its own copy of what it shows, such as the author names, so no query ever crosses two files.

## Accounts and sessions

This is the first, minimal form. A resident signs up with a name, and that browser stays signed in. There is no passphrase yet: an account cannot be opened from a second device, and nothing personal is protected. Both arrive with the personal layer, which encrypts in the browser.

The session cookie holds the number of the device, a counter and a signature of the two made with a key that only the server has. There is no table of sessions, so rebuilding every index signs nobody out. The cookie is not readable by scripts and is never sent by another site.

Every request that changes something must carry the `Origin` of the system itself. Browsers send it with every form they post.

## Search

The full text search of SQLite only knows how to reduce English words to their root. So the text is prepared before it reaches the index: split into words, lowercased, reduced by the Portuguese Snowball stemmer and stripped of accents. The question typed in the search field goes through the same function, so the two always meet.

`plantas`, `Plantar` and `planta` become the same term. Derived words such as `plantio` do not, and some words with a nasal ending do not meet their plural. A list of synonyms is the planned answer.

Whatever a person types is turned into quoted terms, so nothing reaches the index as an operator.

## Access

Browsers only allow camera, microphone and offline use on HTTPS pages, and a public certificate has to be renewed over the internet every few months. So each community has its own certificate authority, created when the community is born:

- A root certificate that devices install once, valid for thirty years.
- An issuer, signed by the root, that signs the server's certificates.

Both are limited to the community's own name, such as `vila.internal`, and to private network addresses, and the issuer cannot create another issuer. The root key signs the issuer and is thrown away at once: it is never written to disk. Whoever steals the machine gets the issuer key, which can only sign for that community's name and for local addresses.

The server answers on two ports. Port 80 serves only the welcome page and the certificate, and no resident data ever passes through it. Port 443 serves the system. The welcome page shows the first characters of the root's fingerprint, to compare with a printed poster, because a page that arrives without HTTPS could have been changed on the way.

The server issues its own certificate each time it starts, for the addresses the machine has at that moment.

## Screens

Pages are built on the server and work without JavaScript, except for the welcome page, which uses a small script to notice when the device starts trusting the certificate. Every text a person reads comes from a file in [`locales`](../../locales), one per language, with Brazilian Portuguese as the first.

## What is not built yet

- The proof that real phones accept this design. The [access proof](../../experiments/access-proof) is the experiment for it, and the access design may change with what it shows.
- Renewing the server certificate while the server runs, and the screens of the setup.
- The passphrase, the personal layer and everything that depends on them.
- Live updates. A page shows new messages when it is loaded again.
- Roles and the permission table. Today a resident can do everything there is to do.
- Erasing, backups, and the record of searches that found nothing.
