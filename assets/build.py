"""Produce the web artwork from the originals beside this file.

Run from the repository root:

    python assets/build.py

The originals are large renders, 1.7 MB and 155 KB, and a page needs a
fraction of that. This is kept as a script rather than done once by hand so
the artwork can be changed and the web copies regenerated to match.
"""

import os
import sys

from PIL import Image

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)

import fonts
# The web files live inside the server package because Go can only embed
# what sits beside the source that embeds it.
OUT = os.path.join(os.path.dirname(HERE), "internal", "server", "img")


def save(im, name, colours):
    """Write a quantised copy.

    Pixel art has a small palette to begin with, so most of the colours in
    these files are anti-aliasing. Quantising takes the logo from 118 KB to
    24 KB for a mean channel error of 2 out of 255, which is not visible.
    """
    path = os.path.join(OUT, name)
    im.quantize(colors=colours, method=Image.FASTOCTREE, dither=Image.NONE).save(
        path, optimize=True
    )
    print(f"  {name:16} {os.path.getsize(path) / 1024:5.1f} KB")


def main():
    os.makedirs(OUT, exist_ok=True)
    print("building web assets")

    logo = Image.open(os.path.join(HERE, "logo.png")).convert("RGBA")
    logo = logo.crop(logo.getbbox())
    width = 440
    save(
        logo.resize((width, round(logo.height * width / logo.width)), Image.LANCZOS),
        "logo.png",
        256,
    )

    # The icon keeps the green ground it was drawn on. Lifting it left the
    # mark floating, and the vine reads as a badge rather than as loose
    # branches when the colour behind it is part of the artwork.
    icon = Image.open(os.path.join(HERE, "icon.png")).convert("RGBA")
    icon = icon.crop(icon.getbbox())
    for size in (32, 180):
        save(icon.resize((size, size), Image.LANCZOS), f"icon-{size}.png", 128)

    css = fonts.build()
    dest = os.path.join(os.path.dirname(HERE), "internal", "server", "fonts.css")
    with open(dest, "w", encoding="utf-8", newline="\n") as f:
        f.write(css)

    total = sum(os.path.getsize(os.path.join(OUT, f)) for f in os.listdir(OUT))
    print(f"  {'artwork':16} {total / 1024:5.1f} KB")


if __name__ == "__main__":
    main()
