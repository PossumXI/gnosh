#!/usr/bin/env python3
"""build_corpus.py - gather what the new tree is compared against, and the
lists of files to compare.

  build_corpus.py crush    Charm's code, the way the earlier inventory built it
                           (taintscan.py, unchanged): "core" = the fork-point
                           tree plus every file version an upstream commit
                           since then brought in (so upstream's tree today and
                           every line it has added); "ext" = core plus every
                           file version reachable from any upstream branch or
                           tag. Writes corpus.pkl, and every text blob to
                           blobs/ with lists/crush-{core,ext}.tsv.
  build_corpus.py libs     corpus (d): Charm's libraries at the pinned versions
  build_corpus.py old      corpus (e): the earlier gnosh's tree, less the files
                           CARRIED.md lists (those are in the new tree)
  build_corpus.py deps     corpus (f): the earlier gnosh's other dependencies
                           that the new go.mod no longer requires
  build_corpus.py targets  lists/targets.tsv: the new tree's files, the control
                           set and the calibration sets
  build_corpus.py ref      lists/ref.tsv: unrelated Go code (the generic-Go
                           reference and the control programs), for the check
                           of names
  build_corpus.py all      all of the above
  build_corpus.py need     print the modules missing from the module caches

Lists are tab-separated: path on disk, name to show. A Crush file is shown as
tier:path@blob, where tier is base (the fork-point tree), up (brought in by an
upstream commit since) or hist (the rest of Crush's history) and blob is the
start of its git blob id, since one path has many versions. targets.tsv has
the set, the group and the kind in front.
"""
import json
import os
import pickle
import re
import sys
import time

import common as X
from common import T


def out_path(*parts):
    p = os.path.join(X.WORK, *parts)
    os.makedirs(os.path.dirname(p), exist_ok=True)
    return p


def crush():
    t0 = time.time()
    C = T.load_corpus()  # builds WORK/corpus.pkl if it is not there
    print('line corpus:', C.stats.get('sizes'), flush=True)
    # The same walk again, writing each text blob to disk for the token and
    # text scans. Same order as taintscan.build_corpus, so a blob keeps the
    # tier and the path it was first seen with.
    cat = T.Cat(X.CRUSH_REPO)
    seen = set()
    rows = []

    def take(sha, path, tier):
        if sha in seen:
            return
        seen.add(sha)
        data = cat.get(sha)
        if data is None or T.text_lines(data) is None:
            return
        ext = os.path.splitext(path)[1]
        if len(ext) > 12 or '/' in ext:
            ext = ''
        fn = out_path('blobs', sha[:2], sha + ext)
        if not os.path.exists(fn):
            with open(fn, 'wb') as f:
                f.write(data)
        rows.append((tier, fn, '%s:%s@%s' % (tier, path, sha[:8])))

    for l in T.git('ls-tree', '-r', X.FORK_POINT).splitlines():
        meta, path = l.split('\t', 1)
        _, typ, sha = meta.split()
        if typ == 'blob':
            take(sha, path, 'base')
    for l in T.git('rev-list', '--objects', X.FORK_POINT + '..' + X.UPSTREAM).splitlines():
        if ' ' in l:
            sha, path = l.split(' ', 1)
            take(sha, path, 'up')
    for l in T.git('rev-list', '--objects', '--remotes=upstream', '--tags').splitlines():
        if ' ' in l:
            sha, path = l.split(' ', 1)
            take(sha, path, 'hist')
    cat.close()
    with open(out_path('lists', 'crush-core.tsv'), 'w') as f:
        for tier, fn, disp in rows:
            if tier != 'hist':
                f.write('%s\t%s\n' % (fn, disp))
    with open(out_path('lists', 'crush-ext.tsv'), 'w') as f:
        for tier, fn, disp in rows:
            f.write('%s\t%s\n' % (fn, disp))
    n = {t: sum(1 for r in rows if r[0] == t) for t in ('base', 'up', 'hist')}
    g = {t: sum(1 for r in rows if r[0] == t and r[2].rsplit('@', 1)[0].endswith('.go')) for t in ('base', 'up', 'hist')}
    print('text blobs written:', n, 'of which .go:', g, '%.0fs' % (time.time() - t0), flush=True)
    return dict(text_blobs=n, go_blobs=g, sizes=C.stats.get('sizes'))


