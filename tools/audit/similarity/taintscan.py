#!/usr/bin/env python3
"""taintscan.py - how much of a file also exists in Charm's Crush code.

Run on the devbox only, from /root/src/gnosh-old-scan (never inside a repo).

Corpus (Charm's code), two tiers:
  core = every blob of the fork-point tree
       + every blob that upstream commits in fork-point..upstream/main brought in
         (so: upstream/main's tree, and every line any of those commits added)
  ext  = core + every blob reachable from ANY upstream branch or tag
         (Crush's whole history, unmerged branches, code deleted before the fork)

Lines are whitespace-normalized (trimmed, inner runs of whitespace collapsed) and
brand-folded (crush/crash/klash/gnosh, any case, become one token) so a copied
line whose only change is the product name still matches.

Measures per target file:
  (a) share of SIGNIFICANT lines found in the corpus. Significant = at least 25
      characters and 12 letters/digits after normalizing, not an import-path
      line, and not found in the generic-Go reference (Go's own source,
      golang.org/x, gno, testify, cobra, pflag): a line that unrelated code
      also has is idiom, not evidence.
  (b1) literal runs: >= 4 consecutive non-blank lines that are each somewhere in
      the corpus (and at least one of them significant).
  (b2) shingle runs: 4 consecutive CONTENT lines (>= 12 characters) that appear
      consecutively, in the same order, in one corpus blob. Overlapping hits are
      merged into runs; a run counts when it holds >= 2 significant lines.
      This is the real signal.
"""
import argparse
import collections
import hashlib
import os
import pickle
import re
import subprocess
import sys
import time

REPO = '/root/src/gnosh-old'
SCAN = '/root/src/gnosh-old-scan'
BASE = '52c75da8acbcd11dc269ae582f3ed7947ba3cff6'
MAIN = '1c77d2bd58482c37dd858b24326a6e1829117904'
UPSTREAM = 'upstream/main'

K = 4
MIN_CONTENT = 12
MIN_SIG = 25
MIN_SIG_ALNUM = 12
RUN_MIN_SIG = 2

GENERIC_DIRS = [
    '/usr/local/go/src',
    '/root/go/pkg/mod/golang.org/x/tools@v0.52.0',
    '/root/go/pkg/mod/golang.org/x/net@v0.61.0',
    '/root/go/pkg/mod/golang.org/x/sys@v0.49.0',
    '/root/go/pkg/mod/golang.org/x/crypto@v0.58.0',
    '/root/go/pkg/mod/golang.org/x/text@v0.43.0',
    '/root/go/pkg/mod/golang.org/x/sync@v0.24.0',
    '/root/go/pkg/mod/golang.org/x/mod@v0.42.0',
    '/root/go/pkg/mod/golang.org/x/term@v0.47.0',
    '/root/go/pkg/mod/golang.org/x/image@v0.47.0',
    '/root/go/pkg/mod/github.com/gnolang/gno@v1.5.1-0.20260928125126-5cdbc25fcde0',
    '/root/go/pkg/mod/github.com/stretchr/testify@v1.12.1',
    '/root/go/pkg/mod/github.com/spf13/cobra@v1.10.2',
    '/root/go/pkg/mod/github.com/spf13/pflag@v1.0.10',
]

WS = re.compile(r'\s+')
BRAND = re.compile(r'crush|klash|crash|gnosh', re.I)
IMPORT_LINE = re.compile(r'^(?:[A-Za-z_][\w.]* |\. |_ )?"[^" ]+"$')
ALNUM = re.compile(r'[A-Za-z0-9]')


def norm(line):
    return BRAND.sub('@', WS.sub(' ', line.strip()))


def h(s):
    return int.from_bytes(hashlib.blake2b(s.encode('utf-8', 'surrogatepass'), digest_size=8).digest(), 'little')


def text_lines(data):
    if data is None or b'\0' in data[:8192]:
        return None
    return data.decode('utf-8', 'replace').split('\n')


