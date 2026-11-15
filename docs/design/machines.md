# How the machines of a community live together

This note is the part of [many machines, one community](many-machines.md) that is built: how a machine joins, how each machine follows the others, and how what is counted is decided. The code is in [`core/machines`](../../core/machines).

What a resident sees is the same on every machine: the same conversation, signed in on all of them with one account. Behind it, each machine writes only in its own journal and keeps a copy of every other machine's journal, and the indexes of each machine apply all of them. With one machine, nothing of this happens and the system is the one of before.

## Joining

The machine that counts, machine 0 today, prints an invitation with `-invite`: a code that works once, for ten minutes, and the fingerprint of the community root. On the new machine, with an empty data folder, a person runs `-join` with the address of machine 0, the code and the fingerprint. The same steps pair a [second machine](replica.md), and the same code checks them.

1. The new machine reads the community root from the welcome page, which comes over plain HTTP, and trusts it only if its fingerprint is the one the person read on machine 0.
2. Over a connection that only that root vouches for, it sends the code, a name for itself and the address at which the others reach it.
3. Machine 0 spends the code, gives the new machine the next number, writes `machine.added` in its journal with the SHA-256 of a new token, and answers with the number, the token, the addresses it knows and the community's secrets: the key that signs the session cookies, the copy key and the certificate chain. The first time, machine 0 also adds itself, with a token of its own, kept on its disk before it is written in the journal.
4. The new machine writes the secrets, reads the journal of machine 0 up to that moment, and only then writes its identity, `secrets/machine.json`. A join cut before that can be tried again with a new code. Then the machine starts as usual, without `-join`.

What could fail on machine 0 is done before it writes the new machine in its journal. Only an answer lost on the way leaves a machine number whose token nobody holds; the others then try to follow it and never get an answer, and there is no way yet to take a machine off.

A machine that holds the secrets is an image of the others: it serves the residents with the same certificate and recognises the same session cookies, so a resident signed in on one machine is signed in on all. The secrets never travel again after the join, and the identity of a machine, its number and its token, is the one file of the secrets folder it never hands on.

Only `machine.added` entries written by the machine that counts are believed. The same line in any other journal changes nothing.

## Following

Machines talk to each other only over HTTPS, even in development mode, so tokens and secrets never travel in the clear. Each machine serves its own journal at `/machines/journal`, to a machine that shows the token of another machine of the community. A token has 256 random bits, so wrong tokens are not counted against an address: a machine that has just joined is refused for a moment by machines that have not heard of it yet, and must not be locked out for it. It sends the entries after the one the other machine asks from, one per line, exactly as they are in the file, and then each new entry as it is written, with an empty line every twenty seconds to keep the connection alive.

Each machine follows each other machine directly, with one follower per machine, and keeps the copy in `journals/<number>` of its data folder. The follower checks that the machine that answers is the one it asked for, and gives the entries to the scribe, which adds them to the copy only if each one follows the last in number and seal and matches its digest, applies them to the indexes and tells the open pages. A machine that is away is asked again after a second, then twice as long each time, up to ten seconds. A stream that says nothing for a minute, not even the empty line, is taken as dead and opened again, and dialling another machine gives up after ten seconds.

A machine never hands on its copy of another machine's journal. An entry of a journal only ever comes from the machine that wrote it, which is what keeps one machine from passing off entries as another's, as long as entries are not signed. The price is that a machine only gets what another wrote while that machine is reachable. Machines learn each other's addresses from the machine that counts at the join and then ask each other every thirty seconds which addresses they know, so each one can follow every other even when machine 0 is away. A machine only learns addresses of machines it did not know: the address it has is the one it reached the machine at, and another machine may reach the same machine at another address. An address that changes, as one handed out by a router can, is not learned again yet.

## A machine restored from a copy

A machine restored from last night's copy has lost the end of its journal, and the other machines still hold it. If it wrote anything, its new entries would take numbers the others hold with other contents, and the community would split in two histories. So before it writes anything, a machine asks each other machine it knows for what they hold of its own journal past its last entry, at `/machines/copy`, and takes it back, checked against the seal chain like any entry it receives. A machine asks only for its own journal. A machine that cannot be reached is skipped, and the machine starts anyway, as a machine alone does.

The night's copy carries the identity of the machine and the addresses of the others, so this works for any copy taken after the machine's first invitation or join. A copy of machine 0 from before its first invitation has no token to show the others, and must not be used once other machines have joined.