def dir_corpus(name, roots, max_bytes=3 << 20):
    """A line corpus (taintscan's own structure, everything in the "core"
    tier) from folders on disk. roots: [(shown prefix, directory)]."""
    C = T.Corpus()
    rows = []
    nfiles = nlines = 0
    for prefix, d in roots:
        for p in X.walk_files(d):
            if p.lower().endswith(X.TEXT_EXTS_SKIP):
                continue
            try:
                if os.path.getsize(p) > max_bytes:
                    continue
            except OSError:
                continue
            lines = X.read_text(p)
            if lines is None:
                continue
            disp = name + ':' + prefix + '/' + os.path.relpath(p, d)
            pid = len(C.paths)
            C.paths.append(disp)
            nlines += C.add(lines, pid, True)
            nfiles += 1
            rows.append((p, disp))
    d = dict(C.__dict__)
    d['generic'] = set()  # the one generic-Go reference is corpus.pkl's; it is attached when loading
    d['stats'] = dict(files=nfiles, lines=nlines, distinct_lines=len(C.core_lines), shingles=len(C.core_sh))
    with open(out_path('corpus-%s.pkl' % name), 'wb') as f:
        pickle.dump(d, f, protocol=pickle.HIGHEST_PROTOCOL)
    with open(out_path('lists', '%s.tsv' % name), 'w') as f:
        for p, disp in rows:
            f.write('%s\t%s\n' % (p, disp))
    print('corpus %s: %s' % (name, d['stats']), flush=True)
    return d['stats']


def libs():
    roots = []
    missing = []
    for m in X.CHARM_LIBS:
        d = X.moddir(m)
        if d:
            roots.append((m, d))
        else:
            missing.append(m)
    if missing:
        print('MISSING Charm libraries (go mod download them into MODCACHE):', missing, flush=True)
    st = dir_corpus('libs', roots)
    st['modules'] = [m for m, _ in roots]
    st['missing'] = missing
    return st


REQ = re.compile(r'^\s*([\w./~-]+\.[\w./~-]+)\s+(v[\w.+-]+)')


def requires(text):
    out = []
    for line in text.splitlines():
        line = line.split('//')[0]
        if line.strip().startswith(('module', 'go ', 'toolchain', 'replace', 'exclude')):
            continue
        m = REQ.match(line.replace('require ', ''))
        if m:
            out.append((m.group(1), m.group(2)))
    return out


def deps():
    """The earlier gnosh's dependencies that are not Charm's, that the new
    go.mod does not require, and that are not in the generic-Go reference."""
    if not X.OLD_MAIN:
        return {}
    old = requires(T.git('show', X.OLD_MAIN + ':go.mod'))
    new = {m for m, _ in requires(open(os.path.join(X.NEW_TREE, 'go.mod')).read())}
    roots = []
    missing = []
    for m, v in old:
        if m.startswith(('charm.land/', 'github.com/charmbracelet/', 'golang.org/x/')) or m in new:
            continue
        d = X.moddir(m + '@' + v)
        if d:
            roots.append((m + '@' + v, d))
        else:
            missing.append(m + '@' + v)
    st = dir_corpus('deps', roots)
    st['modules'] = [m for m, _ in roots]
    st['missing'] = missing
    print('deps: %d modules, %d not in the module cache' % (len(roots), len(missing)), flush=True)
    return st


def old():
    """The earlier gnosh at OLD_MAIN, less the files the new tree carries."""
    if not X.OLD_MAIN:
        return {}
    carried = {r['old'] for r in X.carried_table().values()}
    cat = T.Cat(X.CRUSH_REPO)
    root = out_path('old', '.keep')
    root = os.path.dirname(root)
    n = 0
    for l in T.git('ls-tree', '-r', X.OLD_MAIN).splitlines():
        meta, path = l.split('\t', 1)
        _, typ, sha = meta.split()
        if typ != 'blob' or path in carried:
            continue
        data = cat.get(sha)
        if data is None or T.text_lines(data) is None:
            continue
        fn = os.path.join(root, path)
        os.makedirs(os.path.dirname(fn), exist_ok=True)
        with open(fn, 'wb') as f:
            f.write(data)
        n += 1
    cat.close()
    st = dir_corpus('old', [('gnosh-old', root)])
    st['left_out_as_carried'] = len(carried)
    return st


