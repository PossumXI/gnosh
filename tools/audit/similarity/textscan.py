#!/usr/bin/env python3
"""textscan.py - words. Does any text of the new tree that a model or a person
reads (the prompt texts, the built-in agent definitions, every string literal
of the Go code, the help and golden files, the documents) share sentences or
long runs of words with Crush's texts?

What is compared: text is case-folded, the brand names (crush, crash, klash,
gnosh) become one token, and everything but letters, digits and underscores
separates words. A sentence ends at . ! or ? followed by a space or a line
end, at a blank line, at the start of a list item, heading or quote, and at a
table bar; sentences of fewer than 5 words are left out.

Units: a text file is one unit; a Go file's unit is its string literals of 4
words or more (each literal on its own: a run of words does not cross from one
literal to the next; literals joined with + are one).

Corpora: "core" and "ext" are Crush's (every text file that is not Go source,
and every string literal of its Go source), "libs" the same of Charm's
libraries, "since" the same of what Charm has published after the crush
corpus was fetched, "old" the same of the earlier gnosh's tree less the
carried files.

Per unit and corpus:

  words        words in the unit
  sentences    distinct sentences of 5 words or more
  sent_shared  how many of them also stand, word for word, in the corpus
  run          the longest run of consecutive words that also stands in one
               corpus text, in words (0 when under 5)
  w8           words that lie in shared runs of 8 words or more

text-matches.tsv (kept on the devbox) has the words of each unit's longest
runs and of its shared sentences, with where each stands in the corpus.

  textscan.py [corpus ...]      -> WORK/text.tsv, WORK/text-matches.tsv
"""
import ast
import collections
import os
import re
import sys
import time

import common as X
from common import T

WORD = re.compile(r'[a-z0-9@_]+')
SPLIT = re.compile(r'[.!?]+["\')\]*_`]*(?:\s+|$)|\n\s*\n|\n\s*(?:[-*+>]|\d+[.)]|#{1,6})\s+|\|')
SEED = 5
MIN_SENT = 5
MIN_STR_WORDS = 4
MAX_BYTES = 3 << 20


def fold(text):
    return T.BRAND.sub('@', text.casefold())


def words(text):
    return WORD.findall(fold(text))


def sentences(text):
    out = []
    for part in SPLIT.split(fold(text)):
        w = WORD.findall(part)
        if len(w) >= MIN_SENT:
            out.append(tuple(w))
    return out


def read_strings(fn, five):
    """tokscan -strings output: [(set, shown, line, string)] (set is '' for a
    corpus list)."""
    out = []
    if not os.path.exists(fn):
        return out
    for l in open(fn, encoding='utf-8', errors='replace'):
        p = l.rstrip('\n').split('\t')
        try:
            if five:
                out.append((p[0], p[1], int(p[2]), ast.literal_eval(p[3])))
            else:
                out.append(('', p[0], int(p[1]), ast.literal_eval(p[2])))
        except (ValueError, SyntaxError, IndexError):
            continue
    return out


class Unit:
    __slots__ = ('set', 'group', 'kind', 'path', 'what', 'segs', 'where', 'sents', 'nwords')

    def __init__(self, set_, group, kind, path, what):
        self.set, self.group, self.kind, self.path, self.what = set_, group, kind, path, what
        self.segs = []   # each a list of words
        self.where = []  # for each seg: the line it starts on (0 for a whole file)
        self.sents = set()
        self.nwords = 0

    def add(self, text, line):
        w = words(text)
        self.segs.append(w)
        self.where.append(line)
        self.nwords += len(w)
        self.sents.update(sentences(text))


def target_units():
    units = []
    meta = {}
    for l in open(os.path.join(X.WORK, 'lists', 'targets.tsv')):
        p = l.rstrip('\n').split('\t')
        if len(p) != 5:
            continue
        s, g, k, path, shown = p
        meta[(s, shown)] = (g, k)
        if s != 'new-tree' or k == 'binary' or shown.endswith(('.go', '.gno')):
            continue
        try:
            if os.path.getsize(path) > MAX_BYTES:
                continue
        except OSError:
            continue
        lines = X.read_text(path)
        if lines is None:
            continue
        u = Unit(s, g, k, shown, 'file')
        u.add('\n'.join(lines), 0)
        units.append(u)
    # the documents of the control programs, as controls for prose
    for name, d, lic, mod in X.control_dirs():
        for path in X.walk_files(d, exts=('.md', '.txt', '.tpl', '.tmpl')):
            if os.path.getsize(path) > MAX_BYTES or '/testdata/' in path:
                continue
            lines = X.read_text(path)
            if lines is None:
                continue
            u = Unit(name, 'control', 'doc', os.path.relpath(path, d), 'file')
            u.add('\n'.join(lines), 0)
            units.append(u)
    # calibration: the earlier gnosh's prompt templates and tool descriptions,
    # which are Crush's files as the earlier gnosh changed them
    old = os.path.join(X.WORK, 'old')
    for sub in ('internal/agent/templates', 'internal/agent/tools', 'internal/agent/prompt', 'internal/skills/builtin'):
        d = os.path.join(old, sub)
        if not os.path.isdir(d):
            continue
        for path in X.walk_files(d, exts=('.md', '.tpl')):
            lines = X.read_text(path)
            if lines is None:
                continue
            u = Unit('cal:old-prompts', 'calibration', 'prompt', os.path.relpath(path, old), 'file')
            u.add('\n'.join(lines), 0)
            units.append(u)
    # string literals of Go files
    by = collections.OrderedDict()
    for s, shown, line, text in read_strings(os.path.join(X.WORK, 'strings-targets.tsv'), True):
        if len(WORD.findall(text.casefold())) < MIN_STR_WORDS:
            continue
        key = (s, shown)
        if key not in by:
            g, k = meta.get(key, ('', 'code'))
            by[key] = Unit(s, g, k, shown, 'strings')
        by[key].add(text, line)
    units.extend(by.values())
    return units


