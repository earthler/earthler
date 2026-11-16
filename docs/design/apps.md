# Four apps on one address

**Proposed, except for one proof on a real iPhone. None of it is built.** Today the system is one site, reached through the browser. This note is what the resident sees once Talk, Learn, Decide and Ask become apps. It holds with one machine as much as with many; how many machines stay the same behind it is in [many machines, one community](many-machines.md). The marks are the same as there: **built**, **proposed**, **to prove**, **inference**.

On 2026-11-15 the maintainer approved a fifth app, Maps, the community's map, at `/maps` under the same rules as the four. What it shows and how it behaves is in [Maps](../product/maps.md); the proof on the iPhone below was made with the first four.

## The four apps

The [README](../../README.md) lists four things residents will do: Talk, Learn, Decide and Ask. On 2026-11-13 the maintainer decided that each of them is an app on the phone, with its own name, its own icon and its own button to add it to the home screen. There is no app store: an icon on the home screen is a shortcut to the app in the browser, and it opens full screen, with no browser bar.

| App | What is inside |
| --- | --- |
| Talk | Conversations, calls and alerts |
| Learn | Subjects, lessons, wiki, collection and films |
| Decide | Decisions, the map and delegations |
| Ask | Questions to the Mentor, the answers, and what the collection is missing |

Each app has its own header and its own bar at the bottom. The four share the same colours, the same type, the same way of warning and the same sign in. The community's name at the top of each app leads to the other three.

It is still one program on the machine. Only the way it reaches the resident changes.

## One address, four paths

The four apps live at the same address, each on a path of its own, and no path sits inside another: `/talk`, `/learn`, `/decide` and `/ask`. **Proposed.**

