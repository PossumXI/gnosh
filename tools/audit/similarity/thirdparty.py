#!/usr/bin/env python3
"""thirdparty.py - are the files that CARRIED.md and PROVENANCE.md call
third-party what they are said to be?

  1. gno's standard library (internal/gnovm/stdlibs) and gno's B+tree
     (internal/gnovm/klashlibs/klash/bptree/v0): every .gno file is compared,
     byte for byte, with the same file of the gno module at the version the
     tree's VERSION and SOURCE files name; the licence files are compared
     with gno's and Go's.
  2. The Unicode tables and test files (internal/text): the tree's own
     generator is run again, fetching the Unicode Character Database files it
     names from unicode.org and checking each against the SHA-256 it records;
     its output is compared with tables.go, and the test files it fetches
     with the ones in testdata.
  3. The Markdown specification examples (internal/markdown/testdata/spec):
     CommonMark's spec.json is fetched and compared; the GFM page is fetched
     and the tree's own extractor is run on it.
  4. The HTML entity table (internal/markdown/entity.go): the tree's own
     generator is run again and compared.

Steps 2 and 3 need the network (unicode.org, spec.commonmark.org,
github.github.com) and a Go toolchain; pass --offline to leave them out.
Output: WORK/thirdparty.txt, and the same on standard output.
"""
import hashlib
import os
import re
import subprocess
import sys
import urllib.request

import common as X

OUT = []


def say(s):
    OUT.append(s)
    print(s, flush=True)


def sha(data):
    return hashlib.sha256(data).hexdigest()


def read(path):
    with open(path, 'rb') as f:
        return f.read()


def fetch(url):
    req = urllib.request.Request(url, headers={'User-Agent': 'similarity-audit'})
    with urllib.request.urlopen(req, timeout=60) as r:
        return r.read()


def gno():
    tree = X.NEW_TREE
    std = os.path.join(tree, 'internal/gnovm/stdlibs')
    version = read(os.path.join(std, 'VERSION')).decode().strip()
    mod = X.moddir('github.com/gnolang/gno@' + version)
    say('gno standard library: VERSION says %s; module %s' % (version, 'found in the module cache' if mod else 'NOT in the module cache'))
    if not mod:
        return
    n = same = 0
    diff = []
    for p in X.walk_files(std, exts=('.gno',)):
        rel = os.path.relpath(p, std)
        q = os.path.join(mod, 'gnovm/stdlibs', rel)
        n += 1
        if os.path.exists(q) and read(p) == read(q):
            same += 1
        else:
            diff.append(rel)
    theirs = sum(1 for _ in X.walk_files(os.path.join(mod, 'gnovm/stdlibs'), exts=('.gno',)))
    say('  .gno files in the tree: %d; byte-identical to the module\'s gnovm/stdlibs: %d; different or missing there: %d %s' % (n, same, len(diff), diff[:5]))
    say('  .gno files in the module\'s gnovm/stdlibs: %d (the tree leaves out %d)' % (theirs, theirs - n))
    other = [os.path.relpath(p, std) for p in X.walk_files(std) if not p.endswith('.gno')]
    say('  other files there: %s' % ', '.join(other))
    lic = read(os.path.join(std, 'LICENSE.md'))
    glic = read(os.path.join(mod, 'LICENSE.md'))
    say('  LICENSE.md ends with gno\'s LICENSE.md, whole: %s (preface: %d bytes)' % (lic.endswith(glic), len(lic) - len(glic)))
    golic = os.path.join(os.path.dirname(X.GOROOT_SRC), 'LICENSE')
    if os.path.exists(golic):
        say('  LICENSE-GO is Go\'s LICENSE: %s' % (read(os.path.join(std, 'LICENSE-GO')) == read(golic)))
    bp = os.path.join(tree, 'internal/gnovm/klashlibs/klash/bptree/v0')
    src = read(os.path.join(bp, 'SOURCE')).decode()
    say('gno B+tree: SOURCE says: %s' % ' '.join(src.split()))
    n = same = 0
    for p in X.walk_files(bp, exts=('.gno',)):
        q = os.path.join(mod, 'examples/gno.land/p/nt/bptree/v0', os.path.basename(p))
        n += 1
        same += 1 if os.path.exists(q) and read(p) == read(q) else 0
    blic = read(os.path.join(bp, 'LICENSE.md'))
    say('  .gno files: %d; byte-identical to the module\'s examples/gno.land/p/nt/bptree/v0: %d; LICENSE.md ends with gno\'s: %s' % (n, same, blic.endswith(glic)))


