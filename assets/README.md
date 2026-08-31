# assets

The artwork and typefaces the page is built from, and the script that sizes
them.

## Running it

```
python assets/build.py
```

It writes `internal/server/img/` and `internal/server/fonts.css`. Both of those
are committed, because the binary embeds them and a clone has to build with
nothing but Go installed. This script only needs running when the artwork or
the character set changes.

It needs Pillow and fontTools:

```
pip install pillow fonttools brotli
```

## What is here

`icon.png` and `logo.png` are the original pixel art, 1.7 MB and 155 KB. The
page needs a fraction of that, so they are cropped, resized and quantised down
to 36 KB together. Quantising is invisible on this artwork: pixel art has a
small palette to begin with, and almost all of the colours in these files are
anti-aliasing.

`fonts/` holds the two typefaces as downloaded. Fraunces is variable across four
axes and gets pinned to the one instance the page uses; JetBrains Mono ships as
static weights already. Both are subset to Latin and the punctuation people
actually type, which takes them from 888 KB to 67 KB.

Both faces are under the SIL Open Font License, and the licence text sits beside
them.

## Why the fonts are embedded

Fetching them from a font host would be simpler. It would also mean a request to
somebody else's server every time the page opens, and this page tells the reader
that nothing was sent anywhere. It would mean the design quietly falls apart with
the network unplugged, which is a normal way to run a tool that reads local
history.