def norm_lines(lines):
    out = []
    nos = []
    for i, l in enumerate(lines):
        n = norm(l)
        if n:
            out.append(n)
            nos.append(i + 1)
    return out, nos


class Cat:
    def __init__(self, repo):
        self.p = subprocess.Popen(['git', '-C', repo, 'cat-file', '--batch'],
                                  stdin=subprocess.PIPE, stdout=subprocess.PIPE)

    def get(self, name):
        self.p.stdin.write(name.encode() + b'\n')
        self.p.stdin.flush()
        hdr = self.p.stdout.readline().split()
        if len(hdr) < 3:
            return None
        n = int(hdr[2])
        data = self.p.stdout.read(n)
        self.p.stdout.read(1)
        if hdr[1] != b'blob':
            return None
        return data

    def close(self):
        self.p.stdin.close()
        self.p.wait()


def git(*args):
    return subprocess.run(['git', '-C', REPO] + list(args), check=True, capture_output=True, text=True).stdout


class Corpus:
    def __init__(self):
        self.paths = []          # id -> "tier:path"
        self.core_lines = set()
        self.ext_lines = set()   # superset of core_lines
        self.core_sh = {}        # shingle -> path id (first seen)
        self.ext_sh = {}         # superset of core_sh
        self.core_blobs = set()
        self.ext_blobs = set()
        self.generic = set()
        self.stats = {}
        self.line_paths = {}     # long-line hash -> set of path ids (capped) that hold it

    def note_paths(self, hs, nl, pid):
        lp = self.line_paths
        for x, n in zip(hs, nl):
            if len(n) >= MIN_SIG:
                s = lp.get(x)
                if s is None:
                    lp[x] = {pid}
                elif len(s) < 40:
                    s.add(pid)

    def add(self, lines, pid, core):
        nl, _ = norm_lines(lines)
        hs = [h(n) for n in nl]
        self.ext_lines.update(hs)
        if core:
            self.core_lines.update(hs)
        self.note_paths(hs, nl, pid)
        content = [x for x, n in zip(hs, nl) if len(n) >= MIN_CONTENT]
        for i in range(len(content) - K + 1):
            key = hash(tuple(content[i:i + K]))
            if key not in self.ext_sh:
                self.ext_sh[key] = pid
            if core and key not in self.core_sh:
                self.core_sh[key] = pid
        return len(nl)


def build_corpus():
    t0 = time.time()
    C = Corpus()
    cat = Cat(REPO)
    seen = set()

    def take(sha, path, tier, core):
        if sha in seen:
            if core and sha not in C.core_blobs:
                # seen before only as ext: cannot happen with our ordering (core first)
                pass
            return 0
        seen.add(sha)
        data = cat.get(sha)
        if data is None:
            return 0
        C.ext_blobs.add(sha)
        if core:
            C.core_blobs.add(sha)
        lines = text_lines(data)
        if lines is None:
            return 0
        pid = len(C.paths)
        C.paths.append(tier + ':' + path)
        return C.add(lines, pid, core)

    # 1. fork-point tree
    n = b = 0
    for l in git('ls-tree', '-r', BASE).splitlines():
        meta, path = l.split('\t', 1)
        _, typ, sha = meta.split()
        if typ != 'blob':
            continue
        n += take(sha, path, 'base', True)
        b += 1
    C.stats['base'] = (b, n)
    print('base tree: %d blobs, %d lines, %.0fs' % (b, n, time.time() - t0), flush=True)

    # 2. everything upstream commits brought in after the fork point
    n = b = 0
    for l in git('rev-list', '--objects', BASE + '..' + UPSTREAM).splitlines():
        if ' ' not in l:
            continue
        sha, path = l.split(' ', 1)
        k = take(sha, path, 'up', True)
        n += k
        b += 1 if k else 0
    C.stats['range'] = (b, n)
    print('upstream range: %d text blobs, %d lines, %.0fs' % (b, n, time.time() - t0), flush=True)

    # 3. every other upstream branch and tag, whole history
    n = b = 0
    for l in git('rev-list', '--objects', '--remotes=upstream', '--tags').splitlines():
        if ' ' not in l:
            continue
        sha, path = l.split(' ', 1)
        k = take(sha, path, 'hist', False)
        n += k
        b += 1 if k else 0
        if b % 2000 == 0 and k:
            print('  hist %d blobs, %.0fs' % (b, time.time() - t0), flush=True)
    C.stats['hist'] = (b, n)
    print('upstream history+branches: %d text blobs, %d lines, %.0fs' % (b, n, time.time() - t0), flush=True)
    cat.close()

    # 4. generic Go reference
    nfiles = nlines = 0
    for d in GENERIC_DIRS:
        if not os.path.isdir(d):
            print('  generic dir missing:', d, flush=True)
            continue
        for root, dirs, files in os.walk(d):
            for f in files:
                if not (f.endswith('.go') or f.endswith('.gno')):
                    continue
                try:
                    data = open(os.path.join(root, f), 'rb').read()
                except OSError:
                    continue
                lines = text_lines(data)
                if lines is None:
                    continue
                nfiles += 1
                for l in lines:
                    nrm = norm(l)
                    if len(nrm) >= MIN_SIG:
                        C.generic.add(h(nrm))
                        nlines += 1
    C.stats['generic'] = (nfiles, nlines)
    print('generic: %d files, %d long lines, %d distinct, %.0fs' % (nfiles, nlines, len(C.generic), time.time() - t0), flush=True)
    C.stats['sizes'] = dict(core_lines=len(C.core_lines), ext_lines=len(C.ext_lines),
                            core_sh=len(C.core_sh), ext_sh=len(C.ext_sh),
                            core_blobs=len(C.core_blobs), ext_blobs=len(C.ext_blobs),
                            generic=len(C.generic))
    print(C.stats, flush=True)
    return C