Google recommends one address per app and advises against paths on one address ([building multiple PWAs on the same domain](https://web.dev/articles/building-multiple-pwas-on-the-same-domain)). The design keeps one address anyway. With separate addresses a person would sign in four times on any phone. With one, a phone that keeps data by address, as Android does, signs in once for the four.

An icon stays tied to the address it was added from. That is why every machine of a community answers at the community's one address, as [many machines](many-machines.md#connection) explains: the same icon then opens on whichever machine is near.

The page that the QR code on the community's poster opens is not an app. It shows the four apps, each with its own add button, and it does not become an icon. A resident never chooses a machine and never sees a machine's name or number. Machines are a matter for the administrator's screens.

## The add button

Each app has its own add button, on the poster's page and inside the app. **Proposed**, checked against each maker's documentation and not yet tested on Android.

- **Android, Chrome.** The button opens Chrome's own confirmation. In Portuguese, the words are "Instalar aplicativo" and "Instalar" (Install app, Install), taken from Chrome's translation files in the Chromium source.
- **iPhone, Safari.** Apple does not let a site add itself, so the button shows the steps in Safari. In Portuguese: "Compartilhar", then "Adicionar à Tela de Início", then keep "Abrir como App da Web" on, then "Adicionar" (Share, Add to Home Screen, Open as Web App, Add). The words come from Apple's guide in Portuguese, [Transforme um site em um app no Safari do iPhone](https://support.apple.com/pt-br/guide/iphone/iphea86e5236/ios). Since iOS 26, a site added to the Home Screen opens as a web app by default ([iOS PWA compatibility](https://firt.dev/notes/pwa-ios/)).

The way in is the same on every device: the poster, the community certificate, the account and the passphrase, then the icon. The icon comes after the certificate because Chrome only offers to install a site served over a secure connection, and because the icon keeps the address it was added from, which has to be the secure one.

## The same in every place

| Rule | What it means on the screen |
| --- | --- |
| The same screens on every machine | The machines of a community show the same version of the screens and the same words. A machine with another version follows the journals, but does not serve residents until it is updated |
| Four notices, always the same | Each with a fixed sentence, in the same place, in every app (below) |
| Nothing a person wrote is lost | What was written with no link stays in the device's outbox, in sight, and goes out by itself |
| What waits says that it waits | Creating an account, changing a role or closing a vote answers at once, in one sentence: not now, and why. Nothing sits in a hidden queue |
| The same face | The colours, the type, the cards and the buttons that the screens already use. A notice uses the sun yellow with dark text |

## The four notices

| Notice | When it shows | What the person can do |
| --- | --- | --- |
| No link with the community | The device cannot reach the community | Read what is already there and write. What they write goes out when the link returns, and the notice goes away by itself |
| Not sent yet | A message is in the outbox | It stays in the conversation, with the mark where the time would be. When it goes out, the mark becomes the time |
| Arrived later | A message joined the conversation after others that were written later | It goes where its writing time puts it, with the mark, so nobody thinks they missed something that was already there |
| Not now | Something that is counted, such as closing a vote, creating an account or changing a role, cannot be done yet | The answer comes at once: not now, why, and what happens next. Nothing that was already done is lost |

None of them mentions a machine. **Proposed**; the exact sentences belong to the [locales](../../locales) when the screens are built.

## What a real iPhone showed

On 2026-11-15, one iPhone with Safari 27.0.1 opened test pages served over plain HTTP from a computer on the same Wi-Fi, with nothing of the system in them. The person signed in with a test account on the poster's page, added the four apps to the Home Screen and opened each one from its icon. Each icon reported what it found. The test pages are not in this repository.

Two of the apps also carried a value in their start URL, each in a different way, to see whether a sign in could travel with the icon by the address if Safari did not copy it.

| What was measured | What the iPhone did |
| --- | --- |
| The sign in goes with the icon | Yes, in all four. The session cookie set by the server reached every icon, and each icon started with the same cookies as Safari |
| The rest of the browser's storage goes with the icon | No. What the page kept in Safari outside cookies reached no icon, and each icon got storage of its own, different from the other three and from Safari's |
| The icons see each other afterwards | No. A mark left inside one icon appeared in no other and not in Safari. The copy of the cookies happens once, when the icon is added |
| A value in the start URL that the server wrote in the app's manifest | Arrived |
| A value in the URL of the page itself, with no manifest | Arrived |
| A manifest changed by the page after it opened | Did not arrive. Safari kept the manifest the page had when it opened |

The first row agrees with the compatibility table that lists login cookies as copied to the installed app since iOS 17.2, and the second with the line that says storage is not shared with the browser ([iOS PWA compatibility](https://firt.dev/notes/pwa-ios/)). The icons went on the Home Screen over plain HTTP, so on the iPhone a secure connection is not what allows an icon; it is what the rest of the design needs.

The limits: one device, one version of Safari, one run per icon, over HTTP and without the community certificate. The person signed in before adding the icons, which is the order of the design; an icon added before signing in was not tested. About twenty minutes later two of the icons opened again, still signed in, with the same storage.

So on the iPhone the sign in does not repeat: whoever signed in on the poster's page and then added an icon arrives signed in. What is personal does not follow: on the iPhone, the home of what is personal on the device is one per icon. Personal conversations live in Talk, so the key has to reach the Talk icon. On 2026-11-15 the maintainer decided that the Talk icon asks for the passphrase once, inside the icon, and fetches the encrypted package from a machine. The passphrase never passes through the machine.

The same test pages then showed that screen on the iPhone: Talk opened with the open channels in sight, asked for the passphrase to open the personal conversations, and opened them. That Talk then opens straight into the conversations, without asking again, has not shown in the records yet; what they show is the same storage in that icon across three openings in twenty minutes.

Learn and Decide keep nothing personal in the design and would ask for nothing. Ask keeps the person's recent answers on the device, in its own icon. **Inference**, to be checked when each app's screens are settled.

## Not decided, not proven

- Whether the names under the icons stay Talk, Learn, Decide and Ask in a community that speaks Portuguese, or are translated.
- The add button and the four icons on Android.
- An icon left unopened for more than seven days: whether it still holds the key.
- An icon added before signing in.
- The drawn screens and the icons themselves are not in this repository yet.
