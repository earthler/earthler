# Drawn screens

Every screen of [the product](../product) drawn as a phone of 390 by 844 CSS pixels, in light and in dark mode, from one HTML file each.

- `src/<area>/<screen>.html` is the drawing. It uses [`kit.css`](kit.css) for the palette, the type and the pieces of every screen, and [`kit.js`](kit.js) for the glyphs of the apps, the line icons, the mark and dark mode.
- `png/light/<area>-<screen>.png` and `png/dark/<area>-<screen>.png` are the pictures the documents show.

To see a screen, open its file in a browser; add `#dark` to the address for dark mode. To draw the pictures again, with Google Chrome installed:

```bash
sh docs/screens/render.sh
```

```bash
sh docs/screens/render.sh talk
```

The second form draws one area. The pictures are at twice the size, as a phone shows them. When Python 3 with Pillow is installed, the script then keeps 256 colours in each picture with [`shrink.py`](shrink.py), which looks the same and weighs about a third. The drawing runs Chrome with a profile of its own, thrown away at the end, so it never opens a window in a Chrome that is already running.

The names, messages and numbers on the screens are examples, to show the design. The community in them is called Vila Aurora.

## Rules for a new drawing

- One file per screen, in the folder of its area, named for what it is: `conversation.html`, not `screen3.html`.
- Only the pieces of the kit. A new piece goes in `kit.css`, described in [visual identity](../product/visual.md).
- Colors only through the kit's variables, so dark mode follows by itself.
- Icons through `<i data-icon="name">`, glyphs through `<span data-glyph="app">`, the mark through `<img data-mark alt="">`. Never a picture of an icon, never an emoji.
- Real buttons, links and fields, with names a screen reader reads.
- No status bar of a phone: the drawing starts where the page starts.

## The license of the drawings

The HTML, the kit and the pictures are under the code license, except the Earthler mark and the glyphs of the four apps, which `kit.js` draws and which belong to the Earthler brand: see [TRADEMARKS](../../TRADEMARKS.md). The Figtree font in `fonts/` is under the SIL Open Font License, in `fonts/OFL.txt`.
