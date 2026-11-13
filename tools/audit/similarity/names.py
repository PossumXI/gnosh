#!/usr/bin/env python3
"""names.py - the checks that are by name and by whole file.

  names.py gitsim     git's own copy detection. A scratch repository holds one
                      commit with Crush's fork-point tree under base/ and
                      upstream's tree under up/, and a second that adds the new
                      tree under new/. `git diff -C --find-copies-harder` then
                      says which new file git takes for a copy of which Crush
                      file, at 20% similarity and up. The same again with the
                      earlier gnosh's whole tree as the first commit.
                      -> gitsim-crush.txt, gitsim-old.txt
  names.py basenames  new Go files whose file name is also a Crush file's, with
                      how much each pair shares. -> basenames.tsv
  names.py idents     names Crush declares (packages, types, functions,
                      methods, constants, variables, fields, struct-tag keys)
                      that no unrelated Go code in the reference declares (Go's
                      own source, golang.org/x, gno, testify, cobra, pflag and
                      the control programs), and
                      that stand in the new tree. Needs decl-*.tsv and
                      use-*.tsv from idents.go. -> idents.tsv
  names.py api        for each package of new code, the Charm package that
                      declares the most of the same exported names, with the
                      control programs' packages beside it. -> api.tsv
  names.py rc         the words of the config-file language (flags and option
                      names) that Crush's package for it and the new
                      internal/config both hold. -> rc.tsv
  names.py terms      brand words, environment-variable names and config keys
                      of Crush's, counted in every text file of the new tree.
                      -> terms.tsv
  names.py gomod      what go.mod, go.sum and the import graph say about
                      Charm's modules. -> gomod.txt
"""
import collections
import difflib
import os
import re
import shutil
import subprocess
import sys

import common as X
from common import T


def w(name):
    return os.path.join(X.WORK, name)


def run(cmd, cwd=None, env=None, check=True):
    e = dict(os.environ)
    e.update(env or {})
    return subprocess.run(cmd, cwd=cwd, env=e, check=check, capture_output=True, text=True).stdout


# ---- git's copy detection ---------------------------------------------------

