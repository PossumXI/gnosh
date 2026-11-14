#!/usr/bin/env python3
"""common.py - settings and helpers shared by the similarity audit's scripts.

Every place and commit comes from the environment, with the devbox's values as
defaults, so run.sh can point the audit at any checkout:

  CRUSH_REPO   a git clone whose remote "upstream" is charmbracelet/crush, with
               all of its branches and tags fetched (the earlier gnosh's clone
               serves, and then OLD_MAIN is in it too)
  FORK_POINT   the commit of Crush the earlier gnosh was forked from
  UPSTREAM     the commit of Crush's main branch compared against
  OLD_MAIN     the earlier gnosh's last commit (for calibration and for the
               "earlier gnosh" corpus); empty to leave those out
  NEW_TREE     a clean copy of the commit of the new gnosh being audited
  WORK         where raw working files and outputs go
  GOROOT_SRC   Go's own source tree
  MODCACHE     a Go module cache holding the Charm libraries and the other
               dependencies at the versions the earlier gnosh pinned
  CTLCACHE     a Go module cache holding the control programs
  INVENTORY    the earlier inventory's output folder (scan.tsv,
               classified.json); empty to leave out what needs it
  SINCE_REPO   a second clone (it may borrow CRUSH_REPO's objects) whose
               remote "upstream" was fetched later than CRUSH_REPO's; what it
               holds that CRUSH_REPO's upstream refs do not reach is the
               corpus "since". Empty to leave that corpus out.
  UPSTREAM_NOW the commit of Crush's main branch in SINCE_REPO
"""
import os
import re
import subprocess
import sys

HERE = os.path.dirname(os.path.abspath(__file__))


def env(name, default):
    v = os.environ.get(name)
    return default if v is None else v


CRUSH_REPO = env('CRUSH_REPO', '/root/src/gnosh-old')
FORK_POINT = env('FORK_POINT', '52c75da8acbcd11dc269ae582f3ed7947ba3cff6')
UPSTREAM = env('UPSTREAM', '7ef47d5c85f3b2b55eee234b51ba5275d61bab4a')
OLD_MAIN = env('OLD_MAIN', '1c77d2bd58482c37dd858b24326a6e1829117904')
NEW_TREE = env('NEW_TREE', '/root/src/gnosh-new-audit/tree')
WORK = env('WORK', '/root/src/gnosh-new-audit/work')
GOROOT_SRC = env('GOROOT_SRC', '/usr/local/go/src')
MODCACHE = env('MODCACHE', '/root/go/pkg/mod')
CTLCACHE = env('CTLCACHE', '/root/src/gnosh-new-audit/modcache')
INVENTORY = env('INVENTORY', '/root/src/gnosh-old-scan/out')
SINCE_REPO = env('SINCE_REPO', '')
UPSTREAM_NOW = env('UPSTREAM_NOW', '')

sys.path.insert(0, HERE)
import taintscan as T  # noqa: E402  (the earlier inventory's script, unchanged)

T.REPO = CRUSH_REPO
T.SCAN = WORK
T.BASE = FORK_POINT
T.MAIN = OLD_MAIN
T.UPSTREAM = UPSTREAM

# Charm's separately published libraries, at the versions the earlier gnosh's
# go.mod pinned. Corpus (d).
CHARM_LIBS = [
    'charm.land/bubbles/v2@v2.2.1',
    'charm.land/bubbletea/v2@v2.1.0',
    'charm.land/catwalk@v0.52.49',
    'charm.land/fang/v2@v2.0.1',
    'charm.land/fantasy@v0.45.1',
    'charm.land/glamour/v2@v2.0.1',
    'charm.land/lipgloss/v2@v2.0.6',
    'charm.land/log/v2@v2.0.1',
    'charm.land/x/vcr@v0.1.1',
    'github.com/charmbracelet/colorprofile@v0.4.3',
    'github.com/charmbracelet/openai-go@v0.0.0-20260921175203-216db9e71b83',
    'github.com/charmbracelet/ultraviolet@v0.0.0-20261008173134-6b8d4baf91b4',
    'github.com/charmbracelet/x/ansi@v0.11.9',
    'github.com/charmbracelet/x/editor@v0.2.0',
    'github.com/charmbracelet/x/etag@v0.2.0',
    'github.com/charmbracelet/x/exp/charmtone@v0.1.0',
    'github.com/charmbracelet/x/exp/golden@v0.1.0',
    'github.com/charmbracelet/x/exp/ordered@v0.1.0',
    'github.com/charmbracelet/x/exp/slice@v0.1.0',
    'github.com/charmbracelet/x/exp/strings@v0.1.0',
    'github.com/charmbracelet/x/powernap@v0.1.6',
    'github.com/charmbracelet/x/term@v0.2.2',
    'github.com/charmbracelet/x/termios@v0.1.1',
    'github.com/charmbracelet/x/windows@v0.2.2',
]

