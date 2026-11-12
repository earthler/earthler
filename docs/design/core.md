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

The account and its first device are one entry of the journal. With two entries, a cut between them left a name taken by an account that nobody could open. Journals written with the two entries still work.

A name is made of letters, digits, punctuation, symbols and the plain space. Characters that cannot be seen, or that fool the eye, are refused: zero-width characters, direction overrides, private use characters, text that is not valid UTF-8, and combining marks, because an accented letter can be written as one character or as two and the same name could be taken twice. Two names that differ only in case or in spaces are the same name. What is not done yet: names in a script that needs joiners or marks, and letters from another alphabet that look like Latin ones.

The session cookie holds the number of the device, a counter and a signature of the two made with a key that only the server has. There is no table of sessions, so rebuilding every index signs nobody out. The cookie is not readable by scripts and is never sent by another site.

Every request that changes something must carry the `Origin` of the system itself, with the same scheme and the same host. Browsers send it with every form they post. A page served over plain HTTP, which anyone on the network could have changed on the way, does not count as the secure site.

Forms are read in one place, in front of every screen, with a limit of 64 KB. Anything that is not a plain form, a file for instance, arrives as an empty form and is refused by the screen, and its body is never read or kept on the disk, where it could fill the space the journal needs.

A failure of the system while looking up a session is a failure, not a signed out visitor: the resident gets an error and is not sent to the sign up screen.

## Search

The full text search of SQLite only knows how to reduce English words to their root. So the text is prepared before it reaches the index: split into words, lowercased, reduced by the Portuguese Snowball stemmer and stripped of accents. The question typed in the search field goes through the same function, so the two always meet.

`plantas`, `Plantar` and `planta` become the same term. Derived words such as `plantio` do not, and some words with a nasal ending do not meet their plural. A list of synonyms is the planned answer.

Whatever a person types is turned into quoted terms, so nothing reaches the index as an operator.

## Access

Browsers only allow camera, microphone and offline use on HTTPS pages, and a public certificate has to be renewed over the internet every few months. So each community has its own certificate authority, created when the community is born:

- A root certificate that devices install once, valid for thirty years.
- An issuer, signed by the root, that signs the server's certificates.

Both are limited to the community's own name, such as `vila.internal`, and to private network addresses, and the issuer cannot create another issuer. The root key signs the issuer and is thrown away at once: it is never written to disk. Whoever steals the machine gets the issuer key, which can only sign for that community's name and for local addresses.

The server answers on two ports. Port 80 serves only the welcome pages and the certificate, and no resident data ever passes through it. Port 443 serves the system. The welcome pages show the first characters of the root's fingerprint, to compare with a printed poster, because a page that arrives without HTTPS could have been changed on the way.

The server issues its own certificate each time it starts, for the addresses the machine has at that moment. The root is named after the community, because that name is what a phone lists in its trusted certificates.

### Installing the certificate

This is the hardest step a resident ever takes, and the phone makers made it hard on purpose: a root installed by mistake would let someone read everything the person does. So the welcome screen does not try to make it short. It tries to make it impossible to get lost:

- One page per kind of device, at `/certificate/iphone`, `/certificate/android`, `/certificate/windows`, `/certificate/mac` and `/certificate/linux`, chosen from what the browser says about itself. A browser the system does not recognise gets `/certificate/computer`, which only asks which system it is. Every page links to the others.
- One card per step, saying what the device will show and what to tap, with the names the device uses in the community's language. A step that commonly goes wrong carries a note.
- The person marks each step done, so the right card is highlighted when they come back from the settings of the device. The page loads a small image from the secure site every few seconds, and the moment it loads the page says the device trusts the certificate and offers the way in.
- An iPhone receives the root inside a configuration profile, which carries the community's name and an explanation and is what the Settings app expects. Installing the profile is not the end on an iPhone: trust has to be turned on in a second place, and that has a card of its own. Only Safari hands a profile to Settings, so an iPhone on another browser is sent to Safari first.
- Android and computers receive the root as a file.

### What the first proof showed

On 2026-11-09, an iPhone 13 Pro with iOS 27.0.1 installed the certificate through Safari, opened the system with no warning once trust was on, got a secure context, Web Crypto, a service worker, persistent storage, camera and microphone, and refused forged certificates for two names outside the community. That is the part of the design that no documentation covered: Apple's verifier enforces the limits written in the root. The same run showed that Chrome for iOS cannot install the profile, and that a three-line guide is not enough even for someone experienced, which is why the welcome screen is shaped as above.

Not shown yet: a Wi-Fi network with no internet, and any Android device. The texts of the Android, Windows, Mac and Linux pages use the menu names from the official documentation and from the systems' own language files (Android's settings, Firefox's and macOS's translations), and have not been checked on a device.

## Live updates

A page that is open keeps one connection to the server, over Server-Sent Events, and the server sends on it each change the page should show. The scribe tells its listeners about every batch once the indexes have it and before it answers the writer, so an open page learns of a message before the page that sent it. The screen renders each change once and the hub in [`core/live`](../../core/live) hands it to every connection.

- The browser reopens the connection by itself after a dropped Wi-Fi, saying the number of the last entry it saw, and the page catches up from the index. More than 500 changes behind, it reloads instead.
- A connection that falls more than 32 messages behind is dropped. Its browser reconnects and catches up the same way.
- A comment line every 25 seconds keeps the connection open and lets the browser notice a dead one.
- Everything is sent to everyone, because today everything is open to every resident. The permission table will decide who receives what.

- Text that carries line ends is written to the stream with every kind of line end turned into one, so a message can never start a field of its own.
- A server that is stopping closes the hub first, because connections that never go quiet would keep it waiting.

The page works without the script: it only saves the reload.

## Deletion

A deletion is an entry like any other, so it survives a rebuild of the indexes. The text leaves the indexes at once, the screen says a message was deleted, and the open pages hear of it. Only the author can delete, until roles exist.

The entry that holds the text stays in the journal until the night's work, in [`core/erasure`](../../core/erasure), rewrites its segment with the content replaced by the erased mark. The digest and the seals stay, so the chain stays whole; the salt goes with the text, so the digest that stays cannot confirm a guess of what the text was. What is pending is itself an index derived from the journal, and the erasure is recorded as an entry, so a rebuilt index asks for the same work again and erasing twice changes nothing. The erasure also runs when the system starts, before any screen is served, so a restored copy never serves a text that was deleted. The indexes overwrite what they delete, and the search table does the same.

## Copies

The [copies](copies.md) have a note of their own: what goes in one, the key the person writes down, and the encrypted format.

## Screens

Pages are built on the server and work without JavaScript, except for the welcome page, which uses a small script to notice when the device starts trusting the certificate, and the conversation, which uses one to show new messages without a reload. Every text a person reads comes from a file in [`locales`](../../locales), one per language, with Brazilian Portuguese as the first.

## What is not built yet

- The rest of the [access proof](../../experiments/access-proof): Android, and a Wi-Fi network with no internet.
- Renewing the server certificate while the server runs, and the screens of the setup.
- The passphrase, the personal layer and everything that depends on them.
- Roles and the permission table. Today a resident can do everything there is to do, and every live update goes to everyone.
- A second machine that receives the entries as they are written, and the copies screen.
- Editing a message, and the record of searches that found nothing.