def load_corpus():
    fn = os.path.join(SCAN, 'corpus.pkl')
    if os.path.exists(fn):
        C = Corpus()
        with open(fn, 'rb') as f:
            C.__dict__.update(pickle.load(f))
        return C
    C = build_corpus()
    with open(fn, 'wb') as f:
        pickle.dump(C.__dict__, f, protocol=pickle.HIGHEST_PROTOCOL)
    return C


def is_sig(nrm, hv, C):
    if len(nrm) < MIN_SIG:
        return False
    if len(ALNUM.findall(nrm)) < MIN_SIG_ALNUM:
        return False
    if IMPORT_LINE.match(nrm):
        return False
    return hv not in C.generic


def runs_of(flags):
    """maximal runs of True in flags -> list of (start, end_exclusive)"""
    out = []
    i = 0
    n = len(flags)
    while i < n:
        if flags[i]:
            j = i
            while j < n and flags[j]:
                j += 1
            out.append((i, j))
            i = j
        else:
            i += 1
    return out


def scan_lines(lines, C, detail=False):
    nl, nos = norm_lines(lines)
    hs = [h(n) for n in nl]
    n = len(hs)
    sig = [is_sig(a, b, C) for a, b in zip(nl, hs)]
    res = collections.OrderedDict()
    res['lines'] = n
    res['sig'] = sum(sig)
    cidx = [i for i in range(n) if len(nl[i]) >= MIN_CONTENT]
    res['content'] = len(cidx)
    dsig = {}
    for i in range(n):
        if sig[i] and hs[i] not in dsig:
            dsig[hs[i]] = i
    res['sigd'] = len(dsig)
    # best single Charm file: the one sharing the most distinct significant lines
    pc = collections.Counter()
    for x in dsig:
        for pid in C.line_paths.get(x, ()):
            pc[pid] += 1
    if pc:
        rank = {'base': 0, 'up': 1, 'hist': 2}
        pid, cnt = min(pc.items(), key=lambda kv: (-kv[1], rank.get(C.paths[kv[0]].split(':', 1)[0], 3), kv[0]))
        res['best_src'] = C.paths[pid]
        res['best_src_n'] = cnt
    else:
        res['best_src'] = ''
        res['best_src_n'] = 0
    det = {}
    for tier, lineset, shmap in (('core', C.core_lines, C.core_sh), ('ext', C.ext_lines, C.ext_sh)):
        inl = [x in lineset for x in hs]
        m = sum(1 for i in range(n) if sig[i] and inl[i])
        res[tier + '_sig_match'] = m
        res[tier + '_sig_pct'] = round(100.0 * m / res['sig'], 1) if res['sig'] else 0.0
        md = sum(1 for x in dsig if x in lineset)
        res[tier + '_sigd_match'] = md
        res[tier + '_sigd_pct'] = round(100.0 * md / len(dsig), 1) if dsig else 0.0
        # (b1) literal runs
        lr = [(a, b) for a, b in runs_of(inl) if b - a >= 4 and any(sig[a:b])]
        res[tier + '_lrun_longest'] = max((b - a for a, b in lr), default=0)
        res[tier + '_lrun_n'] = len(lr)
        res[tier + '_lrun_lines'] = sum(b - a for a, b in lr)
        # (b2) shingle runs over content lines
        cov = [False] * len(cidx)
        hitpid = [None] * len(cidx)
        for j in range(len(cidx) - K + 1):
            key = hash(tuple(hs[cidx[t]] for t in range(j, j + K)))
            pid = shmap.get(key)
            if pid is not None:
                for t in range(j, j + K):
                    cov[t] = True
                    if hitpid[t] is None:
                        hitpid[t] = pid
        sr_all = runs_of(cov)
        sr = []
        for a, b in sr_all:
            w = sum(1 for t in range(a, b) if sig[cidx[t]])
            if w >= RUN_MIN_SIG:
                sr.append((a, b, w))
        res[tier + '_srun_longest'] = max((b - a for a, b, w in sr), default=0)
        res[tier + '_srun_sigmax'] = max((w for a, b, w in sr), default=0)
        res[tier + '_srun_n'] = len(sr)
        res[tier + '_srun_lines'] = sum(b - a for a, b, w in sr)
        res[tier + '_srun_pct'] = round(100.0 * res[tier + '_srun_lines'] / len(cidx), 1) if cidx else 0.0
        res[tier + '_srun_weak'] = sum(b - a for a, b in sr_all) - res[tier + '_srun_lines']
        pc = collections.Counter()
        for a, b, w in sr:
            for t in range(a, b):
                if hitpid[t] is not None:
                    pc[hitpid[t]] += 1
        res[tier + '_top'] = '; '.join('%s(%d)' % (C.paths[p], c) for p, c in pc.most_common(3))
        if detail:
            det[tier] = dict(sr=[(nos[cidx[a]], nos[cidx[b - 1]], b - a, w, C.paths[hitpid[a]] if hitpid[a] is not None else '') for a, b, w in sr],
                             lr=[(nos[a], nos[b - 1], b - a) for a, b in lr],
                             sigmatch=[nos[i] for i in range(n) if sig[i] and inl[i]])
    if detail:
        return res, det
    return res


