#!/usr/bin/env python3
"""linescan.py - the line measures, for every file in lists/targets.tsv,
against each corpus.

The measures are the earlier inventory's, computed by its own code
(taintscan.scan_lines), so that its thresholds carry over:

  sigd        the file's distinct significant lines (25 characters or more and
              12 letters or digits after whitespace is normalised and the
              brand names are folded, not an import-path line, and not a line
              that Go's own source, golang.org/x, gno, testify, cobra or pflag
              also has)
  share       how many of those also stand in the corpus, as a percentage
  run         the longest run of 4 or more consecutive content lines (12
              characters or more) that stand consecutively, in the same
              order, in one corpus file, counted when the run holds at least
              2 significant lines
  run_lines   lines in such runs, in all
  lrun        the longest run of 4 or more consecutive lines that each stand
              somewhere in the corpus (any file), with at least one of them
              significant

and, added here because the "significant" filter leaves nothing of a file of
Go's standard library (every line of it is in the generic reference), the
same without that filter:

  rawd        distinct lines of 25 characters or more and 12 letters or
              digits that are not import-path lines
  rawshare    how many of those stand in the corpus, as a percentage
  rawrun      the longest run of consecutive content lines standing
              consecutively in one corpus file, whether or not its lines are
              significant, counted when at least 4 of its lines are not
              imports or a package clause
  rawrun_lines  lines in such runs, in all

  linescan.py scan [corpus ...]         -> WORK/lines.tsv (all corpora by default)
  linescan.py show PATH CORPUS          the runs of one file of the new tree,
                                        with both sides' line numbers
"""
import collections
import os
import pickle
import re
import sys
import time

import common as X
from common import T

PKG = re.compile(r'^package \w+$')
COLS = ['lines', 'content', 'sig', 'sigd', 'sigd_match', 'share', 'run', 'run_n', 'run_lines', 'run_pct',
        'lrun', 'lrun_lines', 'rawd', 'rawd_match', 'rawshare', 'rawrun', 'rawrun_lines', 'best_src_n', 'best_src', 'top']

_main = None


def main_corpus():
    global _main
    if _main is None:
        _main = T.load_corpus()
    return _main


def load(name):
    """A corpus by name: (Corpus, tier). core and ext are the two tiers of the
    main one; libs, old and deps are corpora of their own."""
    if name in ('core', 'ext'):
        return main_corpus(), name
    C = T.Corpus()
    with open(os.path.join(X.WORK, 'corpus-%s.pkl' % name), 'rb') as f:
        C.__dict__.update(pickle.load(f))
    C.generic = main_corpus().generic
    return C, 'core'


def raw(lines, C, tier):
    lineset = C.core_lines if tier == 'core' else C.ext_lines
    shmap = C.core_sh if tier == 'core' else C.ext_sh
    nl, nos = T.norm_lines(lines)
    hs = [T.h(n) for n in nl]
    longd = {}
    for n, x in zip(nl, hs):
        if len(n) >= T.MIN_SIG and len(T.ALNUM.findall(n)) >= T.MIN_SIG_ALNUM and not T.IMPORT_LINE.match(n):
            longd[x] = True
    match = sum(1 for x in longd if x in lineset)
    cidx = [i for i in range(len(nl)) if len(nl[i]) >= T.MIN_CONTENT]
    cov = [False] * len(cidx)
    hit = [None] * len(cidx)
    for j in range(len(cidx) - T.K + 1):
        pid = shmap.get(hash(tuple(hs[cidx[t]] for t in range(j, j + T.K))))
        if pid is not None:
            for t in range(j, j + T.K):
                cov[t] = True
                if hit[t] is None:
                    hit[t] = pid
    runs = []
    for a, b in T.runs_of(cov):
        body = [nl[cidx[t]] for t in range(a, b)]
        nonimp = [x for x in body if not T.IMPORT_LINE.match(x) and not PKG.match(x) and x != 'import (']
        if len(nonimp) >= 4:
            runs.append((nos[cidx[a]], nos[cidx[b - 1]], b - a, C.paths[hit[a]]))
    return dict(rawd=len(longd), rawd_match=match,
                rawshare=round(100.0 * match / len(longd), 1) if longd else 0.0,
                rawrun=max((r[2] for r in runs), default=0),
                rawrun_lines=sum(r[2] for r in runs)), runs


