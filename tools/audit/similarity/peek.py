#!/usr/bin/env python3
"""peek.py - show both sides of a match on the terminal, for the hand review.

It prints the new tree's lines and the corpus file's lines side by side, to
the terminal only. Nothing it prints belongs in a file of the public
repository: one side is Charm's code.

  peek.py pair NEWPATH A-B SOURCE C-D     the new tree's NEWPATH lines A-B and
                                          the corpus file SOURCE lines C-D
                                          (SOURCE as the scans show it, e.g.
                                          base:internal/x/y.go)
  peek.py tok CORPUS FORM WHAT N [SKIP]   the N files of new code with the
                                          longest runs in tok-matches.tsv
                                          (FORM exact|abstract, WHAT
                                          run|varied), each with both sides
  peek.py lines NEWPATH CORPUS            the line runs of one file, both sides
"""
import csv
import os
import sys

import common as X
from common import T

_src = None


def sources():
    global _src
    if _src is None:
        _src = {}
        for fn in ('crush-ext.tsv', 'libs.tsv', 'old.tsv', 'deps.tsv'):
            p = os.path.join(X.WORK, 'lists', fn)
            if os.path.exists(p):
                for l in open(p):
                    a, b = l.rstrip('\n').split('\t')
                    _src.setdefault(b, a)
                    _src.setdefault(b.rsplit('@', 1)[0], a)  # as the line scan names it
    return _src


def rng(s):
    a, b = s.split('-')
    return int(a), int(b)


def cut(lines, a, b, width, pad=2, limit=40):
    a = max(1, a - pad)
    b = min(len(lines), b + pad)
    out = ['%5d| %s' % (i, lines[i - 1].replace('\t', '  ')[:width]) for i in range(a, b + 1)]
    if len(out) > limit:
        out = out[:limit // 2] + ['  ...'] + out[-limit // 2:]
    return out


def pair(newpath, nr, source, sr, width=88):
    mine = X.read_text(os.path.join(X.NEW_TREE, newpath)) or []
    theirs = X.read_text(sources().get(source, '')) or ['(source not found: %s)' % source]
    left = cut(mine, *rng(nr), width)
    right = cut(theirs, *rng(sr), width)
    print('-' * 8, 'NEW', newpath, nr, '|', source, sr)
    for i in range(max(len(left), len(right))):
        l = left[i] if i < len(left) else ''
        r = right[i] if i < len(right) else ''
        print('%-*s || %s' % (width + 7, l, r))


def group_of():
    g = {}
    for l in open(os.path.join(X.WORK, 'lists', 'targets.tsv')):
        p = l.rstrip('\n').split('\t')
        if p[0] == 'new-tree':
            g[p[4]] = p[1]
    return g


def tok(corpus, form, what, n, skip=0):
    g = group_of()
    best = {}
    for r in csv.DictReader(open(os.path.join(X.WORK, 'tok-matches.tsv')), delimiter='\t', quoting=csv.QUOTE_NONE):
        if r['set'] != 'new-tree' or g.get(r['path']) != 'new' or r['corpus'] != corpus or r['form'] != form or r['what'] != what:
            continue
        if r['path'] not in best or int(r['tokens']) > int(best[r['path']]['tokens']):
            best[r['path']] = r
    top = sorted(best.values(), key=lambda r: -int(r['tokens']))[skip:skip + n]
    for r in top:
        print('\n==== %s tokens (%s distinct windows), %s %s vs %s, found in %s corpus files' % (
            r['tokens'], r['distinct_windows'], form, what, corpus, r['sources']))
        pair(r['path'], r['target_lines'], r['source'], r['source_lines'])


def lines(newpath, corpus):
    import linescan
    C, tier = linescan.load(corpus)
    mine = X.read_text(os.path.join(X.NEW_TREE, newpath))
    r, det = T.scan_lines(mine, C, detail=True)
    _, runs = linescan.raw(mine, C, tier)
    for a, b, n, src in runs:
        theirs = X.read_text(sources().get(src, '')) or []
        # find the run's first four content lines in the source
        want = [T.norm(l) for l in mine[a - 1:b] if len(T.norm(l)) >= T.MIN_CONTENT][:T.K]
        nl, nos = T.norm_lines(theirs)
        content = [(x, no) for x, no in zip(nl, nos) if len(x) >= T.MIN_CONTENT]
        at = None
        for i in range(len(content) - len(want) + 1):
            if [c[0] for c in content[i:i + len(want)]] == want:
                at = content[i][1]
                break
        print('\n==== line run of %d content lines' % n)
        pair(newpath, '%d-%d' % (a, b), src, '%d-%d' % (at or 1, (at or 1) + (b - a)))
    print('significant single lines that stand in the corpus:')
    for i in det[tier]['sigmatch']:
        print('   %5d| %s' % (i, mine[i - 1][:150]))


if __name__ == '__main__':
    c = sys.argv[1]
    if c == 'pair':
        pair(*sys.argv[2:6])
    elif c == 'tok':
        tok(sys.argv[2], sys.argv[3], sys.argv[4], int(sys.argv[5]), int(sys.argv[6]) if len(sys.argv) > 6 else 0)
    elif c == 'lines':
        lines(sys.argv[2], sys.argv[3])
