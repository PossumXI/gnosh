#!/usr/bin/env python3
"""report.py - put the scans' outputs together.

Reads WORK/lines.tsv (linescan.py), WORK/tok.tsv (tokscan.go) and
WORK/text.tsv (textscan.py) and writes

  WORK/results.tsv   one row per file of the new tree, with every measure
                     against every corpus (the column names are explained in
                     README.md)
  WORK/summary.md    the tables README.md shows: how each measure is spread
                     in each set of files, the control range, the files of
                     new code above it, and the twenty highest of each
                     measure

The corpora, by the prefix of their columns:

  crush  Crush at the fork point, upstream's tree, and every file version an
         upstream commit since the fork point brought in: (a) to (c)
  hist   crush plus the rest of Crush's history, branches and tags
  libs   Charm's libraries at the pinned versions: (d)
  since  what Charm has published after the crush corpus was fetched (a later
         fetch of upstream's branches and tags), when that corpus was built
  old    the earlier gnosh's tree, less the carried files
  deps   the earlier gnosh's other dependencies that the new go.mod dropped
"""
import collections
import csv
import os
import sys

import common as X

CORP = [('core', 'crush'), ('ext', 'hist'), ('libs', 'libs'), ('since', 'since'), ('old', 'old'), ('deps', 'deps')]
LINE = ['share', 'run', 'run_lines', 'lrun', 'rawshare', 'rawrun', 'rawrun_lines']
TOKX = [('single', 'tok_run'), ('cov', 'tok_cov'), ('vrun', 'tok_vrun'), ('vcov', 'tok_vcov')]
TOKA = [('single', 'abs_run'), ('cov', 'abs_cov'), ('vrun', 'abs_vrun'), ('vcov', 'abs_vcov')]


def load(name):
    fn = os.path.join(X.WORK, name)
    with open(fn, encoding='utf-8', errors='replace') as f:
        return list(csv.DictReader(f, delimiter='\t', quoting=csv.QUOTE_NONE))


def q(v, p):
    v = sorted(v)
    return v[min(len(v) - 1, int(round(p / 100 * (len(v) - 1))))] if v else 0


def fmt(x):
    if isinstance(x, float):
        return ('%.1f' % x)
    return str(x)