COLS = ['lines', 'content', 'sig', 'sigd',
        'core_sig_match', 'core_sig_pct', 'core_sigd_match', 'core_sigd_pct', 'core_lrun_longest', 'core_lrun_n', 'core_lrun_lines',
        'core_srun_longest', 'core_srun_sigmax', 'core_srun_n', 'core_srun_lines', 'core_srun_pct', 'core_srun_weak',
        'ext_sig_match', 'ext_sig_pct', 'ext_sigd_match', 'ext_sigd_pct', 'ext_lrun_longest', 'ext_lrun_n', 'ext_lrun_lines',
        'ext_srun_longest', 'ext_srun_sigmax', 'ext_srun_n', 'ext_srun_lines', 'ext_srun_pct', 'ext_srun_weak',
        'best_src_n', 'best_src', 'core_top', 'ext_top']


def scan_git_paths(C, rev, paths, label, out):
    cat = Cat(REPO)
    tree = {}
    for l in git('ls-tree', '-r', '-l', rev).splitlines():
        meta, path = l.split('\t', 1)
        mode, typ, sha, size = meta.split()
        tree[path] = (sha, size, mode)
    for p in paths:
        if p not in tree:
            continue
        sha, size, mode = tree[p]
        exact = 'core' if sha in C.core_blobs else ('ext' if sha in C.ext_blobs else '')
        data = cat.get(sha)
        lines = text_lines(data)
        row = [label, p, size, 'text' if lines is not None else 'binary', exact]
        if lines is None:
            row += [''] * len(COLS)
        else:
            r = scan_lines(lines, C)
            row += [str(r[c]) for c in COLS]
        out.write('\t'.join(row) + '\n')
    cat.close()


