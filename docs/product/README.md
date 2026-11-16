# What Earthler shows people

This folder is the product: every screen a resident, a moderator or an administrator sees, what each one does and how it behaves, including with no link to the community and with many machines. It is what the code is built against. The maintainer approved the direction on 2026-11-15: a portal at the community's address, four apps with a face of their own, the same rules everywhere, and a dark mode.

The architecture that carries it is in [`docs/design`](../design). Where this folder says how a screen behaves and a design note says how the core works, both hold; where they disagree, the disagreement is a bug to fix in one of them.

## How to read it

| Document | What it covers |
| --- | --- |
| [Principles](principles.md) | The rules every screen follows: one address, the portal, four apps, the same face, the four notices, nothing lost, what waits says so, accessibility, language |
| [Visual identity](visual.md) | The Terra palette in light and dark, the type, the pieces of every screen, and the cover of each app |
| [Portal and search](portal.md) | What the community's address opens, and the search of the whole community |
| [Account](account.md) | Arriving, the certificate, creating an account, the passphrase, signing in on another device, recovering access, the profile, devices, the personal vault and leaving |
| [Talk](talk.md) | Conversations, calls and alerts |
| [Learn](learn.md) | Subjects, lessons, the wiki, the collection and films |
| [Decide](decide.md) | Proposals, votes, delegations and the map |
| [Ask](ask.md) | Questions to the Mentor, answers and what the collection is missing |
| [Administration](admin.md) | Residents and roles, moderation, the charter, the log, guardians, the collection, the machines, the caretaker, copies, doors to the internet and connected AIs |
| [Setup](setup.md) | Installing a community and its first screens, before anyone signs up |

The last area comes in a change of its own, so that it is reviewed on its own: the notices and behavior shared by every screen. It adds its line to this table. Until then, a link from these documents to one of them leads nowhere.

Every screen is drawn in [`docs/screens`](../screens), in light and in dark mode, from one HTML file each. The pictures in these documents come from there.

## How each document is organised

Each area starts with what it is for, in a few sentences about what a person does with it. Then each screen has its own section:

- its path and the picture, in light and dark;
- what the person sees, from top to bottom;
- what they can do there;
- its states: empty, refused, waiting, with no link, and the errors a person can meet, with the sentence they read;
- who may use it, by role;
- what it writes in the journal and what it reads;
- how it behaves with many machines: whether it never waits, or waits for the listed machines.

After the screens come the flows that cross several screens, a table of who may do what, and what is not decided.

## The marks

Every claim that matters carries one:

- **Built**: in the code on `main` today.
- **Proposed**: approved as the direction, not built yet.
- **To prove**: only holds after a test on a real device or machine.
- **Not decided**: waits for the maintainer.

Texts in quotes, such as "Agora não dá para criar a conta", are what the screen says, in Brazilian Portuguese, the first language of the system. The final words of every text live in [`locales`](../../locales) when the screen is built.