# The control set: unrelated Go code of a similar kind. The first three are
# packages of Go's standard library, the rest are permissively licensed
# command-line programs. (name, root directory, licence)
CONTROLS = [
    ('ctl:go-net-http', os.path.join(GOROOT_SRC, 'net/http'), 'BSD-3-Clause'),
    ('ctl:go-os-exec', os.path.join(GOROOT_SRC, 'os/exec'), 'BSD-3-Clause'),
    ('ctl:go-text-template', os.path.join(GOROOT_SRC, 'text/template'), 'BSD-3-Clause'),
]
for _name, _mod, _lic in (
        ('ctl:fzf', 'github.com/junegunn/fzf@v0.75.0', 'MIT'),
        ('ctl:ollama', 'github.com/ollama/ollama@v0.40.16', 'MIT'),
        ('ctl:gh', 'github.com/cli/cli/v2@v2.104.0', 'MIT'),
        ('ctl:mvdan-sh', 'mvdan.cc/sh/v3@v3.14.1', 'BSD-3-Clause')):
    CONTROLS.append((_name, None, _lic, _mod))

# Unrelated libraries that do what a Charm library does, for one comparison
# only: how many exported names a package of new code shares with them
# (names.py api). Not part of the control set.
NAME_CONTROLS = [
    'github.com/gdamore/tcell/v2@v2.13.10',   # a terminal library, Apache-2.0
    'github.com/muesli/termenv@v0.16.0',      # a terminal colour library, MIT
]

TEXT_EXTS_SKIP = ('.png', '.jpg', '.jpeg', '.gif', '.ico', '.pdf', '.gz', '.zip', '.db', '.ldb', '.wasm', '.ttf', '.woff', '.woff2')


def modpath(mod):
    """A module path with a version, as the module cache spells its folder:
    capitals become an exclamation mark and the small letter."""
    return re.sub(r'[A-Z]', lambda m: '!' + m.group(0).lower(), mod)


def moddir(mod, caches=None):
    for c in caches or (MODCACHE, CTLCACHE):
        d = os.path.join(c, modpath(mod))
        if os.path.isdir(d):
            return d
    return None


def control_dirs():
    out = []
    for c in CONTROLS:
        if c[1] is not None:
            out.append((c[0], c[1], c[2], ''))
        else:
            d = moddir(c[3], (CTLCACHE, MODCACHE))
            if d:
                out.append((c[0], d, c[2], c[3]))
            else:
                print('control missing from the module caches:', c[3], file=sys.stderr)
    return out


def git(repo, *args):
    return subprocess.run(['git', '-C', repo] + list(args), check=True, capture_output=True, text=True).stdout


def read_text(path):
    try:
        data = open(path, 'rb').read()
    except OSError:
        return None
    return T.text_lines(data)


def walk_files(root, exts=None, skip_dirs=('.git',)):
    for dp, dn, fn in os.walk(root):
        dn[:] = sorted(d for d in dn if d not in skip_dirs)
        for f in sorted(fn):
            if exts and not f.endswith(exts):
                continue
            yield os.path.join(dp, f)


# ---- the new tree's files, grouped ------------------------------------------