def unicode_data():
    tree = X.NEW_TREE
    d = os.path.join(X.WORK, 'ucd')
    t = os.path.join(X.WORK, 'ucd-testdata')
    os.makedirs(d, exist_ok=True)
    os.makedirs(t, exist_ok=True)
    env = dict(os.environ, PATH='/usr/local/go/bin:' + os.environ.get('PATH', ''), GOFLAGS='-mod=readonly', GOTOOLCHAIN='local')
    r = subprocess.run(['go', 'run', './internal/text/gen', '-fetch', '-dir', d, '-testdata', t], cwd=tree, env=env, capture_output=True)
    if r.returncode != 0:
        say('Unicode tables: the generator failed: %s' % r.stderr.decode()[-400:])
        return
    mine = read(os.path.join(tree, 'internal/text/tables.go'))
    say('Unicode tables: the tree\'s generator, run again on files fetched from unicode.org (each checked against the SHA-256 it records), writes tables.go byte for byte: %s (%d bytes)' % (r.stdout == mine, len(mine)))
    for f in sorted(os.listdir(t)):
        p = os.path.join(tree, 'internal/text/testdata', f)
        say('  testdata/%s is the file fetched from unicode.org: %s (sha256 %s)' % (f, os.path.exists(p) and read(p) == read(os.path.join(t, f)), sha(read(os.path.join(t, f)))[:16]))
    extra = set(os.listdir(os.path.join(tree, 'internal/text/testdata'))) - set(os.listdir(t))
    say('  files in testdata the generator does not fetch: %s' % (sorted(extra) or 'none'))
    lic = read(os.path.join(tree, 'internal/text/LICENSE-UNICODE.txt')).decode('utf-8', 'replace')
    say('  LICENSE-UNICODE.txt: %d bytes, names the Unicode License V3: %s' % (len(lic), 'UNICODE LICENSE V3' in lic.upper()))


def markdown_spec():
    tree = X.NEW_TREE
    d = os.path.join(tree, 'internal/markdown/testdata/spec')
    notice = read(os.path.join(d, 'NOTICE.md')).decode()
    shas = re.findall(r'`([0-9a-f]{64})`', notice)
    cm = read(os.path.join(d, 'commonmark-0.31.2.json'))
    say('CommonMark examples: sha256 %s; the NOTICE records it: %s' % (sha(cm)[:16], sha(cm) in shas))
    try:
        got = fetch('https://spec.commonmark.org/0.31.2/spec.json')
        say('  byte-identical to https://spec.commonmark.org/0.31.2/spec.json fetched now: %s' % (got == cm))
    except Exception as e:  # noqa: BLE001
        say('  could not fetch spec.json: %s' % e)
    gfm = read(os.path.join(d, 'gfm-0.29.json'))
    say('GFM examples: sha256 %s; the NOTICE records it: %s' % (sha(gfm)[:16], sha(gfm) in shas))
    try:
        page = fetch('https://github.github.com/gfm/')
        fn = os.path.join(X.WORK, 'gfm.html')
        with open(fn, 'wb') as f:
            f.write(page)
        say('  the GFM page fetched now has the SHA-256 the NOTICE records: %s' % (sha(page) in shas))
        r = subprocess.run([sys.executable, '-I', os.path.join(d, 'gfm_extract.py'), fn], capture_output=True)
        say('  the tree\'s extractor, run on that page, writes gfm-0.29.json byte for byte: %s' % (r.returncode == 0 and r.stdout == gfm))
    except Exception as e:  # noqa: BLE001
        say('  could not fetch the GFM page: %s' % e)


def entities():
    tree = X.NEW_TREE
    d = os.path.join(tree, 'internal/markdown')
    env = dict(os.environ, PATH='/usr/local/go/bin:' + os.environ.get('PATH', ''))
    r = subprocess.run('%s -I entity_gen.py | gofmt' % sys.executable, shell=True, cwd=d, env=env, capture_output=True)
    say('HTML entity table: the tree\'s generator, run again, writes entity.go byte for byte: %s' % (r.returncode == 0 and r.stdout == read(os.path.join(d, 'entity.go'))))


if __name__ == '__main__':
    gno()
    entities()
    if '--offline' not in sys.argv:
        unicode_data()
        markdown_spec()
    with open(os.path.join(X.WORK, 'thirdparty.txt'), 'w') as f:
        f.write('\n'.join(OUT) + '\n')