def main():
    lines = load('lines.tsv')
    tok = load('tok.tsv')
    text = load('text.tsv')
    L = {(r['set'], r['path'], r['corpus']): r for r in lines}
    Tk = {(r['set'], r['path'], r['corpus'], r['form']): r for r in tok}
    Tx = {(r['set'], r['path'], r['corpus']): r for r in text}
    # the corpora that are Charm's: crush, hist, libs, and since when it was built
    have = {r['corpus'] for r in lines}
    CHARM = [(c, p) for c, p in CORP[:4] if c in have]

    # ---- results.tsv: the new tree --------------------------------------
    files = []
    for l in open(os.path.join(X.WORK, 'lists', 'targets.tsv')):
        p = l.rstrip('\n').split('\t')
        if p[0] == 'new-tree':
            files.append((p[4], p[1], p[2], p[3]))
    cols = ['path', 'group', 'kind', 'bytes', 'sig_lines', 'long_lines', 'tokens', 'words', 'sentences']
    for corp, pre in CORP:
        if corp == 'since' and corp not in have:
            continue  # no later fetch was compared: the columns are the first audit's
        cols += ['%s_%s' % (pre, c) for c in LINE]
        cols += ['%s_%s' % (pre, c) for _, c in TOKX]
        cols += ['%s_%s' % (pre, c) for _, c in TOKA]
        if pre != 'deps':
            cols += ['%s_word_run' % pre, '%s_sent_shared' % pre]
    cols += ['crush_line_source', 'crush_tok_vrun_source', 'crush_abs_vrun_source', 'libs_abs_vrun_source']
    rows = []
    for path, group, kind, disk in files:
        row = collections.OrderedDict((c, '') for c in cols)
        row.update(path=path, group=group, kind=kind, bytes=os.path.getsize(disk))
        base = L.get(('new-tree', path, 'core'))
        if base:
            row['sig_lines'] = base['sigd']
            row['long_lines'] = base['rawd']
            row['crush_line_source'] = base['best_src'] if int(base['best_src_n'] or 0) else ''
        tb = Tk.get(('new-tree', path, 'core', 'exact'))
        if tb:
            row['tokens'] = tb['ntok']
            row['crush_tok_vrun_source'] = ('%s L%s' % (tb['vrun_source'], tb['vrun_source_lines'])) if int(tb['vrun']) else ''
            ta = Tk[('new-tree', path, 'core', 'abstract')]
            row['crush_abs_vrun_source'] = ('%s L%s' % (ta['vrun_source'], ta['vrun_source_lines'])) if int(ta['vrun']) else ''
            tl = Tk.get(('new-tree', path, 'libs', 'abstract'))
            if tl and int(tl['vrun']):
                row['libs_abs_vrun_source'] = '%s L%s' % (tl['vrun_source'], tl['vrun_source_lines'])
        xb = Tx.get(('new-tree', path, 'core'))
        if xb:
            row['words'] = xb['words']
            row['sentences'] = xb['sentences']
        for corp, pre in CORP:
            r = L.get(('new-tree', path, corp))
            if r:
                for c in LINE:
                    row['%s_%s' % (pre, c)] = r[c]
            for form, names in (('exact', TOKX), ('abstract', TOKA)):
                t = Tk.get(('new-tree', path, corp, form))
                if t:
                    for src, c in names:
                        row['%s_%s' % (pre, c)] = t[src]
            x = Tx.get(('new-tree', path, corp))
            if x and pre != 'deps':
                row['%s_word_run' % pre] = x['run']
                row['%s_sent_shared' % pre] = x['sent_shared']
        rows.append(row)
    with open(os.path.join(X.WORK, 'results.tsv'), 'w') as f:
        f.write('\t'.join(cols) + '\n')
        for row in rows:
            f.write('\t'.join(str(row[c]) for c in cols) + '\n')

    # ---- summary.md ------------------------------------------------------
    out = []
    w = out.append

    def setname(r):
        if r['set'] != 'new-tree':
            return r['set']
        k = r['kind']
        if r['group'] == 'new':
            if k == 'code':
                return 'new code'
            return 'new: ' + {'prompt': 'prompt texts', 'testdata': 'test data', 'doc': 'documents', 'other': 'go.mod, go.sum and the like'}.get(k, k)
        return {'carried': 'carried', 'carried-tools': 'carried with larger changes', 'carried-bench': 'carried with larger changes',
                'third-party': 'third-party'}[r['group']] + ('' if k == 'code' else ': ' + k)

    ORDER = ['new code', 'new: prompt texts', 'new: test data', 'new: documents', 'new: go.mod, go.sum and the like',
             'carried', 'carried with larger changes', 'third-party']

    def order(names):
        names = set(names)
        head = [n for n in ORDER if n in names]
        rest = sorted(n for n in names if n not in head and not n.startswith(('ctl:', 'cal:')))
        return head + rest + sorted(n for n in names if n.startswith('ctl:')) + sorted(n for n in names if n.startswith('cal:'))

    for corp, pre in CHARM:
        w('\n### Lines, against %s\n' % pre)
        w('| set | files | with 10+ significant lines | share p50 | p95 | max | files with a counted run | longest | files with any run of 4+ | longest | raw share p95 | max |')
        w('|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|')
        G = collections.defaultdict(list)
        for r in lines:
            if r['corpus'] == corp:
                G[setname(r)].append(r)
        for name in order(G):
            ds = G[name]
            big = [d for d in ds if int(d['sigd']) >= 10]
            rb = [d for d in ds if int(d['rawd']) >= 10]
            sh = [float(d['share']) for d in big]
            rs = [float(d['rawshare']) for d in rb]
            w('| %s | %d | %d | %s%% | %s%% | %s%% | %d | %d | %d | %d | %s%% | %s%% |' % (
                name, len(ds), len(big), fmt(q(sh, 50)), fmt(q(sh, 95)), fmt(q(sh, 100)),
                sum(1 for d in ds if int(d['run']) > 0), max(int(d['run']) for d in ds),
                sum(1 for d in ds if int(d['rawrun']) > 0), max(int(d['rawrun']) for d in ds),
                fmt(q(rs, 95)), fmt(q(rs, 100))))
    for form, label, k in (('exact', 'Exact tokens (windows of 40)', 40), ('abstract', 'Identifier-insensitive tokens (windows of 60)', 60)):
        for corp, pre in CHARM:
            w('\n### %s, against %s\n' % (label, pre))
            w('| set | files of 100+ tokens | with a varied shared window | longest varied run p95 | p99 | max | varied cover p99 | max | with any shared window | longest run p99 | max | cover p99 | max |')
            w('|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|')
            G = collections.defaultdict(list)
            for r in tok:
                if r['corpus'] == corp and r['form'] == form and int(r['ntok']) >= 100:
                    G[setname(r)].append(r)
            for name in order(G):
                ds = G[name]
                vr = [int(d['vrun']) for d in ds]
                vc = [100.0 * int(d['vcov']) / int(d['ntok']) for d in ds]
                sr = [int(d['single']) for d in ds]
                cv = [100.0 * int(d['cov']) / int(d['ntok']) for d in ds]
                w('| %s | %d | %d | %d | %d | %d | %s%% | %s%% | %d | %d | %d | %s%% | %s%% |' % (
                    name, len(ds), sum(1 for d in ds if int(d['vwin']) > 0), q(vr, 95), q(vr, 99), q(vr, 100), fmt(q(vc, 99)), fmt(q(vc, 100)),
                    sum(1 for d in ds if int(d['nwin']) > 0), q(sr, 99), q(sr, 100), fmt(q(cv, 99)), fmt(q(cv, 100))))
    for corp, pre in CHARM:
        w('\n### Words, against %s\n' % pre)
        w('| set | units of 20+ words | longest shared word run p50 | p95 | p99 | max | units with a run of 12+ words | sentences | of them also in the corpus |')
        w('|---|---:|---:|---:|---:|---:|---:|---:|---:|')
        G = collections.defaultdict(list)
        for r in text:
            if r['corpus'] == corp and int(r['words']) >= 20:
                n = setname(r)
                if r['what'] == 'strings':
                    n += ' (string literals)'
                G[n].append(r)
        names = sorted(G, key=lambda n: (n.startswith('cal:'), n.startswith('ctl:'), n))
        for name in names:
            ds = G[name]
            rn = [int(d['run']) for d in ds]
            w('| %s | %d | %d | %d | %d | %d | %d | %d | %d |' % (name, len(ds), q(rn, 50), q(rn, 95), q(rn, 99), q(rn, 100),
                                                                 sum(1 for x in rn if x >= 12), sum(int(d['sentences']) for d in ds), sum(int(d['sent_shared']) for d in ds)))

    # the control range and what lies above it
    w('\n### The control range, and new code above it\n')
    w('| measure | corpus | highest in the control set | new code: highest | files of new code above the control set\'s highest |')
    w('|---|---|---:|---:|---|')
    above = collections.OrderedDict()

    def rng(label, corp, pre, ctl, new, key, show, thr_corp=None):
        cmax = max((key(r) for r in ctl), default=0)
        hi = sorted([r for r in new if key(r) > cmax], key=lambda r: -key(r))
        nmax = max((key(r) for r in new), default=0)
        w('| %s | %s | %s | %s | %s |' % (label, pre, show(cmax), show(nmax), '; '.join('`%s` (%s)' % (r['path'], show(key(r))) for r in hi[:12]) + (' and %d more' % (len(hi) - 12) if len(hi) > 12 else '') if hi else 'none'))
        for r in hi:
            above.setdefault(r['path'], []).append('%s vs %s: %s (control %s)' % (label, pre, show(key(r)), show(cmax)))

    pct = lambda v: '%.1f%%' % v  # noqa: E731
    num = lambda v: '%d' % v  # noqa: E731
    for corp, pre in CHARM:
        # hist's controls are in part inside Crush's history (vendored code), so the yardstick for hist is crush's;
        # and since is a few dozen files of Crush's, too few for the controls to have a range against: crush's again
        cc = 'core' if corp in ('ext', 'since') else corp
        lc = [r for r in lines if r['set'].startswith('ctl:') and r['corpus'] == cc]
        ln = [r for r in lines if r['set'] == 'new-tree' and r['group'] == 'new' and r['kind'] == 'code' and r['corpus'] == corp]
        rng('line share (10+ significant lines)', corp, pre, [r for r in lc if int(r['sigd']) >= 10], [r for r in ln if int(r['sigd']) >= 10], lambda r: float(r['share']), pct)
        rng('line share, no generic-Go filter (10+ long lines)', corp, pre, [r for r in lc if int(r['rawd']) >= 10], [r for r in ln if int(r['rawd']) >= 10], lambda r: float(r['rawshare']), pct)
        rng('longest counted line run', corp, pre, lc, ln, lambda r: int(r['run']), num)
        rng('longest line run, counted or not', corp, pre, lc, ln, lambda r: int(r['rawrun']), num)
        for form, lab in (('exact', 'exact tokens'), ('abstract', 'abstract tokens')):
            tc = [r for r in tok if r['set'].startswith('ctl:') and r['corpus'] == cc and r['form'] == form and int(r['ntok']) >= 100]
            tn = [r for r in tok if r['set'] == 'new-tree' and r['group'] == 'new' and r['corpus'] == corp and r['form'] == form and int(r['ntok']) >= 100]
            rng('%s: longest varied run' % lab, corp, pre, tc, tn, lambda r: int(r['vrun']), num)
            rng('%s: varied cover' % lab, corp, pre, tc, tn, lambda r: 100.0 * int(r['vcov']) / int(r['ntok']), pct)
            rng('%s: longest run, lists and tables included' % lab, corp, pre, tc, tn, lambda r: int(r['single']), num)
            rng('%s: cover, lists and tables included' % lab, corp, pre, tc, tn, lambda r: 100.0 * int(r['cov']) / int(r['ntok']), pct)
    w('\nFiles of new code above the control set on any measure: %d\n' % len(above))
    for p, why in above.items():
        w('- `%s`: %s' % (p, '; '.join(why)))

    # the twenty highest of each measure, new code against crush, hist, libs
    w('\n### The twenty highest of each measure (new code)\n')
    tops = collections.OrderedDict()
    for corp, pre in CHARM:
        ln = [r for r in lines if r['set'] == 'new-tree' and r['group'] == 'new' and r['kind'] in ('code', 'prompt') and r['corpus'] == corp]
        for label, sel, key, show, src in (
                ('line share', [r for r in ln if int(r['sigd']) >= 10], lambda r: float(r['share']), pct, lambda r: r['best_src']),
                ('longest line run of 4+ (counted or not)', ln, lambda r: int(r['rawrun']), num, lambda r: r['top']),
                ('longest run of lines each somewhere in the corpus', ln, lambda r: int(r['lrun']), num, lambda r: '')):
            top = [r for r in sorted(sel, key=lambda r: -key(r))[:20] if key(r) > 0]
            tops[(label, pre)] = [(r['path'], show(key(r)), src(r)) for r in top]
        for form, lab in (('exact', 'exact tokens'), ('abstract', 'abstract tokens')):
            tn = [r for r in tok if r['set'] == 'new-tree' and r['group'] == 'new' and r['corpus'] == corp and r['form'] == form]
            top = [r for r in sorted(tn, key=lambda r: (-int(r['vrun']), -int(r['vcov'])))[:20] if int(r['vrun']) > 0]
            tops[('%s: longest varied run' % lab, pre)] = [(r['path'], '%s tokens, lines %s' % (r['vrun'], r['vrun_target_lines']), '%s lines %s' % (r['vrun_source'], r['vrun_source_lines'])) for r in top]
            top = [r for r in sorted(tn, key=lambda r: -int(r['single']))[:20] if int(r['single']) > 0]
            tops[('%s: longest run, lists and tables included' % lab, pre)] = [(r['path'], '%s tokens (%s distinct windows), lines %s' % (r['single'], r['single_dw'], r['target_lines']), '%s lines %s' % (r['source'], r['source_lines'])) for r in top]
    for (label, pre), items in tops.items():
        w('\n**%s, against %s** (%d files above zero shown)\n' % (label, pre, len(items)))
        for path, val, src in items:
            w('- `%s`: %s%s' % (path, val, (' <- ' + src) if src else ''))
    with open(os.path.join(X.WORK, 'summary.md'), 'w') as f:
        f.write('\n'.join(out) + '\n')
    print('results.tsv: %d rows, %d columns; summary.md: %d lines; new-code files above the control range: %d' % (len(rows), len(cols), len(out), len(above)))


