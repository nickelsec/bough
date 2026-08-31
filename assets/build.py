"""Produce the web artwork from the originals beside this file.

Run from the repository root:

    python assets/build.py

The originals are large renders, 1.7 MB and 155 KB, and a page needs a
fraction of that. This is kept as a script rather than done once by hand so
the artwork can be changed and the web copies regenerated to match.
"""

import os
from collections import deque

from PIL import Image

HERE = os.path.dirname(os.path.abspath(__file__))
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


def sage(c):
    """Report whether a pixel belongs to the icon's background.

    The background is a vignette that brightens from around (155,160,105) at
    the border to (184,191,132) near the mark, so matching one colour will not
    follow it. What separates it from the artwork is that sage stays light with
    far more green than blue, while the leaves are a harsher yellow-green and
    the bark and outline are dark.
    """
    r, g, b = c[:3]
    return (
        130 <= r <= 200
        and 135 <= g <= 205
        and 80 <= b <= 145
        and g >= r - 8
        and 40 <= g - b <= 80
    )


def lift_background(im):
    """Clear the background, working inward from the edges.

    Following it in from the border rather than matching by colour means an
    enclosed leaf of a similar shade cannot be eaten by mistake.
    """
    w, h = im.size
    px = im.load()
    seen = bytearray(w * h)

    queue = deque()
    for x in range(w):
        queue.append((x, 0))
        queue.append((x, h - 1))
    for y in range(h):
        queue.append((0, y))
        queue.append((w - 1, y))

    while queue:
        x, y = queue.popleft()
        i = y * w + x
        if seen[i] or not sage(px[x, y]):
            continue
        seen[i] = 1
        px[x, y] = (0, 0, 0, 0)
        if x:
            queue.append((x - 1, y))
        if x < w - 1:
            queue.append((x + 1, y))
        if y:
            queue.append((x, y - 1))
        if y < h - 1:
            queue.append((x, y + 1))
    return im


def main():
    os.makedirs(OUT, exist_ok=True)
    print("building web artwork")

    logo = Image.open(os.path.join(HERE, "logo.png")).convert("RGBA")
    logo = logo.crop(logo.getbbox())
    width = 440
    save(
        logo.resize((width, round(logo.height * width / logo.width)), Image.LANCZOS),
        "logo.png",
        256,
    )

    # The icon ships with its background baked in, which would show as a square
    # behind the mark in a browser tab.
    icon = lift_background(Image.open(os.path.join(HERE, "icon.png")).convert("RGBA"))
    icon = icon.crop(icon.getbbox())
    for size in (32, 180):
        save(icon.resize((size, size), Image.LANCZOS), f"icon-{size}.png", 128)

    total = sum(os.path.getsize(os.path.join(OUT, f)) for f in os.listdir(OUT))
    print(f"  {'total':16} {total / 1024:5.1f} KB")


if __name__ == "__main__":
    main()
