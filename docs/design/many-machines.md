# Many machines, one community

**Proposed, and being built since 2026-11-15, on the maintainer's word.** Today a community runs on one machine, with one [journal](journal.md) and one scribe, and a [second machine](replica.md) can keep an encrypted copy that it cannot read. This note is the design that comes after that, and it changes as the maintainer decides. What is built so far is listed in [what is built](#what-is-built).

The measure of the design is what a resident sees: it is right only if nobody ever notices which machine they are on. That part has a note of its own, [four apps on one address](apps.md).

Each claim carries a mark where it matters. **Built** is in the code today. **Proposed** is a choice that can change. **To prove** only holds after a test on real machines or devices. **Inference** is a reading, to be corrected.

The words in this note are working words. The [glossary](../glossary.md) takes a word when the code does.

## What it has to do

- Data, connection and services spread over the community's machines. No machine is the community.
- When one machine leaves, nothing changes for the people using the system.
- What is personal lives first on the person's own device, encrypted.
- Some things are essential and never wait. Others can wait.
- It fits a community of up to a thousand people with no technician on call.

## The life it assumes

Power comes from panels and batteries, and a machine turns off at night or when the battery is low. The link between houses is an antenna, and it drops with rain, wind and distance. The phone is the computer people always carry. Nobody is on call. **Inference.**

So a machine leaving and coming back is routine, not an accident. Nobody presses a button when it happens, and nobody types the copy key, as taking over with a second machine needs today.

## The computers of a community

A machine that carries the whole of the essential, described below, is an image of the others: any of them can serve any resident.

| Computer | What it does | Does anyone wait for it |
| --- | --- | --- |
| A resident's phone or laptop, in the browser | Keeps what is personal to its owner, encrypted, and an outbox with what has not been delivered yet | No |
| A listed machine, one of the fixed machines the community chooses | Is an image, serves residents and takes part in what is counted | Yes, only for what is counted |
| A machine that comes and goes | Is an image and serves whoever is near | No |
| A second machine, which is **built** | Keeps an encrypted copy and reads nothing | No |
| A service machine, such as the one that runs the Mentor | Runs a heavy service | No |

The third row is where any computer of the community comes in: it can become an image without becoming a point the others depend on.

## Where each thing lives

| Thing | Where it lives | Who reads it | When a machine leaves |
| --- | --- | --- | --- |
| What is personal: conversations between two people, closed groups, files | First on the person's device, encrypted. An encrypted package on every machine, which works as mail and as a safe | Only whoever has the key | Nothing changes. With every machine off, the person still reads what is theirs |
| The essential of the community: alerts, open channels, accounts, roles, wiki, map, proposals and votes | On every machine, the same | The residents | Nothing changes |
| The heavy part: video lessons, films, the downloaded collection | On the machines with disk for it | The residents | Only what was on that machine alone is missing, until that machine returns |
| Machine secrets: the certificate and the key that signs the session cookies | On every machine that serves | Nobody through a screen | Nothing changes |

The machines keep the encrypted package as well as the device, as the maintainer decided on 2026-11-13, for two reasons. Someone who loses their phone signs in on another device with the passphrase and finds what is theirs. And Safari on the iPhone can delete a site's stored data after seven days without use; whether an icon on the Home Screen escapes that is **to prove**. The device is the first home of what is personal, and it cannot be the only one.

## How the machines stay the same with none in charge

Today there is one journal and one scribe numbers its entries (**built**). In this design, each machine has its own journal, written the way it is today: one scribe, numbered entries, each sealed to the one before. Each machine also keeps every other machine's journal and follows it live, the way an open page follows a conversation today and picks up where it stopped. The community is the sum of the journals.

Because each machine only writes in its own journal, two machines never compete for the same entry. When one leaves, the others go on. When it comes back, it hands over what it wrote, receives what it missed, and every machine holds the same journals again. That is what makes them images.

The entry number only orders entries inside one journal, so the identity of each thing becomes the machine and the entry. Across journals the order is the time, with one rule: no entry gets a time earlier than an entry the machine has already seen. An answer then always comes after its question, even with a wrong clock, and every machine builds a conversation in the same order. With no internet a machine only has its own clock, set by hand, so when two machines meet they compare clocks and warn an administrator if the difference is large. The ids and the rule on time are **built**; the comparison of clocks is **proposed**.