def gitsim():
    ident = dict(GIT_AUTHOR_NAME='audit', GIT_AUTHOR_EMAIL='audit@example.com',
                 GIT_COMMITTER_NAME='audit', GIT_COMMITTER_EMAIL='audit@example.com')
    for label, trees in (('crush', (('base', X.FORK_POINT), ('up', X.UPSTREAM))), ('old', (('old', X.OLD_MAIN),))):
        if not all(rev for _, rev in trees):
            continue
        d = w('gitsim-' + label)
        shutil.rmtree(d, ignore_errors=True)
        os.makedirs(d)
        run(['git', 'init', '-q', '.'], cwd=d)
        run(['git', 'config', 'core.autocrlf', 'false'], cwd=d)
        objs = run(['git', '-C', X.CRUSH_REPO, 'rev-parse', '--git-path', 'objects']).strip()
        if not os.path.isabs(objs):
            objs = os.path.join(X.CRUSH_REPO, objs)
        with open(os.path.join(d, '.git', 'objects', 'info', 'alternates'), 'w') as f:
            f.write(objs + '\n')
        for prefix, rev in trees:
            run(['git', 'read-tree', '--prefix=%s/' % prefix, rev + '^{tree}'], cwd=d)
        a = run(['git', 'commit-tree', '-m', 'crush', run(['git', 'write-tree'], cwd=d).strip()], cwd=d, env=ident).strip()
        shutil.copytree(X.NEW_TREE, os.path.join(d, 'new'), symlinks=True)
        run(['git', 'add', '-f', 'new'], cwd=d)
        b = run(['git', 'commit-tree', '-p', a, '-m', 'new', run(['git', 'write-tree'], cwd=d).strip()], cwd=d, env=ident).strip()
        out = run(['git', 'diff-tree', '-r', '-l0', '-C20%', '--find-copies-harder', '--diff-filter=CR', '--name-status', a, b], cwd=d)
        added = run(['git', 'diff-tree', '-r', '--no-renames', '--diff-filter=A', '--name-only', a, b], cwd=d).split('\n')
        with open(w('gitsim-%s.txt' % label), 'w') as f:
            f.write(out)
        rows = [l.split('\t') for l in out.splitlines()]
        hist = collections.Counter()
        for r in rows:
            hist[int(r[0][1:]) // 10 * 10] += 1
        print('%s: %d files of the new tree added; git pairs %d of them with a source at 20%% or more: %s' % (
            label, len([x for x in added if x]), len(rows), dict(sorted(hist.items()))), flush=True)
        shutil.rmtree(os.path.join(d, 'new'), ignore_errors=True)


# ---- same file names --------------------------------------------------------

def long_lines(lines):
    nl, _ = T.norm_lines(lines)
    return [n for n in nl if len(n) >= T.MIN_SIG and len(T.ALNUM.findall(n)) >= T.MIN_SIG_ALNUM and not T.IMPORT_LINE.match(n)]


def basenames():
    crush = collections.defaultdict(list)  # basename -> [(shown, disk path)]
    for l in open(w('lists/crush-core.tsv')):
        path, shown = l.rstrip('\n').split('\t')
        name = shown.rsplit('@', 1)[0]
        if name.endswith('.go'):
            crush[name.rsplit('/', 1)[-1]].append((shown, path))
    rows = []
    n_go = n_hit = 0
    for rel, group, kind in X.new_tree_files():
        if not rel.endswith('.go') or group == 'third-party':
            continue
        n_go += 1
        base = rel.rsplit('/', 1)[-1]
        if base not in crush:
            continue
        n_hit += 1
        mine = X.read_text(os.path.join(X.NEW_TREE, rel))
        ml = long_lines(mine)
        mset = set(ml)
        mcontent = [n for n in T.norm_lines(mine)[0] if len(n) >= T.MIN_CONTENT]
        best = None
        for shown, path in crush[base]:
            theirs = X.read_text(path)
            if theirs is None:
                continue
            tset = set(long_lines(theirs))
            shared = len(mset & tset)
            tcontent = [n for n in T.norm_lines(theirs)[0] if len(n) >= T.MIN_CONTENT]
            ratio = difflib.SequenceMatcher(None, mcontent, tcontent, autojunk=False).ratio() if len(mcontent) * len(tcontent) < 4_000_000 else -1
            cand = (shared, ratio, shown, len(tset))
            if best is None or cand[:2] > best[:2]:
                best = cand
        if best:
            rows.append((rel, group, len(mset), best[0], round(100.0 * best[0] / len(mset), 1) if mset else 0.0, round(best[1], 3), len(crush[base]), best[2]))
    rows.sort(key=lambda r: (-r[4], -r[5]))
    with open(w('basenames.tsv'), 'w') as f:
        f.write('path\tgroup\tdistinct_long_lines\tshared_with_best\tshared_pct\tline_sequence_ratio\tcrush_files_of_that_name\tbest_crush_file\n')
        for r in rows:
            f.write('\t'.join(str(x) for x in r) + '\n')
    print('Go files of the new tree (third-party left out): %d; with a file name Crush also has: %d' % (n_go, n_hit))
    for r in rows[:15]:
        print('  %5.1f%% lines, ratio %.3f  %s [%s] <- %s' % (r[4], r[5], r[0], r[1], r[7]))


# ---- names ------------------------------------------------------------------

def read_decl(fn):
    out = collections.defaultdict(dict)  # name -> {kind: (nfiles, first)}
    if not os.path.exists(fn):
        return out
    for l in open(fn, encoding='utf-8', errors='replace'):
        p = l.rstrip('\n').split('\t')
        if len(p) == 4 and p[2].isdigit():
            out[p[1]][p[0]] = (int(p[2]), p[3])
    return out


def idents():
    crush = read_decl(w('decl-core.tsv'))
    ext = read_decl(w('decl-ext.tsv'))
    libs = read_decl(w('decl-libs.tsv'))
    ref = read_decl(w('decl-ref.tsv'))
    use = collections.defaultdict(lambda: collections.defaultdict(lambda: [0, 0, []]))  # name -> group -> [files, count, examples]
    for l in open(w('use-targets.tsv'), encoding='utf-8', errors='replace'):
        s, g, shown, name, n = l.rstrip('\n').split('\t')
        if s == 'new-tree':
            if g == 'new' and (shown.endswith('_test.go') or '/apptest/' in shown or '/vtest/' in shown):
                g = 'new (tests)'
            e = use[name][g]
            e[0] += 1
            e[1] += int(n)
            if len(e[2]) < 3:
                e[2].append(shown)
    rows = []
    for name, groups in use.items():
        kinds = {}
        where = ''
        for src, d in (('core', crush), ('ext', ext), ('libs', libs)):
            if name in d:
                for k, v in d[name].items():
                    if not k.startswith('tag:'):
                        kinds.setdefault(k, (src, v[0], v[1]))
                where = where or src
        if not kinds:
            continue
        if name in ref:
            continue  # unrelated Go code declares it too: an ordinary name
        k = sorted(kinds, key=lambda k: ('package', 'type', 'func', 'method', 'const', 'var', 'field').index(k) if k in ('package', 'type', 'func', 'method', 'const', 'var', 'field') else 9)[0]
        src, nf, first = kinds[k]
        rows.append((name, k, ','.join(sorted(kinds)), src, nf, first,
                     groups.get('new', [0, 0, []])[0], groups.get('new (tests)', [0, 0, []])[0],
                     groups.get('carried', [0, 0, []])[0], groups.get('carried-tools', [0, 0, []])[0],
                     groups.get('third-party', [0, 0, []])[0],
                     '; '.join((groups.get('new') or groups.get('new (tests)') or groups.get('carried-tools') or groups.get('carried') or [0, 0, []])[2])))
    rows.sort(key=lambda r: (('package', 'type', 'func', 'method', 'const', 'var', 'field').index(r[1]), r[0]))
    with open(w('idents.tsv'), 'w') as f:
        f.write('name\tkind\tall_kinds\tfirst_corpus\tcrush_files_declaring\tfirst_declared_in\tnew_files\tnew_test_files\tcarried_files\tcarried_tools_files\tthird_party_files\texamples\n')
        for r in rows:
            f.write('\t'.join(str(x) for x in r) + '\n')
    c = collections.Counter()
    for r in rows:
        if r[6] or r[7]:
            c[(r[1], r[3])] += 1
    print('names Crush or a Charm library declares, that no reference code declares, and that stand in the new tree: %d' % len(rows))
    print('of them in new code, by kind and corpus:', dict(sorted(c.items())))


# ---- exported names, package by package -------------------------------------

DECL = re.compile(r'^(?:func (?:\([^)]*\) )?([A-Z]\w*)|type ([A-Z]\w*))')
MEMBER = re.compile(r'^\t([A-Z]\w*)\b')


def exported(lines):
    """The exported names a Go file declares at top level: functions, methods,
    types, and the constants and variables of its blocks."""
    out = set()
    block = False
    for l in lines:
        if block:
            if l.startswith(')'):
                block = False
                continue
            m = MEMBER.match(l)
            if m:
                out.add(m.group(1))
            continue
        if l.startswith(('const (', 'var (')):
            block = True
            continue
        m = DECL.match(l)
        if m:
            out.add(m.group(1) or m.group(2))
            continue
        m = re.match(r'^(?:const|var) ([A-Z]\w*)', l)
        if m:
            out.add(m.group(1))
    return out


def api():
    """For every package of new code, the Charm package (Crush's or a
    library's) that declares the most of the same exported names; and the
    same for the control programs' packages, to show what chance gives."""
    corp = collections.defaultdict(set)   # package -> exported names
    for fn, strip in (('crush-core.tsv', True), ('libs.tsv', False)):
        for l in open(w('lists/' + fn)):
            path, shown = l.rstrip('\n').split('\t')
            name = shown.rsplit('@', 1)[0] if strip else shown
            if not name.endswith('.go') or name.endswith('_test.go'):
                continue
            if strip:
                name = 'crush:' + name.split(':', 1)[1]
            lines = X.read_text(path)
            if lines:
                corp[name.rsplit('/', 1)[0]] |= exported(lines)
    pk = collections.defaultdict(set)
    for l in open(w('lists/targets.tsv')):
        s_, g, k, path, shown = l.rstrip('\n').split('\t')
        if not shown.endswith('.go') or shown.endswith('_test.go'):
            continue
        if s_ == 'new-tree' and g != 'new':
            continue
        if s_.startswith('cal:'):
            continue
        lines = X.read_text(path)
        if lines:
            pk[(s_, os.path.dirname(shown) or '.')] |= exported(lines)
    rows = []
    for (s_, d), names in pk.items():
        if len(names) < 8:
            continue
        best = (0, '', set())
        for c, cn in corp.items():
            sh = names & cn
            if len(sh) > best[0]:
                best = (len(sh), c, sh)
        rows.append((s_, d, len(names), best[0], round(100.0 * best[0] / len(names), 1), best[1], ' '.join(sorted(best[2]))))
    rows.sort(key=lambda r: (r[0] != 'new-tree', -r[4]))
    with open(w('api.tsv'), 'w') as f:
        f.write('set\tpackage\texported_names\tshared_with_best\tshared_pct\tbest_charm_package\tshared_names\n')
        for r in rows:
            f.write('\t'.join(str(x) for x in r) + '\n')

    def q(v, p):
        v = sorted(v)
        return v[min(len(v) - 1, int(round(p / 100 * (len(v) - 1))))] if v else 0
    for label, sel in (('new code', [r for r in rows if r[0] == 'new-tree']), ('control', [r for r in rows if r[0] != 'new-tree'])):
        pct = [r[4] for r in sel]
        print('%s: %d packages with 8 or more exported names; share of them a single Charm package also declares: p50 %.1f%% p95 %.1f%% max %.1f%%' % (
            label, len(sel), q(pct, 50), q(pct, 95), q(pct, 100)))
    for r in [r for r in rows if r[0] == 'new-tree'][:14]:
        print('  %5.1f%% %3d/%-3d %-28s <- %-60s %s' % (r[4], r[3], r[2], r[1], r[5][-60:], r[6][:150]))
    # the packages that stand out, against unrelated libraries of the same trade
    with open(w('api.tsv'), 'a') as f:
        for m in X.NAME_CONTROLS:
            d = X.moddir(m, (X.CTLCACHE, X.MODCACHE))
            if not d:
                print('  not in the module caches:', m)
                continue
            theirs = set()
            for p in X.walk_files(d, exts=('.go',)):
                if not p.endswith('_test.go'):
                    theirs |= exported(X.read_text(p) or [])
            for r in [r for r in rows if r[0] == 'new-tree' and r[2] >= 30 and r[4] >= 30.0]:
                mine = pk[('new-tree', r[1])]
                sh = mine & theirs
                f.write('\t'.join(['unrelated-library', r[1], str(len(mine)), str(len(sh)), '%.1f' % (100.0 * len(sh) / len(mine)), m, ' '.join(sorted(sh))]) + '\n')
                print('  %s shares %d of its %d exported names with %s (and %d with %s)' % (r[1], len(sh), len(mine), m, r[3], r[5].split(':', 1)[-1]))
    for r in sorted([r for r in rows if r[0] != 'new-tree'], key=lambda r: -r[4])[:8]:
        print('  ctl %5.1f%% %3d/%-3d %-40s <- %s' % (r[4], r[3], r[2], (r[0] + ' ' + r[1])[-40:], r[5][-60:]))


# ---- the words of the config file ------------------------------------------

RCWORD = re.compile(r'^--?[a-z][a-z0-9]*(?:-[a-z0-9]+)*$|^[a-z][a-z0-9]*(?:-[a-z0-9]+)+$')
GOSTR = re.compile(r'"((?:[^"\\]|\\.)*)"')


def rc():
    """Crush reads a config file of shell-like lines ("crushrc": provider
    add ... --base-url ..., option request-timeout 900), and the new gnosh
    reads the same language as "gnoshrc". This counts the words of that
    language (flags, and option names with a dash in them) that stand as
    string literals both in Crush's package for it (internal/shellconfig, at
    the fork point or upstream) and in the new internal/config, tests left
    out; and the same for the words the earlier gnosh added to that package."""
    def words(paths):
        out = set()
        for p in paths:
            for l in X.read_text(p) or []:
                for m in GOSTR.findall(l):
                    if RCWORD.match(m):
                        out.add(m)
        return out
    crush = []
    for l in open(w('lists/crush-core.tsv')):
        path, shown = l.rstrip('\n').split('\t')
        name = shown.rsplit('@', 1)[0].split(':', 1)[1]
        if name.startswith('internal/shellconfig/') and name.endswith('.go') and not name.endswith('_test.go'):
            crush.append(path)
    old = [p for p in X.walk_files(w('old/internal/shellconfig'), exts=('.go',)) if not p.endswith('_test.go')] if os.path.isdir(w('old/internal/shellconfig')) else []
    new = [p for p in X.walk_files(os.path.join(X.NEW_TREE, 'internal/config'), exts=('.go',)) if not p.endswith('_test.go')]
    cw, ow, nw = words(crush), words(old), words(new)
    with open(w('rc.tsv'), 'w') as f:
        f.write('word\tkind\tin_crush\tin_earlier_gnosh_only\tin_new_config\n')
        for x in sorted(cw | ow | nw):
            f.write('%s\t%s\t%s\t%s\t%s\n' % (x, 'flag' if x.startswith('-') else 'name', 'yes' if x in cw else '', 'yes' if x in ow - cw else '', 'yes' if x in nw else ''))
    for label, sel in (('flags', lambda x: x.startswith('-')), ('names with a dash', lambda x: not x.startswith('-'))):
        c = {x for x in cw if sel(x)}
        o = {x for x in ow - cw if sel(x)}
        n = {x for x in nw if sel(x)}
        print('%s: Crush\'s package has %d, of which the new internal/config also has %d; the earlier gnosh added %d more, of which %d; the new one has %d of its own' % (
            label, len(c), len(c & n), len(o), len(o & n), len(n - c - o)))


# ---- brand words, environment variables, config keys ------------------------

BRANDS = ['crush', 'charm', 'charmbracelet', 'charmtone', 'catwalk', 'fantasy', 'bubbletea', 'bubble tea', 'bubbles',
          'lipgloss', 'lip gloss', 'glamour', 'ultraviolet', 'opencode', 'termai', 'klash', 'crash', 'hyper']
ENVNAME = re.compile(r'^[A-Z][A-Z0-9]*(?:_[A-Z0-9]+)+$')


def terms():
    files = []
    for rel, group, kind in X.new_tree_files():
        lines = X.read_text(os.path.join(X.NEW_TREE, rel))
        if lines is None:
            continue
        if group == 'new' and kind == 'code' and (rel.endswith('_test.go') or '/apptest/' in rel or '/vtest/' in rel):
            kind = 'test'
        text = '\n'.join(lines)
        files.append((rel, group, kind, text,
                      collections.Counter(re.findall(r'[A-Z][A-Z0-9_]*', text)),
                      collections.Counter(re.findall(r'[a-z][a-z0-9]*(?:[_-][a-z0-9]+)+', text))))
    out = open(w('terms.tsv'), 'w')
    out.write('class\tterm\tgroup\tkind\tfiles\toccurrences\texamples\n')

    def emit(cls, term, per_file):
        agg = collections.defaultdict(lambda: [0, 0, []])
        for (rel, group, kind), n in per_file:
            if n:
                e = agg[(group, kind)]
                e[0] += 1
                e[1] += n
                if len(e[2]) < 4:
                    e[2].append('%s(%d)' % (rel, n))
        for (group, kind), e in sorted(agg.items()):
            out.write('\t'.join([cls, term, group, kind, str(e[0]), str(e[1]), '; '.join(e[2])]) + '\n')
        return sum(e[1] for e in agg.values())

    for b in BRANDS:
        rx = re.compile(r'(?<![a-z])' + re.escape(b) + r'(?![a-z])', re.I)
        emit('brand', b, [((f[0], f[1], f[2]), len(rx.findall(f[3]))) for f in files])
    # environment-variable names that stand as string literals in Crush's code
    env = set()
    if not os.path.exists(w('strings-ext-all.tsv')):
        print('missing strings-ext-all.tsv (tokscan -strings -uniq -minstr 3 of the ext list)')
    else:
        for l in open(w('strings-ext-all.tsv'), encoding='utf-8', errors='replace'):
            p = l.rstrip('\n').split('\t')
            if len(p) >= 3 and ENVNAME.match(p[2].strip('"')):
                env.add(p[2].strip('"'))
    ref_env = set()
    if os.path.exists(w('strings-ref-all.tsv')):
        for l in open(w('strings-ref-all.tsv'), encoding='utf-8', errors='replace'):
            p = l.rstrip('\n').split('\t')
            if len(p) >= 3 and ENVNAME.match(p[-1].strip('"')):
                ref_env.add(p[-1].strip('"'))
    n_env = 0
    for e in sorted(env - ref_env):
        n_env += 1 if emit('env', e, [((f[0], f[1], f[2]), f[4].get(e, 0)) for f in files]) else 0
    for prefix in ('CRUSH_', 'CHARM_', 'CATWALK_', 'FANTASY_', 'OPENCODE_', 'BUBBLETEA_', 'GLAMOUR_', 'LIPGLOSS_', 'KLASH_', 'CRASH_', 'GNOSH_'):
        emit('env-prefix', prefix, [((f[0], f[1], f[2]), sum(n for t, n in f[4].items() if t.startswith(prefix) and len(t) > len(prefix))) for f in files])
    # config keys: what Crush's struct tags name under json, yaml and toml,
    # less what unrelated code's tags also name
    crush = read_decl(w('decl-core.tsv'))
    ext = read_decl(w('decl-ext.tsv'))
    ref = read_decl(w('decl-ref.tsv'))
    reftags = {n for n, d in ref.items() if any(k.startswith('tag:') for k in d)}
    keys = set()
    for d in (crush, ext):
        for n, kinds in d.items():
            if any(k in ('tag:json', 'tag:yaml', 'tag:toml', 'tag:env', 'tag:flag', 'tag:mapstructure') for k in kinds):
                if n not in reftags and re.match(r'^[a-z][a-z0-9]*(?:[_-][a-z0-9]+)+$', n):
                    keys.add(n)
    n_key = 0
    for k in sorted(keys):
        n_key += 1 if emit('config-key', k, [((f[0], f[1], f[2]), f[5].get(k, 0)) for f in files]) else 0
    out.close()
    print('environment names in Crush strings and in no reference string: %d, of which in the new tree: %d' % (len(env - ref_env), n_env))
    print('config keys (multi-word struct-tag names of Crush, in no reference tag): %d, of which in the new tree: %d' % (len(keys), n_key))


# ---- go.mod -----------------------------------------------------------------

def gomod():
    out = []
    pat = re.compile(r'charm', re.I)
    for fn in ('go.mod', 'go.sum'):
        text = open(os.path.join(X.NEW_TREE, fn)).read()
        hits = [l for l in text.splitlines() if pat.search(l)]
        out.append('%s: %d lines, %d naming charm' % (fn, len(text.splitlines()), len(hits)))
        out.extend('   ' + h for h in hits)
    env = dict(GOFLAGS='-mod=readonly', GOPROXY='off', GOTOOLCHAIN='local', PATH='/usr/local/go/bin:' + os.environ.get('PATH', ''))
    for title, cmd in (('modules in the build list (go list -m all)', ['go', 'list', '-m', 'all']),
                       ('packages in the import graph, tests included (go list -deps -test ./...)', ['go', 'list', '-deps', '-test', './...'])):
        try:
            r = subprocess.run(cmd, cwd=X.NEW_TREE, env=dict(os.environ, **env), capture_output=True, text=True)
            names = [l for l in r.stdout.splitlines() if l]
            hits = [l for l in names if pat.search(l)]
            out.append('%s: %d, %d naming charm%s' % (title, len(names), len(hits), '' if r.returncode == 0 else ' (go exited %d: %s)' % (r.returncode, r.stderr.strip()[:300])))
            out.extend('   ' + h for h in hits)
        except OSError as e:
            out.append('%s: could not run go: %s' % (title, e))
    # an import of a Charm module anywhere in the tree's Go files
    n = 0
    imp = re.compile(r'^\s*(?:[\w.]+\s+)?"((?:charm\.land|github\.com/charmbracelet)/[^"]+)"')
    for rel, group, kind in X.new_tree_files():
        if rel.endswith(('.go', '.gno')):
            for i, l in enumerate(X.read_text(os.path.join(X.NEW_TREE, rel)) or []):
                m = imp.match(l)
                if m:
                    n += 1
                    out.append('   import of %s at %s:%d' % (m.group(1), rel, i + 1))
    out.append('import lines naming a Charm module in the tree\'s Go files: %d' % n)
    with open(w('gomod.txt'), 'w') as f:
        f.write('\n'.join(out) + '\n')
    print('\n'.join(out))


if __name__ == '__main__':
    cmds = dict(gitsim=gitsim, basenames=basenames, idents=idents, api=api, rc=rc, terms=terms, gomod=gomod)
    for c in sys.argv[1:] or list(cmds):
        cmds[c]()