## What is counted

What can have only one owner or one result waits for the machine that counts. Today that is machine 0, and two things are counted: a new account, because names are unique, and a change of role. On another machine the screen sends the request to machine 0, which decides with the rules and the roles it knows, writes the entry in its own journal and answers with the id. The screen then waits a moment for the entry to reach its own indexes, so the next page knows it.

When the request never reached machine 0, or machine 0 answered that it could not do it, the screen says at once that it cannot be done now and why: "Agora não dá para criar a conta", in the Portuguese of the screens. When the request reached machine 0 and the answer was lost, the act may have been done. A sign up carries a random mark that the account keeps, so the machine that asked looks for an account with its mark, and for a change of role it looks for the role; it says not now only when that does not arrive. Everything that never waits, writing, reading, deleting one's own message, goes on.

## Picking up live

A page cannot say how far it has seen with one number once several machines write at the same time. It keeps a position in each journal, written like `0.120-1.33`: entry 120 of machine 0 and entry 33 of machine 1. The page receives it when it opens, each live message carries the position after it, and the browser sends the last one back when it reconnects, to any machine, which sends what is past it. A page opened before there were many machines sends a plain number, which is read as a position in the journal of machine 0.

A message that arrives after messages written later, because it was written on a machine that was away, goes in its place in the conversation by time, on the page and in the index.

## On one computer

Two machines can run on one computer, for development, each with its own data folder and ports. With `-dev` and `-https` together, a machine serves the screens over plain HTTP for the browser and the secure site for the other machines:

```bash
go run ./cmd/earthler -data m0 -community "Vila" -name vila -dev 127.0.0.1:8090 -https 127.0.0.1:8543 -http 127.0.0.1:8180
```

```bash
go run ./cmd/earthler -data m0 -invite
```

```bash
go run ./cmd/earthler -data m1 -join https://127.0.0.1:8543 -plain-url http://127.0.0.1:8180 -pair-code THE-CODE -fingerprint THE-FINGERPRINT -label "second" -reach https://127.0.0.1:8544 -https 127.0.0.1:8544
```

```bash
go run ./cmd/earthler -data m1 -dev 127.0.0.1:8091 -https 127.0.0.1:8544 -http 127.0.0.1:8181
```

A browser shares cookies between two ports of the same host, so `127.0.0.1:8090` and `localhost:8091` behave as two different people.

## What the tests cover

- Two machines over TLS, with the real program: joining with a code, a folder that already belongs to a community refused, a resident of machine 0 signed in on machine 1, a sign up through machine 1 decided by machine 0 and a name taken twice refused, the same conversation on both, a page open on machine 1 hearing what is written on machine 0, machine 0 away while machine 1 goes on and says not now to a sign up, machine 0 back with each machine getting what it missed, and machine 1 restarted from its folder.
- The journal of a machine refused without a token, with a wrong one and with the machine's own, and too many wrong tokens from one address refused for a while.
- A `machine.added` from a journal other than the one of the machine that counts changing nothing.
- A join on a machine that does not count, with a wrong code, with a code used twice and with an address that is not https, refused.
- A copy that refuses an entry of another history, an entry it already has, and a content that does not match its digest, and goes on receiving the right one.
- The night's copy of machine 1 restored into an empty folder: the machine comes back with its number, its copy of machine 0's journal and machine 0's address. The night's copy carries the copies of the other machines' journals and their addresses for that reason.
- Machine 0 restored from a copy older than its journal, while machine 1 holds the end: machine 0 takes the end back before writing, and machine 1 follows what it writes next.
- A sign up whose answer was lost: found by its mark when the account was made, not now when it was not.
- Tokens and joins refused over plain HTTP, wrong tokens not locking a machine out, and a machine refused when it asks for another machine's journal back.

It also ran by hand on one computer, two machines in two tabs of a browser: each page received what was written on the other machine as it was written, machine 0 was stopped and started again, and each got what the other wrote meanwhile.

## Not built yet

- The list of machines and what waits for all of them. Today what is counted waits for machine 0 alone, and if machine 0 dies for good, nothing counted can be done until an administrator can choose another, which is not built either.
- Signed entries, so a copy could be handed on and a machine that is away could be caught up through another.
- The comparison of clocks when two machines meet.
- A screen for the machines, for the administrator: today it is `-invite` and `-join` on the command line. Taking a machine off, or a token away, is not built either.
- Learning a new address of a machine already known.
- The network, the device's part and the four apps.
