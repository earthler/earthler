# Principles

Every screen of Earthler follows these rules. They come from the maintainer's decisions of 2026-11-13 to 2026-11-15, and a screen that breaks one is wrong even if it works.

## A person always knows where they are, and the common things work the same everywhere

The maintainer put it as an essential rule, always: consistency of what people see. In practice:

- **One address.** A community has one address, the same on every machine, printed on a poster with a QR code. A resident never chooses a machine and never sees a machine's name or number. Machines are a matter for the administrator's screens. **Proposed**; the machines behind it are in [many machines](../design/many-machines.md).
- **The portal is the center.** The address opens the portal: the search of the whole community, the apps, and what is happening now. Earthler is the whole, and the apps are parts of it. The portal never becomes an icon. See [portal](portal.md).
- **Five apps.** Talk, Learn, Decide, Ask and Maps each live on a path of their own, `/talk/`, `/learn/`, `/decide/`, `/ask/` and `/maps/`, each with its icon, its cover, its bar and its button to add it to the phone, with no app store. Maps joined the first four on 2026-11-15. See [apps on one address](../design/apps.md) and [Maps](maps.md).
- **The same face.** The header, the bar at the bottom, the buttons, the cards, the type, the four notices and the button to add are the same in every app. What changes from one app to another is its cover and the shape of its home, made for what it does. See [visual identity](visual.md).
- **The community's name, at the top of every app, leads to the portal.** From the portal, every app is one tap away.
- **The same screens on every machine.** A machine with another version of the screens follows the journals but does not serve residents until it is updated.

## Nothing a person wrote is lost

What is written with no link to the community stays in the device's outbox, in sight, with the mark "Ainda não enviada", and goes out by itself when the link returns. A draft survives an error: a page that refuses a message shows it again with the text kept. **Proposed**; the outbox needs the device's part, which is not built.

## What waits says that it waits

What is counted, such as creating an account, changing a role or closing a vote, waits for the machines that decide it. When they cannot be reached, the screen says so at once, in one sentence: not now, why, and what happens next. Nothing sits in a hidden queue. **Built** for creating an account, which says not now within five seconds, and for changing a role, which today first waits about ten seconds for the change to arrive before it says not now. Saying it at once there is **Proposed**.

## Four notices, always the same

| Notice | When | What the person reads |
| --- | --- | --- |
| No link with the community | The device cannot reach any machine | "Sem ligação com a comunidade." What is already on the device can be read, and what is written goes to the outbox |
| Not sent yet | A message is in the outbox | The mark "Ainda não enviada" where the time would be |
| Arrived later | A message joined a conversation after others written later | The mark "Chegou depois" beside its time |
| Not now | Something counted cannot be done yet | "Agora não dá para…", why, and what happens next |

None of them mentions a machine. Each has its fixed sentence and its fixed place in every app, drawn in the sun color with dark text. The behavior behind each one is in [notices and behavior](behavior.md).

## The way in is the same on every device

The poster, the community certificate, the account and the passphrase, then the icon. The icon comes after the certificate, because a browser only installs a site served over a secure connection, and because the icon keeps the address it was added from.

## What is personal stays personal

Conversations between people, closed groups and the vault are encrypted on the device, with the passphrase, before they leave it. No screen of an administrator shows them. A child's protected conversation is the one exception, and only to the child's guardian. See [account](account.md) and [Talk](talk.md).

## Accessible as drawn

- Every touch target is at least 44 by 44 pixels. Main buttons are 54 pixels tall.
- Text has a contrast of at least 4.5 to 1 against its background, in light and in dark mode.
- Color never carries meaning alone: a state also has a word or a mark.
- Every control is a real button, link or field, with a name a screen reader reads, and the order of reading follows the order on the screen.
- A sheet over the page keeps the focus inside it and gives it back when it closes.
- Nothing depends on a gesture with no other way to do it.

## Language

The screens speak the community's language, Brazilian Portuguese first, in plain words a person uses every day. A screen says what happened and what the person can do, never a technical term. Times are written as people say them: "há 2 horas", "ontem", "seg". Every text lives in the locales, never in the code.

## Light and dark

Every screen exists in light and in dark mode, and follows the device's choice. Dark mode is the same palette turned around, described in [visual identity](visual.md). **Proposed**: the dark palette waits for the maintainer's visual approval.

## Not decided

- Whether the names under the icons stay Talk, Learn, Decide and Ask in a community that speaks Portuguese, or are translated.
- The end of the community's address. The maintainer wrote "minhacomunidade.earthler". Today the community certificate is limited to names that end in `.internal`, a name reserved for private networks that will never exist on the internet; a name ending in `.earthler` works inside the community's network without that guarantee.
