# Glossary

The words the code uses. A word listed here means one thing everywhere: in package names, in event types, in folders on the machine and in addresses.

## The system

| Word | Meaning |
| --- | --- |
| Community | The group of people that runs one installation, up to about a thousand |
| Resident | A person who belongs to the community |
| Core | The program that serves the screens and owns the data |
| Screen | One page of the system, with its own folder in the code |
| Service | A ready-made program that the core talks to, such as a video converter |

## Data

| Word | Meaning |
| --- | --- |
| Journal | The append-only record of everything that happened. The single source of truth |
| Entry | One line of the journal. Its number is the identity of what it created |
| Segment | One file of the journal |
| Digest | The SHA-256 of the content of an entry |
| Seal | What ties an entry to the previous one, so a change in the past shows |
| Scribe | The single writer. Nothing reaches the journal except through it |
| Index | A file derived from the journal that screens read. It can always be rebuilt |
| Area | The description of one index: its tables and how each event changes them |
| Event | What an entry carries: who did what, and when |
| Snapshot | A saved copy of the indexes, to avoid rebuilding everything |
| Copy | An encrypted copy of the journal and the secrets, kept somewhere else. Only the copy key opens it |
| Copy key | The key that encrypts the copies, made when the community is born and written down on paper |
| Deletion | An entry that takes a message out of its channel. The text leaves the indexes at once |
| Erasure | The night's work that replaces deleted content in the journal with the erased mark |
| Live updates | The connection an open page keeps, on which the server sends what just changed |

## People and access

| Word | Meaning |
| --- | --- |
| Account | A resident, as the system knows them |
| Device | A browser that an account uses |
| Passphrase | The words that open what is personal to a resident. Not built yet |
| Vault | Where a resident keeps personal files and keys, encrypted. Not built yet |
| Trusted contacts | The people who can help a resident recover an account. Not built yet |
| Guardian | The adult responsible for a child's account |
| Admin | A resident who looks after the community through the screens |
| Operator | The person with access to the machine itself |
| Charter | The rules for living together that each community chooses |

## Parts of the system

| Word | Meaning |
| --- | --- |
| Chat | Conversation between residents |
| Channel | A conversation open to every resident |
| Collection | The downloaded library: encyclopedias, manuals, maps, films |
| Pack | One item of the collection, with its license and origin |
| Lesson | A video a resident publishes to teach something |
| Mentor | The local artificial intelligence that answers by citing the collection |
| Caretaker | The part that watches the machine and proposes fixes, which a person approves |
| Bridge | The only program allowed to reach the internet |
| Door | One kind of connection to the outside, which the community opens or keeps shut |
| Alert | An urgent message to everyone |
| Proposal | A text the community votes on |
| Delegation | A vote lent to someone the resident trusts |
| Report | Something a resident marks on the map: a problem or a resource |
| Flag | A resident's warning about content that should not be there |

## Event types

A type is a noun and a verb in the past, in lowercase, joined by a dot: `message.created`, `account.created`. The list is in [`core/event/types.go`](../core/event/types.go).