def measure(lines, C, tier):
    r = T.scan_lines(lines, C)
    w, _ = raw(lines, C, tier)
    out = dict(lines=r['lines'], content=r['content'], sig=r['sig'], sigd=r['sigd'],
               sigd_match=r[tier + '_sigd_match'], share=r[tier + '_sigd_pct'],
               run=r[tier + '_srun_longest'], run_n=r[tier + '_srun_n'], run_lines=r[tier + '_srun_lines'],
               run_pct=r[tier + '_srun_pct'], lrun=r[tier + '_lrun_longest'], lrun_lines=r[tier + '_lrun_lines'],
               best_src_n=r['best_src_n'], best_src=r['best_src'], top=r[tier + '_top'])
    out.update(w)
    return out


def targets():
    rows = []
    for l in open(os.path.join(X.WORK, 'lists', 'targets.tsv')):
        p = l.rstrip('\n').split('\t')
        if len(p) == 5 and p[2] != 'binary':
            rows.append(p)
    return rows


def scan(names):
    t0 = time.time()
    rows = targets()
    texts = {}
    for s, g, k, path, shown in rows:
        lines = X.read_text(path)
        if lines is not None:
            texts[path] = lines
    fn = os.path.join(X.WORK, 'lines.tsv')
    keep = []
    if os.path.exists(fn) and names != ALL:
        # keep the rows of the corpora not being scanned again
        for l in open(fn):
            if l.split('\t')[5] not in names and not l.startswith('set\t'):
                keep.append(l)
    with open(fn, 'w') as out:
        out.write('\t'.join(['set', 'group', 'kind', 'path', 'bytes', 'corpus'] + COLS) + '\n')
        out.writelines(keep)
        for name in names:
            if name not in ('core', 'ext') and not os.path.exists(os.path.join(X.WORK, 'corpus-%s.pkl' % name)):
                print('no corpus %s here: left out' % name, flush=True)
                continue
            C, tier = load(name)
            for s, g, k, path, shown in rows:
                if path not in texts:
                    continue
                if name in ('old', 'deps') and s != 'new-tree':
                    continue  # the calibration sets are the earlier tree, and a control may be a dependency
                r = measure(texts[path], C, tier)
                out.write('\t'.join([s, g, k, shown, str(os.path.getsize(path)), name] + [str(r[c]) for c in COLS]) + '\n')
            print('scanned against %s, %.0fs' % (name, time.time() - t0), flush=True)
            if name not in ('core', 'ext'):
                del C


def show(path, name):
    """Print where a file of the new tree and a corpus share runs: line
    numbers on both sides, and the new tree's own lines. The corpus's lines
    are not printed; peek.py shows both sides on the terminal."""
    C, tier = load(name)
    lines = X.read_text(os.path.join(X.NEW_TREE, path))
    r, det = T.scan_lines(lines, C, detail=True)
    print(path, 'against', name)
    print({k: v for k, v in measure(lines, C, tier).items()})
    for a, b, n, w, src in det[tier]['sr']:
        print('--- counted run, lines %d-%d (%d content lines, %d significant) <- %s' % (a, b, n, w, src))
    _, runs = raw(lines, C, tier)
    for a, b, n, src in runs:
        print('--- raw run, lines %d-%d (%d content lines) <- %s' % (a, b, n, src))
    srcov = set()
    for a, b, n, w, src in det[tier]['sr']:
        srcov.update(range(a, b + 1))
    rest = [i for i in det[tier]['sigmatch'] if i not in srcov]
    print('--- significant lines that stand in the corpus, outside counted runs: %d' % len(rest))
    for i in rest[:80]:
        print('   %5d| %s' % (i, lines[i - 1][:150]))


ALL = ['core', 'ext', 'libs', 'old', 'deps']

if __name__ == '__main__':
    cmd = sys.argv[1] if len(sys.argv) > 1 else 'scan'
    if cmd == 'scan':
        scan(sys.argv[2:] or ALL)
    elif cmd == 'show':
        show(sys.argv[2], sys.argv[3])
