"""Pull the numbered examples out of the rendered GitHub Flavored Markdown
specification (the HTML page) into the JSON shape CommonMark publishes:
a list of {markdown, html, example, section}.

usage: python3 -I gfm_extract.py gfm.html > gfm.json
"""
import html
import json
import re
import sys

page = open(sys.argv[1], encoding="utf-8").read()

heading = re.compile(r'<h[12] id="[^"]*"[^>]*>(.*?)</h[12]>', re.S)
example = re.compile(
    r'<div class="example" id="example-(\d+)">.*?'
    r'<pre><code class="language-markdown">(.*?)</code></pre>.*?'
    r'<pre><code class="language-html">(.*?)</code></pre>',
    re.S,
)


def plain(s):
    s = s.replace('<span class="space"> </span>', " ")
    s = re.sub(r"<[^>]*>", "", s)
    s = html.unescape(s)
    return s.replace("→", "\t")


def title(s):
    s = re.sub(r"<span class=\"number\">.*?</span>", "", s, flags=re.S)
    s = re.sub(r"<[^>]*>", "", s)
    return " ".join(html.unescape(s).split())


marks = [(m.start(), title(m.group(1))) for m in heading.finditer(page)]
out = []
for m in example.finditer(page):
    section = ""
    for pos, name in marks:
        if pos > m.start():
            break
        section = name
    out.append(
        {
            "markdown": plain(m.group(2)),
            "html": plain(m.group(3)),
            "example": int(m.group(1)),
            "section": section,
        }
    )
json.dump(out, sys.stdout, ensure_ascii=False, indent=1)
sys.stdout.write("\n")
