#!/usr/bin/env python3
"""carried_check.py - the carried files, checked again from the outside.

For every row of CARRIED.md's tables:

  bytes     the file in the new tree against the earlier gnosh's file at the
            row's old path (git blob at OLD_MAIN): identical, or how many
            lines differ; and whether that blob's SHA-256 is the one the row
            records for the staged original.
  numbers   the row's "line share" and "longest run" against the same two
            measures taken again (the inventory's own script, its "ext"
            tier): (a) of the earlier gnosh's blob against the inventory's
            own corpus, which must give the row's numbers exactly; (b) of the
            file as it stands in the new tree against that same corpus; and
            (c) of the file as it stands against this audit's corpus, which
            was built against a later commit of upstream.

It also runs the tree's own check, tools/audit/carried.py check.

Output: WORK/carried.tsv, and a summary on standard output and in
WORK/carried.txt.
"""
import difflib
import hashlib
import os
import pickle
import subprocess
import sys

import common as X
from common import T


def load(fn):
    C = T.Corpus()
    with open(fn, 'rb') as f:
        C.__dict__.update(pickle.load(f))
    return C


def main():
    out = []

    def say(s):
        out.append(s)
        print(s, flush=True)

    table = X.carried_table()
    inv_fn = os.path.join(os.path.dirname(X.INVENTORY.rstrip('/')), 'corpus.pkl') if X.INVENTORY else ''
    inv = load(inv_fn) if inv_fn and os.path.exists(inv_fn) else None
    now = load(os.path.join(X.WORK, 'corpus.pkl'))
    if inv is None:
        say('the inventory\'s corpus is not at hand: the numbers are taken against this audit\'s corpus only')
    cat = T.Cat(X.CRUSH_REPO)
    rows = []
    for path in sorted(table):
        r = table[path]
        p = os.path.join(X.NEW_TREE, path)
        mine = open(p, 'rb').read() if os.path.exists(p) else None
        old = cat.get(X.OLD_MAIN + ':' + r['old']) if X.OLD_MAIN else None
        if mine is None:
            state = 'MISSING from the new tree'
        elif old is None:
            state = 'no such file in the earlier gnosh'
        elif mine == old:
            state = 'identical'
        else:
            a, b = old.split(b'\n'), mine.split(b'\n')
            if len(a) == len(b):
                state = '%d of %d lines differ' % (sum(1 for x, y in zip(a, b) if x != y), len(a))
            else:
                sm = difflib.SequenceMatcher(None, a, b, autojunk=False)
                kept = sum(m.size for m in sm.get_matching_blocks())
                state = '%d of %d lines kept in order' % (kept, len(a))
        sha_ok = old is not None and hashlib.sha256(old).hexdigest() == r['sha']
        m = dict(inv_old=None, inv_new=None, now_new=None)
        lines_old = T.text_lines(old) if old is not None else None
        lines_new = T.text_lines(mine) if mine is not None else None
        for key, C, lines in (('inv_old', inv, lines_old), ('inv_new', inv, lines_new), ('now_new', now, lines_new)):
            if C is not None and lines is not None:
                s = T.scan_lines(lines, C)
                m[key] = (s['ext_sigd_pct'], s['ext_srun_longest'])
        rows.append((path, r['kind'], state, sha_ok, r['share'], r['run'], m['inv_old'], m['inv_new'], m['now_new']))
    cat.close()

    def cell(v, i):
        return '' if v is None else str(v[i])
    with open(os.path.join(X.WORK, 'carried.tsv'), 'w') as f:
        f.write('path\tkind\tagainst_earlier_gnosh\tsha256_as_recorded\ttable_share\tshare_original_inventory_corpus\tshare_now_inventory_corpus\tshare_now_this_corpus\ttable_run\trun_original_inventory_corpus\trun_now_inventory_corpus\trun_now_this_corpus\n')
        for path, kind, state, sha_ok, share, run, a, b, c in rows:
            f.write('\t'.join([path, kind, state, 'yes' if sha_ok else 'NO', '' if share is None else str(share), cell(a, 0), cell(b, 0), cell(c, 0),
                               '' if run is None else str(run), cell(a, 1), cell(b, 1), cell(c, 1)]) + '\n')
    import collections
    say('rows in CARRIED.md\'s tables: %d' % len(rows))
    by = collections.Counter((k, s if s == 'identical' else ('one line differs' if s.startswith('1 of') else s.split(' of ')[0] + ' ...' if 'kept' in s else s)) for _, k, s, *_ in rows)
    for (k, s), n in sorted(by.items()):
        say('  %-28s %-34s %d' % (k, s, n))
    say('SHA-256 of the earlier gnosh\'s file equals the one recorded: %d of %d' % (sum(1 for r in rows if r[3]), len(rows)))
    text = [r for r in rows if r[4] is not None]
    for label, idx in (('the earlier gnosh\'s file against the inventory\'s corpus', 6), ('the file as it stands against the inventory\'s corpus', 7), ('the file as it stands against this audit\'s corpus', 8)):
        have = [r for r in text if r[idx] is not None]
        if not have:
            continue
        same_share = sum(1 for r in have if abs(r[idx][0] - r[4]) < 0.05)
        same_run = sum(1 for r in have if r[idx][1] == r[5])
        say('%s: line share equals the table\'s in %d of %d rows, longest run in %d of %d' % (label, same_share, len(have), same_run, len(have)))
        for r in have:
            if abs(r[idx][0] - r[4]) >= 0.05 or r[idx][1] != r[5]:
                say('    differs: %s [%s]: table %.1f%% / %d, measured %.1f%% / %d' % (r[0], r[1], r[4], r[5], r[idx][0], r[idx][1]))
    say('rows with no numbers in the table (binary test data): %d' % (len(rows) - len(text)))
    chk = os.path.join(X.NEW_TREE, 'tools/audit/carried.py')
    if os.path.exists(chk):
        r = subprocess.run([sys.executable, chk, 'check'], cwd=X.NEW_TREE, capture_output=True, text=True)
        tail = (r.stdout + r.stderr).strip().splitlines()[-3:]
        say('tools/audit/carried.py check: exit %d: %s' % (r.returncode, ' | '.join(tail)))
    with open(os.path.join(X.WORK, 'carried.txt'), 'w') as f:
        f.write('\n'.join(out) + '\n')


if __name__ == '__main__':
    main()
