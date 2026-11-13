# The second machine

A community that keeps everything on one machine stops when that machine stops. The second machine is the answer that needs no new idea: the same program, in a mode that opens no screens, keeps an up to date encrypted copy of what cannot be rebuilt, and when the first machine is gone, a person types the copy key and it becomes the system.

## What it is

The main machine already keeps an encrypted copy of the journal and the secrets, for the disk that goes to a neighbour (see [copies](copies.md)). The second machine is that copy moved over HTTPS. It adds three things: a way to pair, a token that can only pull, and a program that keeps pulling.

The second machine never holds the copy key. What it keeps is the encrypted copy, exactly as it came, so a machine in another house reads nothing, and its owner cannot read the community. What it does not hide is the shape of the copy: how many files there are, how big they are and when they change.

## Pairing

A person asks the main machine for a pairing code. It is twelve letters and digits, it works once, and it expires in ten minutes. The main machine also prints the fingerprint of the community root, which is the one thing the second machine has to check before it trusts anything.

The second machine fetches the root from the welcome page, which anyone on the network could have changed, and refuses it unless its fingerprint is the one the person typed from the main machine's own screen. From then on every connection is checked against that root and nothing else, and only for the community's own name, which the root is limited to. With the root it trades the code, over that connection, for a token.

The main machine writes only the hash of the code and of each token. Wrong codes and wrong tokens are counted for each address, and five wrong answers in a minute stop the address for the rest of the minute. A lost second machine is unpaired with a command, and its next pull is refused.

## Following

The main machine brings its copy up to date when a second machine asks, at most once every few seconds, and answers with an index: the files of the copy with their size and the digest of their encrypted bytes, the last entry of the journal and its seal. The second machine downloads what it does not have, checks each file against its size and digest, and only when all of them are in does it put the manifest in place and remove the files the copy no longer has. A pull that is cut, or a file that arrives changed, leaves the copy that was there before, whole and readable with the key.

An erasure on the main machine rewrites a segment of the journal, which makes a new file with a new name. The old file leaves the second machine in the pull that follows, so a text that was deleted is not kept for long on a machine somewhere else.

## When the history is not the one followed

On every pull the second machine tells the main one the last entry and seal it holds. If the main machine says that history is not its own, or its journal is now shorter, the main machine was restored from something older, or it is another community. The second machine moves what it holds into a folder of its own, deletes nothing, and takes the new history in full. A person can still look at what was set aside.

## Taking over

The first machine is turned off. On the second, `earthler -restore` with the copy key puts the journal and the secrets in a data folder, and the system starts from there, with the same certificate chain, so no device has to install anything again. The indexes are rebuilt from the journal and the erasure runs before any screen is served. The devices reach the system by its address, so the second machine has to take the first one's address, or the people open the new one.

`earthler -check` reads a whole copy with the key without writing anything, which is the monthly test that a copy can be restored.

## What it costs, and what is not done

- A file of the copy that changes is sent whole. The open segment of the journal can be up to 64 MB, so a second machine that pulls every minute can move that much every time something was written. Sending only what was appended is possible with the format as it is, and is left until the cost is measured on a real community.
- The second machine is as current as its last pull. What was written after it is lost if the main machine dies at that moment.
- Pairing and unpairing are commands. The screen for them belongs to the page of copies, which is not built.
- A machine in another house has the same pairing, over an address that the other house can reach. How two houses find each other without the internet is not solved here.