def scan_fs(C, root, label, out, exts=('.go',), limit=None):
    k = 0
    for dp, dn, fn in os.walk(root):
        for f in sorted(fn):
            if not f.endswith(exts):
                continue
            p = os.path.join(dp, f)
            data = open(p, 'rb').read()
            lines = text_lines(data)
            if lines is None:
                continue
            r = scan_lines(lines, C)
            out.write('\t'.join([label, os.path.relpath(p, root), str(len(data)), 'text', ''] + [str(r[c]) for c in COLS]) + '\n')
            k += 1
            if limit and k >= limit:
                return


def show(C, rev, path, tier='ext', maxrun=400):
    cat = Cat(REPO)
    data = cat.get(rev + ':' + path)
    cat.close()
    lines = text_lines(data)
    r, det = scan_lines(lines, C, detail=True)
    print(path)
    print({k: r[k] for k in COLS})
    d = det[tier]
    for a, b, n, w, cp in d['sr']:
        print('--- shingle run lines %d-%d (%d content lines, %d significant) <- %s' % (a, b, n, w, cp))
        for i in range(a, min(b, a + maxrun) + 1):
            print('   %5d| %s' % (i, lines[i - 1][:160]))
    print('--- literal runs:', d['lr'])
    srcov = set()
    for a, b, n, w, cp in d['sr']:
        srcov.update(range(a, b + 1))
    rest = [i for i in d['sigmatch'] if i not in srcov]
    print('--- significant single-line matches outside shingle runs: %d' % len(rest))
    for i in rest[:60]:
        print('   %5d| %s' % (i, lines[i - 1][:160]))


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument('cmd', choices=['build', 'added', 'tree', 'fs', 'show'])
    ap.add_argument('--out')
    ap.add_argument('--root')
    ap.add_argument('--label', default='')
    ap.add_argument('--path')
    ap.add_argument('--tier', default='ext')
    ap.add_argument('--rev', default=MAIN)
    ap.add_argument('--limit', type=int)
    a = ap.parse_args()
    C = load_corpus()
    if a.cmd == 'build':
        print(C.stats)
        return
    hdr = '\t'.join(['set', 'path', 'bytes', 'kind', 'exact_blob'] + COLS) + '\n'
    if a.cmd == 'added':
        added = git('diff', '--no-renames', '--diff-filter=A', '--name-only', BASE, MAIN).splitlines()
        with open(a.out, 'w') as out:
            out.write(hdr)
            scan_git_paths(C, MAIN, added, 'added', out)
    elif a.cmd == 'tree':
        # every file of gnosh main, labelled added / modified / unchanged
        st = {}
        for l in git('diff', '--no-renames', '--name-status', BASE, MAIN).splitlines():
            s, p = l.split('\t', 1)
            st[p] = {'A': 'added', 'M': 'modified', 'D': 'deleted'}[s]
        allp = git('ls-tree', '-r', '--name-only', MAIN).splitlines()
        with open(a.out, 'w') as out:
            out.write(hdr)
            for lab in ('added', 'modified', 'unchanged'):
                scan_git_paths(C, MAIN, [p for p in allp if st.get(p, 'unchanged') == lab], lab, out)
    elif a.cmd == 'fs':
        with open(a.out, 'a') as out:
            scan_fs(C, a.root, a.label, out, limit=a.limit)
    elif a.cmd == 'show':
        show(C, a.rev, a.path, a.tier)


if __name__ == '__main__':
    main()