def corpus_segments(name):
    """Yield (source name, text) for every text of a corpus."""
    lists = {'core': 'crush-core.tsv', 'ext': 'crush-ext.tsv', 'libs': 'libs.tsv', 'since': 'since.tsv', 'old': 'old.tsv'}
    for l in open(os.path.join(X.WORK, 'lists', lists[name])):
        path, shown = l.rstrip('\n').split('\t')
        if shown.rsplit('@', 1)[0].endswith(('.go', '.gno', '.sum', '.svg', '.lock')):
            continue
        try:
            if os.path.getsize(path) > MAX_BYTES:
                continue
        except OSError:
            continue
        lines = X.read_text(path)
        if lines is not None:
            yield shown, '\n'.join(lines)
    for _, shown, line, text in read_strings(os.path.join(X.WORK, 'strings-%s.tsv' % name), False):
        yield '%s:%d' % (shown, line), text


def scan(names):
    t0 = time.time()
    units = target_units()
    index = collections.defaultdict(list)
    for ui, u in enumerate(units):
        for si, w in enumerate(u.segs):
            for i in range(len(w) - SEED + 1):
                index[hash(tuple(w[i:i + SEED]))].append((ui, si, i))
    print('units: %d, seeds: %d, %.0fs' % (len(units), len(index), time.time() - t0), flush=True)
    out = open(os.path.join(X.WORK, 'text.tsv'), 'w')
    det = open(os.path.join(X.WORK, 'text-matches.tsv'), 'w')
    out.write('\t'.join(['set', 'group', 'kind', 'path', 'what', 'corpus', 'words', 'sentences', 'sent_shared', 'run', 'run_line', 'run_src', 'w8']) + '\n')
    det.write('\t'.join(['set', 'path', 'what', 'corpus', 'type', 'words', 'line', 'source', 'text']) + '\n')
    for name in names:
        sent_src = {}
        best = [[] for _ in units]         # per unit: [(len, seg, start, src)]
        cov = [None] * len(units)          # per unit: set of (seg, pos) in runs of 8+
        nseg = nw = 0
        for src, text in corpus_segments(name):
            for s in sentences(text):
                sent_src.setdefault(hash(s), src)
            cw = words(text)
            nseg += 1
            nw += len(cw)
            done = {}
            for ci in range(len(cw) - SEED + 1):
                ps = index.get(hash(tuple(cw[ci:ci + SEED])))
                if not ps:
                    continue
                for ui, si, ti in ps:
                    key = (ui, si, ti - ci)
                    if done.get(key, -1) > ti:
                        continue
                    tw = units[ui].segs[si]
                    if tw[ti:ti + SEED] != cw[ci:ci + SEED]:
                        continue
                    a, c = ti, ci
                    while a > 0 and c > 0 and tw[a - 1] == cw[c - 1]:
                        a -= 1
                        c -= 1
                    b, c = ti + SEED, ci + SEED
                    while b < len(tw) and c < len(cw) and tw[b] == cw[c]:
                        b += 1
                        c += 1
                    done[key] = b
                    n = b - a
                    lst = best[ui]
                    if not any(x[1] == si and x[2] == a and x[0] == n for x in lst):
                        lst.append((n, si, a, src))
                        if len(lst) > 12:
                            lst.sort(reverse=True)
                            del lst[8:]
                    if n >= 8:
                        if cov[ui] is None:
                            cov[ui] = set()
                        cov[ui].update((si, t) for t in range(a, b))
        print('corpus %s: %d texts, %d words, %d distinct sentences, %.0fs' % (name, nseg, nw, len(sent_src), time.time() - t0), flush=True)
        for ui, u in enumerate(units):
            shared = [s for s in u.sents if hash(s) in sent_src]
            lst = sorted(best[ui], reverse=True)
            run, line, src = 0, '', ''
            if lst:
                run, si, a, src = lst[0]
                line = str(u.where[si])
            out.write('\t'.join([u.set, u.group, u.kind, u.path, u.what, name, str(u.nwords), str(len(u.sents)), str(len(shared)),
                                 str(run), line, src, str(len(cov[ui]) if cov[ui] else 0)]) + '\n')
            for n, si, a, s in lst[:6]:
                if n >= 6:
                    det.write('\t'.join([u.set, u.path, u.what, name, 'run', str(n), str(u.where[si]), s, ' '.join(u.segs[si][a:a + n])]) + '\n')
            for s in shared:
                det.write('\t'.join([u.set, u.path, u.what, name, 'sentence', str(len(s)), '', sent_src[hash(s)], ' '.join(s)]) + '\n')
    out.close()
    det.close()


if __name__ == '__main__':
    scan(sys.argv[1:] or ['core', 'ext', 'libs', 'old'])