With one machine, all of this is the system of today. A community starts with one machine and adds more.

### What is built

- **Built:** the id of each thing is the machine and the entry, in one number. The machine that founded the community is machine 0, so its ids are its entry numbers and nothing written before changes. See [the core](core.md#ids).
- **Built:** the rule on time. The scribe never writes a time earlier than its own last entry, and always one later than what it has seen from the other machines. The conversation is in the order of time, and inside one journal in the order of the entries.
- **Built:** each index applies several journals, with a position in each, and an entry that arrives late takes its place. A test applies the journals of two machines in many orders, with messages written at the same time on both and the same message deleted on both, and every table comes out the same.
- **Built:** each machine erases deleted text in its own journal and in the copies it keeps.
- **Built:** a machine joins with a code and the fingerprint of the community root, gets its number and the community's secrets, and becomes an image. Each machine serves its own journal and follows each other machine directly, live. What is counted, a new account and a change of role, goes to machine 0, and on another machine the screen says not now when machine 0 cannot be reached. An open page picks up live from a position in each journal. How this works is in [how the machines live together](machines.md).
- **Not built:** the listed machines and what waits for all of them; today the machine that counts is machine 0 alone. Signed entries, so a copy could be handed on. The comparison of clocks, a screen for the machines, the device's part and the network.

## Essential and not essential

The rule is short. A fact someone states never waits: I wrote, I called, I warned, I reported, I voted in the open. Two such facts at the same time just pile up. What can only have one owner or one result waits for the machines to meet: who gets a name, who is an administrator, how a vote came out.

| Column | What goes in | Who accepts it |
| --- | --- | --- |
| Essential, never waits | Emergency alert and siren. Message in an open channel and in a personal conversation. Call of up to five people. Reading everything that exists. Report on the map and note in the collection. Writing in the wiki. Proposing and voting in an open vote. Deleting one's own message. Flagging content, and removal by a moderator. Signing in with the passphrase on another device | Any machine, alone |
| Not essential, waits for the listed machines | A new account, because names are unique. A change of role. Closing and counting a vote. A secret ballot. Changing the charter. Adding a machine to the community. Approving a fix the caretaker proposes for the machine | Only with every listed machine able to see the others |
| Not essential, lives where it fits | The Mentor. Publishing and converting a lesson. Films. A large meeting. Search by meaning | Works while the machine that carries it is up |

The maintainer accepted this list on 2026-11-13.

The secret ballot is in the second column for a precise reason. Its ballot carries no name. If the same person voted on both sides of a split community, there would be no way to find one of the two ballots and undo it.

What waits uses a rule with no majority and no election. The community chooses which machines are listed, and that is written in the journal. What is counted only happens when every listed machine can see the others, and always the same machine does the count, as the scribe does today. The price is that with a listed machine off, those things wait until it returns. If it died for good, an administrator takes it off the list, and that is an act of a person, recorded. One administrator is enough to add or remove a machine, as the maintainer decided on 2026-11-13; a community can raise that number later. A machine that comes and goes stays off the list: it serves and follows the journals, and nobody waits for it.

A majority, three of five, would keep the waiting column moving with a machine off. It needs consensus between machines, a piece judged hard to get right, and with this split it is not needed. If the waiting ever gets in the way, a majority can come back for that column alone, if the maintainer asks for it.

## Each part of the system

| Part | One machine leaves | The community is split in two |
| --- | --- | --- |
| Emergency alert | Goes out from any machine | Each side warns its own side, and the alert reaches the other when the machines meet. The siren sounds within reach of the machine that got the alert |
| Open and personal conversation | Continues | Continues on both sides. When they join, the other side's messages take their place in the order, and the screen marks that they arrived later |
| Call of up to five | Continues | Only between people on the same side |
| Large meeting | Drops if it was on the machine that left, and starts again on another that has the service | Each side has its own |
| Wiki, map and notes | Continue | Continue. If two people changed the same page, both versions stay and the screen says so |
| Collection, lessons and films | Continue with what the other machines carry | The same, on each side |
| Search | Continues | Each side searches what it has |
| Proposal and vote | Proposing and voting in the open continue. Closing waits, if the machine is listed | The same |
| New account, roles and charter | Wait, if the machine is listed | Wait |
| Mentor | Off, if it lived there | Only on the side where it lives |
| Deletion | Continues. Each machine deletes in its own journal and in the journals it keeps, and a machine that was away deletes before it serves any screen | The same |

A call of up to five needs two things between machines. The invitation has to pass from one machine to the other at once, without going to the journal. And the two devices, which then connect directly, need a path between them when each is on a different machine. **To prove.**

With every machine off, the device is what is left. The person reads what is personal to them and writes what will go out when a machine appears. A phone cannot find another phone without a machine in between: that is a limit of the browser, and only an app on the device would change it.

## Connection

Connection is one of the three things the machines carry, because five images reached through one router help little. **Proposed:** each machine is also a point of the network. It gives Wi-Fi to whoever is near, with the same network name on every machine, and links to the other machines by whatever path there is, cable or antenna. The community's network becomes the set of its machines. If one leaves, whoever was on it reaches the next, and the others stay linked. If the link between two houses drops, each neighbourhood keeps its own machine. This is the least proven part of the design. It needs a Wi-Fi radio on the machine or an antenna attached to it, and it will only be known after a test with two machines and two real phones. **To prove.**

A limit of the browser shapes it: the browser keeps keys and data by address. If each machine had a different address, a phone would treat each as another site, ask for the passphrase again, and the home of what is personal on the device would break. So every machine answers at the same address, the community's. When each machine is the network point for the people near it, it is the machine itself that hands that address to the devices. **To prove.**

The other limit is why a phone does not carry the connection. Through the browser a phone keeps data, but it does not pass a connection on and does not serve another phone. That would need an app on the device, and for now Earthler has no app, as the maintainer decided on 2026-11-13. The network is carried by the machines.

## Services

The light parts, conversation, wiki, map, voting and search by word, go inside every image, so any machine offers them. The heavy services, the Mentor, video conversion and large meetings, live where the machine can take them. Any machine that can take one may offer it, and a request goes to whichever is up. If the only machine that carries a heavy service leaves, only that service is off. Splitting one heavy job across weak machines is not proposed.

## What it costs

| Piece | Today | In this design | What changes |
| --- | --- | --- | --- |
| The identity of each thing | The machine and the entry (**built**) | The same | Done for accounts, devices, channels, messages and sessions |
| The order of a conversation | The time, with its one rule (**built**) | The same | Done for the conversation, the log and the search |
| Picking up live | A position in each journal (**built**) | The same | Done for the conversation |
| Indexes | Apply several, each in its own order, and an entry that arrives late takes its place (**built**) | The same | Done for every index |
| Exchange between machines | Machines that are images follow each other's journals, live, each straight from the machine that wrote it (**built**). The second machine stays as an option | The same, with signed entries so a copy can be handed on | Reuses the pairing code, the fingerprint and the seal check of the second machine |
| The device | Keeps nothing (**built**) | Keeps what is personal and an outbox | New code in the browser |
| The network | Outside the software | Each machine is a point of the network | A new piece, the least proven |
| Screens | The portal and four apps, on four paths (**built**) | The same, with each app's own screens | See [a portal and four apps](apps.md) |

It is a rework of the core that exists, plus one new piece, the network. Consensus between machines is not on the list.

## What remains a risk

| Risk | What protects | What is left |
| --- | --- | --- |
| A stolen machine | What is personal stays encrypted | The community layer is exposed, and there are more machines to steal |
| An operator, the person with access to a machine | Cannot read what is personal | Reads the community layer and can see who talks to whom. That holds for the operator of each machine. A house that should only keep a copy runs a second machine |
| Two people, one on each side of a split, each taking the other's machine off the list | The essential of both sides joins | In what is counted they become two communities, and people decide which one holds |
| The browser deletes the device's data | The encrypted package on the machines | The person types the passphrase again |
| A wrong clock on a machine | The rule on time and the warning to an administrator | Messages written during a split can appear out of order. Nothing is lost. A clock far ahead carries the times of every machine with it, because none writes earlier than what it has seen, until the warning exists |
| Deleted text on a machine or device that was away for a long time | A machine deletes before it serves any screen | The text stays there until that machine or device returns |
| More machines on | Nothing | More energy. How many stay on is the community's decision |

## Not decided, not proven

- Whether the community's address is a number or a name, and how each machine hands it out.
- The network with two machines and two phones, and a call between devices on different machines.
- Not designed yet: how the machines share the key that signs the session cookies and the certificate chain, and what the first entries of a new machine's journal say.
- Building any of this, which waits for the maintainer's word.
