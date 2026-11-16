# Visual identity

The face of Earthler is the Terra direction: technology in the service of life, high technology in the middle of the woods. It is made of the mark the maintainer created, a palette of earth, leaf and sun, one typeface and a few pieces repeated on every screen. The kit that draws every screen of this folder, [`docs/screens/kit.css`](../screens/kit.css), is the reference in code.

## The mark

The mark is a sun and three leaves; the lettering is the name. Both were created by the maintainer and are never redrawn. When they appear together, the letters are 0.885 of the height of the mark and the gap between them is 0.2 of that height. They live in [`brand/`](../../brand), outside the code license: see [TRADEMARKS](../../TRADEMARKS.md).

The glyphs of the four apps are made of the pieces of the mark: the sun, the leaves with thin tips and the wide gaps between the shapes. The question mark of Ask has the round, thick stroke of the lettering. A new icon or button is born from the mark, never from a generic symbol. The maintainer approved the glyphs on 2026-11-15, on a white background with the leaf and sun colors.

## Palette

| Role | Light | Dark (proposed) | Used for |
| --- | --- | --- | --- |
| Background | `#F3EFE7` | `#15120E` | The page |
| Surface | `#FFFFFF` | `#221E18` | Cards, lists, the bar, sheets |
| Surface 2 | `#F3EFE7` | `#2E2820` | A field or a chip on a surface |
| Text | `#2A241C` | `#F3EFE7` | Text |
| Secondary text | `#5F5546` | `#CFC7B8` | Lines under a title, times, labels |
| Primary | `#025D2E` (hover `#014A25`) | `#01A26D` | Main buttons, the current tab, links |
| On primary | `#FFFFFF` | `#0E1510` | Text on a primary button |
| Leaf | `#01A26D` | `#01A26D` | Charts, progress, the glyphs |
| Sun | `#FDB504`, always with dark text | `#FDB504`, always with dark text | The notices, the button to add, what asks for the person's action |
| Soft | `#E3EBDF` | `#1E3328` | Quiet panels, the current tab's pad, chips |
| On soft | `#025D2E` | `#BFE3CF` | Text on soft |
| Lines | `#ECE6DA` | `#332C24` | Between rows |
| Track | `#DED6C7` | `#3D352C` | Progress bars, switches off |
| Border | `#CFC7B8` | `#4A4136` | Fields |

Dark mode turns the palette around instead of inventing colors: the background is the warm dark of the text color, surfaces step up from it, and the leaf green takes the place of the primary, which is too dark to read on a dark background. The sun keeps its dark text in both modes. Every pair of text and background keeps a contrast of at least 4.5 to 1. **Proposed**: the dark palette waits for the maintainer's visual approval.

## Type

Figtree, under the SIL Open Font License, weights 400 to 800. It travels with the system, so the screens look the same with no internet.

| Use | Size and weight |
| --- | --- |
| The title of a cover | 32 px, 800 |
| The name of an app in its header | 26 px, 800 |
| The title of an inner page | 20 px, 700 |
| A row's title | 16 px, 700 |
| Body | 16 to 17 px, 400 |
| A row's second line, labels | 14 px, 400 to 600 |
| Marks and chips | 12 to 13 px, 700 |

## Shapes

Cards have a radius of 20 px. Buttons are 54 px tall with a radius of 27 px; small buttons and chips are 44 px tall. Touch targets are never smaller than 44 px. Sheets rise from the bottom with a radius of 28 px.

## The pieces of every screen

| Piece | What it is |
| --- | --- |
| The community's header | On a page of no app: the mark and the community's name, which lead to the portal |
| The cover of an app | The app's header, in its own color, with its glyph drawn large and cut by the corner, its title and one line under it |
| The inner header | On a page drawn full inside an app, such as a conversation: a way back, the title and one line under it |
| The bar | At the bottom of an app: its three to five parts, the current one marked with a soft pad and its name in bold |
| The card | A surface with a radius of 20 px |
| The list | A card of rows, each row at least 62 px tall, with a line between rows |
| The button | Primary, soft or quiet; a button that cannot be undone is drawn with a border in the danger color and asks before it acts |
| The button to add | A soft pill with a sun holding a plus sign |
| The notice | The sun color with dark text, always the same sentence in the same place |
| The sheet | A panel that rises from the bottom over a veil, for a choice or for steps |

## The cover of each app

Each app has a face of its own, so a person knows where they are at a glance, while everything else stays the same in the four.

| App | Cover in light | Cover in dark | Its home starts with |
| --- | --- | --- | --- |
| Talk | Primary `#025D2E`, light text | `#025D2E` | The alerts and the conversations |
| Learn | Surface `#FFFFFF`, dark text | Surface `#221E18` | The question "O que você quer saber?" and the search |
| Decide | Soft `#E3EBDF`, dark text | Soft `#1E3328` | The deadlines and the person's vote |
| Ask | Text `#2A241C`, light text | `#0B0A08` | The question to the Mentor |

The glyph on the cover keeps its gaps in the cover's own color, as the mark keeps them in the background.

## The portal

The portal has no cover. It has the community's mark and name in the middle, the search under them, and the four apps as tiles, like the icons they become on the phone. See [portal](portal.md).