def targets():
    rows = []
    # the new tree
    for rel, group, kind in X.new_tree_files():
        p = os.path.join(X.NEW_TREE, rel)
        if X.read_text(p) is None:
            kind = 'binary'
        rows.append(('new-tree', group, kind, p, rel))
    # the control set
    for name, d, lic, mod in X.control_dirs():
        for p in X.walk_files(d, exts=('.go',)):
            rows.append((name, 'control', 'code', p, os.path.relpath(p, d)))
    # calibration: files known to be Crush's, from the earlier gnosh's tree
    if X.OLD_MAIN:
        st = {}
        for l in T.git('diff', '--no-renames', '--name-status', X.FORK_POINT, X.OLD_MAIN).splitlines():
            s, p = l.split('\t', 1)
            st[p] = s
        verdicts = {}
        fn = os.path.join(X.INVENTORY, 'classified.json') if X.INVENTORY else ''
        if fn and os.path.exists(fn):
            for p, r in json.load(open(fn)).items():
                verdicts[p] = r['verdict'] + ('-short' if r.get('sub') else '')
        cat = T.Cat(X.CRUSH_REPO)
        for l in T.git('ls-tree', '-r', X.OLD_MAIN).splitlines():
            meta, path = l.split('\t', 1)
            _, typ, sha = meta.split()
            if typ != 'blob' or not path.endswith('.go'):
                continue
            s = st.get(path, '')
            if s == '':
                label = 'cal:crush-unchanged'
            elif s == 'M':
                label = 'cal:crush-modified'
            elif verdicts.get(path) == 'REWRITE':
                label = 'cal:old-rewrite'
            elif verdicts.get(path) == 'REWRITE-short':
                label = 'cal:old-shortblock'
            else:
                continue
            data = cat.get(sha)
            if data is None:
                continue
            fn = out_path('cal', label.split(':')[1], path)
            with open(fn, 'wb') as f:
                f.write(data)
            rows.append((label, 'calibration', 'code', fn, path))
        cat.close()
    with open(out_path('lists', 'targets.tsv'), 'w') as f:
        for r in rows:
            f.write('\t'.join(r) + '\n')
    import collections
    c = collections.Counter((r[0], r[1]) for r in rows)
    for k in sorted(c):
        print('  %-24s %-14s %d' % (k[0], k[1], c[k]), flush=True)
    return {'%s/%s' % k: v for k, v in c.items()}


def ref():
    """lists/ref.tsv: unrelated Go code whose names count as ordinary: the
    generic-Go reference the line scan uses, and the control programs."""
    n = 0
    with open(out_path('lists', 'ref.tsv'), 'w') as f:
        for d in T.GENERIC_DIRS:
            for p in X.walk_files(d, exts=('.go',)):
                f.write('%s\tref:%s\n' % (p, os.path.relpath(p, os.path.dirname(d))))
                n += 1
        for name, d, lic, mod in X.control_dirs():
            for p in X.walk_files(d, exts=('.go',)):
                f.write('%s\t%s/%s\n' % (p, name, os.path.relpath(p, d)))
                n += 1
    print('reference Go files:', n, flush=True)
    return dict(files=n)


def need():
    """Print the modules (path@version) that are in neither module cache: the
    Charm libraries and the control programs. run.sh downloads them."""
    for m in X.CHARM_LIBS:
        if not X.moddir(m):
            print(m)
    for c in X.CONTROLS:
        if c[1] is None and not X.moddir(c[3], (X.CTLCACHE, X.MODCACHE)):
            print(c[3])
    for m in X.NAME_CONTROLS:
        if not X.moddir(m, (X.CTLCACHE, X.MODCACHE)):
            print(m)


def main():
    cmd = sys.argv[1] if len(sys.argv) > 1 else 'all'
    if cmd == 'need':
        need()
        return
    stats_fn = out_path('corpus-stats.json')
    stats = json.load(open(stats_fn)) if os.path.exists(stats_fn) else {}
    stats['commits'] = dict(fork_point=X.FORK_POINT, upstream=X.UPSTREAM, old_main=X.OLD_MAIN)
    for name, fn in (('crush', crush), ('libs', libs), ('old', old), ('deps', deps), ('targets', targets), ('ref', ref)):
        if cmd in (name, 'all'):
            stats[name] = fn()
            with open(stats_fn, 'w') as f:
                json.dump(stats, f, indent=1, sort_keys=True)


if __name__ == '__main__':
    main()