ROW = re.compile(r'^\| `([^`]+)` \| `([^`]+)` \| ([^|]+?) \| ([\d,]+) \| ([\d.]+%|n/a \(binary\)) \| (\d+|n/a \(binary\)) \| `([0-9a-f]{64})` \| (.*?) \|$')
TOOLROW = re.compile(r'^\| `([^`]+)` \| `([^`]+)` \| (\w+) \| (\d+) \| ([\d,]+) \| ([\d.]+)% \| (\d+) \| `([0-9a-f]{64})` \| (.*?) \|$')
# "The bench": one path (the file kept the path it had), then how it came.
BENCHROW = re.compile(r'^\| `([^`]+)` \| (carried|carried with changes|not brought) \| ([\d,]+) \| ([\d.]+)% \| (\d+) \| `([0-9a-f]{64})` \| ([^|]*?) \| (.*?) \|$')


def carried_table(tree=None):
    """CARRIED.md's three tables: {new path: dict(old, kind, bytes, share, run,
    sha, change)}. The kinds: carried, carried with changes, third-party (the
    table of files); carried with larger changes (the tools); and, for the
    bench, "bench: carried", "bench: carried with changes" and "bench: not
    brought" (a file that was staged and is not in the tree)."""
    tree = tree or NEW_TREE
    out = {}
    for line in open(os.path.join(tree, 'CARRIED.md'), encoding='utf-8'):
        line = line.rstrip('\n')
        m = ROW.match(line)
        if m:
            binary = m.group(5).startswith('n/a')
            out[m.group(1)] = dict(old=m.group(2), kind=m.group(3).strip(), bytes=int(m.group(4).replace(',', '')),
                                   share=None if binary else float(m.group(5).rstrip('%')),
                                   run=None if binary else int(m.group(6)), sha=m.group(7), change=m.group(8))
            continue
        m = TOOLROW.match(line)
        if m:
            out[m.group(1)] = dict(old=m.group(2), kind='carried with larger changes', verdict=m.group(3),
                                   bytes=int(m.group(5).replace(',', '')), share=float(m.group(6)), run=int(m.group(7)),
                                   sha=m.group(8), change=m.group(9))
            continue
        m = BENCHROW.match(line)
        if m:
            out[m.group(1)] = dict(old=m.group(1), kind='bench: ' + m.group(2), bytes=int(m.group(3).replace(',', '')),
                                   share=float(m.group(4)), run=int(m.group(5)), sha=m.group(6), kept=m.group(7).strip(),
                                   change=m.group(8))
    return out


# Data that PROVENANCE.md says is not gnosh's, beyond what CARRIED.md lists.
THIRD_PARTY_DATA = (
    'internal/text/testdata/',
    'internal/text/LICENSE-UNICODE.txt',
    'internal/markdown/testdata/spec/commonmark-0.31.2.json',
    'internal/markdown/testdata/spec/gfm-0.29.json',
)
# Go files that hold tables generated from someone else's data. They are new
# code for the scan; the tables are named so a reader can tell.
GENERATED = ('internal/text/tables.go', 'internal/markdown/entity.go')


def kind_of(path):
    """What sort of file it is: code, prompt (model-facing text the program
    embeds), testdata, doc or other."""
    base = path.rsplit('/', 1)[-1]
    if path.startswith('internal/prompt/texts/') or path.startswith('internal/app/agentdef/builtin/'):
        return 'prompt'
    if '/testdata/' in path:
        return 'testdata'
    if base.endswith(('.go', '.gno', '.sh', '.py')):
        return 'code'
    if base.endswith('.md'):
        return 'doc'
    return 'other'


def new_tree_files(tree=None):
    """[(path, group, kind)] for every file of the new tree. group is one of
    new, carried, carried-tools, carried-bench, third-party. carried holds
    the files that are byte for byte the earlier gnosh's or differ by the few
    lines CARRIED.md names; carried-tools (internal/gnovm/tools) and
    carried-bench (the bench files CARRIED.md calls carried with changes)
    are the ones whose changes are larger."""
    tree = tree or NEW_TREE
    car = carried_table(tree)
    out = []
    for p in walk_files(tree):
        rel = os.path.relpath(p, tree)
        if rel in car:
            k = car[rel]['kind']
            if k == 'third-party':
                group = 'third-party'
            elif k == 'carried with larger changes':
                group = 'carried-tools'
            elif k == 'bench: carried with changes':
                group = 'carried-bench'
            else:
                group = 'carried'
        elif rel.startswith(THIRD_PARTY_DATA):
            group = 'third-party'
        else:
            group = 'new'
        out.append((rel, group, kind_of(rel)))
    return out