def calibrate():
    """The table of README.md that says how the setting for a "varied" window
    was chosen: for each setting (the one run.sh uses, in tok.tsv, and the
    others, in tok-div-E-A.tsv, which run.sh writes when CALIBRATE is set),
    how many files of each set share a varied window with crush, and the
    longest varied run. Files of 100 tokens or more, against crush only."""
    import glob
    import re
    runs = [((12, 9), os.path.join(X.WORK, 'tok.tsv'))]
    for fn in glob.glob(os.path.join(X.WORK, 'tok-div-*.tsv')):
        m = re.search(r'tok-div-(\d+)-(\d+)\.tsv$', fn)
        if m:
            runs.append(((int(m.group(1)), int(m.group(2))), fn))
    sets = (('Crush files the earlier gnosh modified', lambda r: r['set'] == 'cal:crush-modified'),
            ('short-block files', lambda r: r['set'] == 'cal:old-shortblock'),
            ('control files', lambda r: r['set'].startswith('ctl:')),
            ('new code', lambda r: r['set'] == 'new-tree' and r['group'] == 'new'))
    out = ['| a window is varied at (exact / abstract) | ' + ' | '.join('%s: with a varied window, exact / abstract; longest run' % n for n, _ in sets) + ' |',
           '|---|' + '---|' * len(sets)]
    for (e, a), fn in sorted(runs):
        with open(fn, encoding='utf-8', errors='replace') as f:
            rows = [r for r in csv.DictReader(f, delimiter='\t', quoting=csv.QUOTE_NONE) if r['corpus'] == 'core' and int(r['ntok']) >= 100]
        cells = []
        for name, sel in sets:
            ex = [r for r in rows if sel(r) and r['form'] == 'exact']
            ab = [r for r in rows if sel(r) and r['form'] == 'abstract']
            cells.append('%d / %d of %d; %d / %d' % (sum(1 for r in ex if int(r['vwin']) > 0), sum(1 for r in ab if int(r['vwin']) > 0), len(ex),
                                                     max((int(r['vrun']) for r in ex), default=0), max((int(r['vrun']) for r in ab), default=0)))
        out.append('| %s%d / %d%s | %s |' % ('**' if (e, a) == (12, 9) else '', e, a, '**' if (e, a) == (12, 9) else '', ' | '.join(cells)))
    with open(os.path.join(X.WORK, 'calibration.md'), 'w') as f:
        f.write('\n'.join(out) + '\n')
    print('\n'.join(out))


if __name__ == '__main__':
    if sys.argv[1:] == ['calibrate']:
        calibrate()
    else:
        main()
