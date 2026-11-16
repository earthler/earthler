"""Makes the pictures in png/ lighter by keeping 256 colours.

The screens are flat colours and text, so the result looks the same and
weighs about a third. Needs Python 3 with Pillow. render.sh runs it when
Pillow is there; it can also be run alone: python3 docs/screens/shrink.py
"""

import pathlib
import sys

from PIL import Image

here = pathlib.Path(__file__).resolve().parent
names = sys.argv[1:]

for path in sorted(here.glob("png/*/*.png")):
    if names and path.stem not in names:
        continue
    image = Image.open(path)
    if image.mode == "P":
        continue
    small = image.convert("RGB").quantize(
        colors=256, method=Image.Quantize.MEDIANCUT, dither=Image.Dither.NONE
    )
    small.save(path, optimize=True)
