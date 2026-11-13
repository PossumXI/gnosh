# Specification examples in this directory

These two files are the numbered examples of the Markdown specifications,
used by the tests to check that the renderer loses no text, reads inline
and block syntax as the specifications do, and never panics. They are not
gnosh's.

| file | what it is | from | SHA-256 |
|---|---|---|---|
| `commonmark-0.31.2.json` | The examples of the CommonMark Spec, version 0.31.2, as its authors publish them. Unchanged. | https://spec.commonmark.org/0.31.2/spec.json (fetched 2026-11-13) | `d431b29d97b6f73e69d547109cf5081578fac931e72afe95639ebe766c1b2a20` |
| `gfm-0.29.json` | The examples of the GitHub Flavored Markdown Spec, version 0.29-gfm (2019-04-06), in the same shape. Extracted by `gfm_extract.py` from the specification's own page, whose SHA-256 was `b153d814fdfc8624bb6da7449162c1cd707a637f7d1c1b636eb44b9cf63fa220`. | https://github.github.com/gfm/ (fetched 2026-11-13) | `8287925e19c1d29da623fb68c5e4ade3b891e7ee63155bf0cbea30267f536a03` |

Licence: both specifications are licensed under the Creative Commons
Attribution-ShareAlike 4.0 International licence (CC BY-SA 4.0,
https://creativecommons.org/licenses/by-sa/4.0/). The CommonMark Spec is
© John MacFarlane; the GitHub Flavored Markdown Spec is GitHub's, based on
the CommonMark Spec. The examples here are copies of parts of those
documents and stay under that licence.

`gfm_extract.py` is gnosh's own. To make `gfm-0.29.json` again:

    curl -sSL -o gfm.html https://github.github.com/gfm/
    python3 -I gfm_extract.py gfm.html > gfm-0.29.json

Only the specifications were used. No Markdown implementation was read to
write this package.
