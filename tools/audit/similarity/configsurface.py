#!/usr/bin/env python3
"""configsurface.py - the names of the configuration, compared with Crush's.

gnosh reads one JSON file, gnosh.json. Crush reads a JSON file (crush.json,
which schema.json describes) and a file of shell-like lines (crushrc), and
takes its providers' and models' fields from Charm's catwalk library. A
config surface is names, not code, so the line and token measures say little
about it. This compares the names themselves.

  configsurface.py names   every name of gnosh's configuration (the keys of
                           gnosh.json, the option names, the provider types,
                           the hook events and fields, the permission modes,
                           the environment variables, and every other
                           word-like string of internal/config that is not a
                           test's) against every name of Crush's
                           configuration, in every version of its files:
                             json     a json, yaml, toml or mapstructure
                                      struct tag of Crush's internal/config
                             schema   a property of schema.json, or one of
                                      its enum values
                             rc       a flag or word of the crushrc language
                                      (a string of internal/shellconfig)
                             catwalk  a struct tag or string constant of
                                      catwalk's package of provider and
                                      model types
                             discover a string of internal/discover (the
                                      local model servers Crush looks for)
                             env      an environment-variable name that
                                      stands as a string in Crush's Go code
                             string   any other word-like string of Crush's
                                      internal/config
                           Two names are the same when they are equal once
                           letter case, dashes, underscores, dots and a
                           leading -- or $ are set aside (base_url,
                           --base-url and baseUrl are one name), and once
                           the brand names are folded (GNOSH_X and CRUSH_X
                           are one name), which is how the line scan folds
                           them. For each name of gnosh's that Crush has
                           too, it says who else has it: Claude Code's
                           documented interface (the name stands in the
                           tree's spec/cc), the model providers' Go SDKs,
                           and unrelated Go code (the generic-Go reference
                           and the control programs).
                           -> config-surface.tsv (one row per name of
                           gnosh's), config-near.tsv (names that are not the
                           same and share most of their words)
  configsurface.py flags   the commands and flags of gnosh's command line
                           (internal/cli/table.go) against the cobra commands
                           and flags of Crush's internal/cmd, with whether
                           Claude Code's documented command line (spec/cc)
                           has the flag. -> config-flags.tsv
  configsurface.py idents  the names internal/config declares (types,
                           functions, methods, constants, variables, struct
                           fields) that a file of Crush's config packages
                           (internal/config, internal/shellconfig,
                           internal/env, internal/discover) also declares,
                           with whether unrelated Go code declares the name
                           as well. Needs the idents tool and decl-ref.tsv.
                           -> config-idents.tsv
  configsurface.py brand   names in strings of the tree's Go code (a string
                           with no space in it) that hold the name gnosh and
                           that stand in Crush's code with the name crush in
                           its place (gnosh.json and crush.json), and the
                           same for file names. Needs strings-ext-all.tsv.
                           -> config-brand.tsv

The outputs hold names, paths and counts only.
"""
import collections
import json
import os
import re
import subprocess
import sys

import common as X
from common import T

CFG_DIRS = ('internal/config/', 'internal/shellconfig/', 'internal/env/', 'internal/discover/')
WORDISH = re.compile(r'^(?:--?|\$|\.)?[A-Za-z][A-Za-z0-9]*(?:[-_./][A-Za-z0-9]+)*$')
ENVNAME = re.compile(r'^[A-Z][A-Z0-9]*(?:_[A-Z0-9]+)+$')
GOSTR = re.compile(r'"((?:[^"\\]|\\.)*)"')
TAG = re.compile(r'\b(json|yaml|toml|mapstructure|env|flag):"([^"]*)"')
RAW = re.compile(r'`([^`]*)`')
TIERS = ('base', 'up', 'now', 'hist', 'nowhist')
TIER_WORDS = {'base': 'fork point', 'up': 'upstream', 'now': 'upstream since', 'hist': 'history only', 'nowhist': 'a branch, since'}


def w(name):
    return os.path.join(X.WORK, name)


def fold(s):
    """A name with its spelling set aside: case, separators, a leading -- or
    $, and the brand."""
    s = T.BRAND.sub('@', s)
    s = re.sub(r'^(?:--?|\$)', '', s)
    f = re.sub(r'[-_.\s/]', '', s).lower()
    if f == '@':            # nothing but the brand: keep the punctuation, so
        return s.lower()    # that .gnosh meets .crush and gnosh meets nothing new
    return f


def words_of(s):
    """The words of a name: split at separators and at camelCase humps."""
    s = T.BRAND.sub('@', re.sub(r'^(?:--?|\$)', '', s))
    out = []
    for part in re.split(r'[-_.\s/]+', s):
        out += re.findall(r'[A-Z]+(?![a-z])|[A-Z]?[a-z]+|\d+|@', part)
    return [x.lower() for x in out if x]


# ---- gnosh's side -----------------------------------------------------------

FROM = {'fromClaudeCode': 'Claude Code', 'fromAPIs': 'provider APIs', 'fromPlain': 'plain words'}
QS = r'"((?:[^"\\]|\\.)*)"'


def from_words(expr):
    """The builder's own account of where a name comes from, as table.go and
    options.go write it: fromPlain + ", after the --data-dir flag"."""
    expr = expr.strip()
    if expr in ('""', ''):
        return ''
    m = re.match(r'^(from\w+)(?:\s*\+\s*' + QS + r')?$', expr)
    if not m:
        return expr
    return FROM.get(m.group(1), m.group(1)) + (m.group(2) or '').replace('\\"', '"')


def blocks(lines):
    """Yield (name, first line number, lines) for each top-level
    `var NAME = ...{` block of a Go file, to its closing brace."""
    i = 0
    while i < len(lines):
        m = re.match(r'^var (\w+) = .*\{\s*$', lines[i])
        if m:
            j = i + 1
            while j < len(lines) and not lines[j].startswith('}'):
                j += 1
            yield m.group(1), i + 1, lines[i + 1:j]
            i = j
        i += 1


def gnosh_names(tree=None):
    """Every name of gnosh's configuration, from the tables the reader works
    from (internal/config/table.go, options.go, env.go):
    [dict(name, shown, where, from_, file, line)]."""
    tree = tree or X.NEW_TREE
    out = []

    def add(name, shown, where, from_, file, line):
        out.append(dict(name=name, shown=shown, where=where, from_=from_, file=file, line=line))

    keydefs = {'topKeys': ('', 'top-level key'), 'providerFields': ('providers.<id>.', 'provider field'),
               'modelFields': ('models.<provider>/<model id>.', 'model field'),
               'slotFields': ('model. / smallModel.', 'field of "model" and "smallModel"'),
               'hostFields': ('hosts.<name>.', 'host field'), 'permissionKeys': ('permissions.', 'permissions key')}
    fn = 'internal/config/table.go'
    lines = (X.read_text(os.path.join(tree, fn)) or [])
    for name, first, body in blocks(lines):
        if name in keydefs:
            prefix, where = keydefs[name]
            for k, l in enumerate(body):
                m = re.match(r'^\t\{' + QS + r', (?:""|\w+), .*, ((?:from\w+|"")(?:\s*\+\s*' + QS + r')?)\},$', l)
                if m:
                    add(m.group(1), prefix + m.group(1), where, from_words(m.group(2)), fn, first + 1 + k)
        elif name == 'providerTypes':
            for k, l in enumerate(body):
                m = re.match(r'^\t\{name: ' + QS + r', wire: ' + QS, l)
                if m:
                    add(m.group(1), 'provider type ' + m.group(1), 'provider type', "plain words: the server's own name", fn, first + 1 + k)
        elif name == 'hookEvents':
            for k, l in enumerate(body):
                m = re.match(r'^\t\{' + QS + r', "[^"]*", (?:true|false)\},$', l)
                if m:
                    add(m.group(1), 'hook event ' + m.group(1), 'hook event', 'Claude Code', fn, first + 1 + k)
        elif name == 'hookFields':
            for k, l in enumerate(body):
                m = re.match(r'^\t' + QS + r':\s', l)
                if m:
                    add(m.group(1), 'hooks: handler field ' + m.group(1), 'hook handler field', 'Claude Code', fn, first + 1 + k)
        elif name in ('claudeCodeKeys', 'claudeCodePermissionKeys', 'ownEnvPrefixes'):
            for k, l in enumerate(body):
                for s in GOSTR.findall(l):
                    if name == 'ownEnvPrefixes':
                        add(s, '$' + s + '*', 'environment prefix kept for tests', '', fn, first + 1 + k)
                    else:
                        add(s, s + ' (refused)', 'a Claude Code key gnosh names only to refuse it', 'Claude Code', fn, first + 1 + k)
        elif name == 'ownEnv':
            for k, l in enumerate(body):
                m = re.match(r'^\t\{' + QS + r', "(reads|sets|tests)", ' + QS + r', (.*)\},$', l)
                if m:
                    use = {'reads': 'environment variable gnosh reads', 'sets': 'environment variable gnosh sets for hooks and commands',
                           'tests': 'environment switch of gnosh\'s own tests'}[m.group(2)]
                    add(m.group(1), '$' + m.group(1), use, from_words(m.group(4)), fn, first + 1 + k)
    # one-line lists
    for k, l in enumerate(lines):
        m = re.match(r'^var (permissionModes|claudeCodePermissionKeys|ownEnvPrefixes) = \[\]string\{(.*)\}$', l)
        if m:
            for s in GOSTR.findall(m.group(2)):
                if m.group(1) == 'permissionModes':
                    add(s, 'permission mode ' + s, 'permission mode', 'Claude Code', fn, k + 1)
                elif m.group(1) == 'ownEnvPrefixes':
                    add(s, '$' + s + '*', 'environment prefix kept for tests', '', fn, k + 1)
                else:
                    add(s, s + ' (refused)', 'a Claude Code key gnosh names only to refuse it', 'Claude Code', fn, k + 1)
    fn = 'internal/config/options.go'
    text = '\n'.join(X.read_text(os.path.join(tree, fn)) or [])
    for m in re.finditer(r'\{key: ' + QS + r', kind: \w+, def: ' + QS + r',\s+what: ' + QS + r', from: ((?:from\w+)(?:\s*\+\s*' + QS + r')?),', text):
        add(m.group(1), 'options.' + m.group(1), 'option', from_words(m.group(4)), fn, text.count('\n', 0, m.start()) + 1)
    fn = 'internal/config/env.go'
    lines = (X.read_text(os.path.join(tree, fn)) or [])
    for name, first, body in blocks(lines):
        if name == 'limitVars':
            for k, l in enumerate(body):
                m = re.match(r'^\t\{' + QS + r', ' + QS + r', (.*)\},$', l)
                if m:
                    add(m.group(1), '$' + m.group(1), 'environment variable gnosh reads', from_words(m.group(3)), fn, first + 1 + k)
    return out


def literals(path):
    """(line number, string) for the double-quoted strings of a Go file and
    (line number, key, value) for its struct tags. Comment lines are left
    out. A regular expression, not a parser: good enough for names."""
    strs, tags = [], []
    imports = False
    for i, l in enumerate(X.read_text(path) or []):
        if l.lstrip().startswith('//'):
            continue
        if imports:                      # the paths of an import block are not names
            imports = not l.startswith(')')
            continue
        if l.startswith('import ('):
            imports = True
            continue
        if l.startswith('import '):
            continue
        for raw in RAW.findall(l):
            for key, val in TAG.findall(raw):
                val = val.split(',')[0]
                if val and val != '-':
                    tags.append((i + 1, key, val))
        for s in GOSTR.findall(RAW.sub('``', l)):
            strs.append((i + 1, s))
    return strs, tags


def gnosh_other_strings(tree, known):
    """Word-like strings of internal/config's files that are not tests and
    that the tables above did not give: file names, kinds, layer names."""
    out = []
    d = os.path.join(tree, 'internal/config')
    for p in X.walk_files(d, exts=('.go',)):
        if p.endswith('_test.go'):
            continue
        rel = os.path.relpath(p, tree)
        seen = set()
        strs, tags = literals(p)
        for line, s in strs + [(ln, v) for ln, _, v in tags]:
            if s in known or s in seen or len(s) < 2 or not WORDISH.match(s):
                continue
            seen.add(s)
            out.append(dict(name=s, shown=s, where='another string of internal/config', from_='', file=rel, line=line))
    return out


# ---- Crush's side -----------------------------------------------------------

def crush_files():
    """[(disk path, tier, path in Crush's repository)] for every version of
    every text file of Crush, in the corpora crush, hist and since."""
    rows = []
    for fn in ('crush-ext.tsv', 'since.tsv'):
        if not os.path.exists(w('lists/' + fn)):
            continue
        for l in open(w('lists/' + fn)):
            disk, shown = l.rstrip('\n').split('\t')
            tier, rest = shown.split(':', 1)
            rows.append((disk, tier, rest.rsplit('@', 1)[0]))
    return rows


def catwalk_files():
    rows = []
    for l in open(w('lists/libs.tsv')):
        disk, shown = l.rstrip('\n').split('\t')
        if '/catwalk@' in shown and shown.endswith('.go') and not shown.endswith('_test.go'):
            rows.append((disk, 'catwalk', shown.split(':', 1)[1]))
    return rows


def schema_names(obj, out, ctx=''):
    """Property names and enum values of a JSON schema, recursively."""
    if isinstance(obj, dict):
        props = obj.get('properties')
        if isinstance(props, dict):
            for k, v in props.items():
                out.append(('schema', k))
                schema_names(v, out, k)
        en = obj.get('enum')
        if isinstance(en, list):
            for v in en:
                if isinstance(v, str) and v:
                    out.append(('schema', v))
        for k, v in obj.items():
            if k not in ('properties', 'enum'):
                schema_names(v, out, ctx)
    elif isinstance(obj, list):
        for v in obj:
            schema_names(v, out, ctx)


class Names:
    """Crush's configuration names: name -> kinds, tiers, an example."""

    def __init__(self):
        self.by = collections.OrderedDict()
        self.folded = collections.defaultdict(list)

    def add(self, name, kind, tier, where):
        e = self.by.get(name)
        if e is None:
            e = self.by[name] = dict(kinds=set(), tiers=set(), where=where)
            self.folded[fold(name)].append(name)
        e['kinds'].add(kind)
        e['tiers'].add(tier)


def crush_names():
    N = Names()
    nfiles = collections.Counter()
    for disk, tier, path in crush_files():
        if path == 'schema.json':
            nfiles['schema.json'] += 1
            try:
                found = []
                schema_names(json.load(open(disk, encoding='utf-8', errors='replace')), found)
            except (ValueError, OSError):
                continue
            for kind, name in found:
                N.add(name, kind, tier, '%s:%s' % (tier, path))
            continue
        if not path.endswith('.go'):
            continue
        strs, tags = literals(disk)
        # environment-variable names anywhere in Crush's Go code
        for line, s in strs:
            if ENVNAME.match(s):
                N.add(s, 'env', tier, '%s:%s:%d' % (tier, path, line))
        if not path.startswith(CFG_DIRS) or path.endswith('_test.go'):
            continue
        nfiles[path.split('/')[1]] += 1
        pkg = path.split('/')[1]
        for line, key, val in tags:
            N.add(val, 'json' if pkg == 'config' else pkg, tier, '%s:%s:%d' % (tier, path, line))
        for line, s in strs:
            if len(s) < 2 or not WORDISH.match(s) or ENVNAME.match(s):
                continue
            kind = {'config': 'string', 'shellconfig': 'rc', 'discover': 'discover', 'env': 'string'}[pkg]
            N.add(s, kind, tier, '%s:%s:%d' % (tier, path, line))
    for disk, tier, path in catwalk_files():
        nfiles['catwalk'] += 1
        strs, tags = literals(disk)
        for line, key, val in tags:
            N.add(val, 'catwalk', 'libs', 'libs:%s:%d' % (path, line))
        for line, s in strs:
            if len(s) >= 2 and WORDISH.match(s):
                N.add(s, 'catwalk', 'libs', 'libs:%s:%d' % (path, line))
    return N, nfiles


def earlier_gnosh_names():
    """The same names from the earlier gnosh's copies of those packages, so
    that a name the earlier gnosh added to Crush's can be told from one
    Crush has."""
    out = set()
    root = w('old')
    for d in CFG_DIRS:
        for p in X.walk_files(os.path.join(root, d), exts=('.go',)) if os.path.isdir(os.path.join(root, d)) else []:
            if p.endswith('_test.go'):
                continue
            strs, tags = literals(p)
            out.update(v for _, _, v in tags)
            out.update(s for _, s in strs if len(s) >= 2 and WORDISH.match(s))
    p = os.path.join(root, 'schema.json')
    if os.path.exists(p):
        found = []
        try:
            schema_names(json.load(open(p, encoding='utf-8', errors='replace')), found)
        except ValueError:
            pass
        out.update(n for _, n in found)
    return out


# ---- who else has a name ----------------------------------------------------

SDKS = ('github.com/anthropics/anthropic-sdk-go@', 'github.com/openai/openai-go', 'github.com/charmbracelet/openai-go@',
        'google.golang.org/genai@', 'github.com/aws/aws-sdk-go-v2/service/bedrockruntime@', 'github.com/ollama/ollama@')


def sdk_names():
    """Folded names the model providers' Go SDKs declare (struct tags are the
    wire names; identifiers are the SDKs' option names), from the lists of
    the dropped dependencies, the Charm libraries (its fork of openai-go)
    and the control programs (ollama's API package). fold -> example."""
    out = {}
    lists = []
    for fn in ('deps.tsv', 'libs.tsv'):
        if os.path.exists(w('lists/' + fn)):
            for l in open(w('lists/' + fn)):
                disk, shown = l.rstrip('\n').split('\t')
                if shown.endswith('.go') and not shown.endswith('_test.go') and any(s in shown for s in SDKS):
                    lists.append((disk, shown))
    for l in open(w('lists/targets.tsv')) if os.path.exists(w('lists/targets.tsv')) else []:
        p = l.rstrip('\n').split('\t')
        if p[0] == 'ctl:ollama' and p[4].startswith('api/') and not p[4].endswith('_test.go'):
            lists.append((p[3], 'ctl:ollama/' + p[4]))
    mods = collections.Counter()
    for disk, shown in lists:
        mod = shown.split(':', 1)[1].split('@')[0] if '@' in shown else shown.split('/')[0]
        mods[mod] += 1
        strs, tags = literals(disk)
        for _, key, val in tags:
            if key == 'json':
                out.setdefault(fold(val), 'wire name %s in %s' % (val, mod.rsplit('/', 1)[-1]))
        for l in X.read_text(disk) or []:
            m = re.match(r'^func (?:\([^)]*\) )?(?:With)?([A-Z]\w*)\(', l) or re.match(r'^\t([A-Z]\w*)\s+[\w.\[\]*]+', l)
            if m:
                out.setdefault(fold(m.group(1)), 'identifier %s in %s' % (m.group(1), mod.rsplit('/', 1)[-1]))
    return out, mods


def ref_names():
    """Folded names unrelated Go code declares: the generic-Go reference and
    the control programs (decl-ref.tsv). fold -> (kind, where)."""
    out = {}
    fn = w('decl-ref.tsv')
    if not os.path.exists(fn):
        return out
    for l in open(fn, encoding='utf-8', errors='replace'):
        p = l.rstrip('\n').split('\t')
        if len(p) == 4:
            out.setdefault(fold(p[1]), (p[0], p[3]))
    return out


def spec_cc(tree):
    """The text of the tree's spec/cc: Claude Code's documented interface, as
    the spec side wrote it down."""
    out = []
    d = os.path.join(tree, 'spec/cc')
    for p in X.walk_files(d) if os.path.isdir(d) else []:
        lines = X.read_text(p)
        if lines is not None:
            out.append((os.path.relpath(p, tree), '\n'.join(lines)))
    return out


def in_cc(name, cc):
    """The files of spec/cc in which a name stands as a whole word. A flag
    is not found inside a longer flag (--session in --session-id)."""
    after = r'(?![A-Za-z0-9_-])' if name.startswith('-') else r'(?![A-Za-z0-9_])'
    rx = re.compile(r'(?<![A-Za-z0-9_-])' + re.escape(name) + after if name.startswith('-') else r'(?<![A-Za-z0-9_])' + re.escape(name) + after)
    return [rel for rel, text in cc if rx.search(text)]


# ---- names ------------------------------------------------------------------

def check_table(G):
    """table.tsv is config.Table() as the program prints it (tabledump.go,
    run by run.sh). Every entry of it must be a name read from the source
    text above, and every key, type, event and variable read from the
    tables must be an entry of it."""
    fn = w('table.tsv')
    if not os.path.exists(fn) or not os.path.getsize(fn):
        print('config.Table() was not printed (table.tsv): the tables were read from the source text alone')
        return
    theirs = set()
    for l in open(fn, encoding='utf-8'):
        name = l.split('\t')[0]
        for a, b in (('model.', 'model. / smallModel.'), ('smallModel.', 'model. / smallModel.')):
            if name.startswith(a):
                name = b + name[len(a):]
                break
        theirs.add(name)
    kinds = ('top-level key', 'provider field', 'model field', 'field of "model" and "smallModel"', 'host field', 'permissions key',
             'option', 'provider type', 'hook event', 'environment variable gnosh reads')
    mine = {g['shown'] for g in G if g['where'] in kinds}
    print('config.Table() has %d entries (%d distinct names once model. and smallModel. are one); read from the source text: %d; in one and not the other: %s' % (
        sum(1 for _ in open(fn)), len(theirs), len(mine), sorted(theirs ^ mine) or 'none'))


def bare(s):
    return re.sub(r'^(?:--?|\$)', '', s)


def how_same(mine, theirs):
    """How a name of gnosh's and the names of Crush's with the same fold
    differ: not at all, in the brand alone, in letter case alone, or in
    spelling (dashes, underscores, capitals)."""
    if not theirs:
        return ''
    m = bare(mine)
    t = [bare(x) for x in theirs]
    if m in t:
        return 'same'
    if T.BRAND.search(m):
        if any(T.BRAND.sub('@', x) == T.BRAND.sub('@', m) for x in t):
            return 'same but for the brand'
        return 'respelled, and the brand'
    if m.lower() in [x.lower() for x in t]:
        return 'same but for letter case'
    return 'respelled'


def tiers_of(names, N):
    t = set()
    for n in names:
        t |= N.by[n]['tiers']
    order = ['base', 'up', 'now', 'hist', 'nowhist', 'libs']
    return [x for x in order if x in t]


def names():
    tree = X.NEW_TREE
    G = gnosh_names(tree)
    known = {g['name'] for g in G}
    G += gnosh_other_strings(tree, known)
    N, nfiles = crush_names()
    old = earlier_gnosh_names()
    sdk, mods = sdk_names()
    ref = ref_names()
    cc = spec_cc(tree)
    print('gnosh: %d names from the tables, %d other word-like strings of internal/config' % (len(known), len(G) - len([g for g in G if g['name'] in known])))
    check_table(G)
    print('Crush: %d distinct names; files read: %s' % (len(N.by), dict(nfiles)))
    print('provider SDKs read: %s' % dict(mods))
    cols = ['name', 'what_it_is', 'where_in_gnosh', 'builder_says_from', 'match', 'crush_names', 'crush_kinds', 'crush_tiers',
            'crush_example', 'earlier_gnosh_only', 'in_spec_cc', 'in_provider_sdks', 'in_unrelated_go']
    rows = []
    seen = set()
    for g in G:
        key = (g['name'], g['where'])
        if key in seen:
            continue
        seen.add(key)
        f = fold(g['name'])
        theirs = N.folded.get(f, [])
        # an environment name of gnosh's also meets Crush's without the brand
        match = how_same(g['name'], theirs)
        kinds = sorted(set().union(*[N.by[t]['kinds'] for t in theirs])) if theirs else []
        only_old = '' if theirs else ('yes' if any(fold(o) == f for o in old) else '')
        ccf = in_cc(g['name'].lstrip('$'), cc)
        rows.append([g['shown'], g['where'], '%s:%d' % (g['file'], g['line']), g['from_'], match, ' '.join(theirs), ' '.join(kinds),
                     ', '.join(TIER_WORDS.get(t, t) for t in tiers_of(theirs, N)),
                     N.by[theirs[0]]['where'] if theirs else '', only_old,
                     ('%d files' % len(ccf)) if ccf else '', sdk.get(f, ''), ('%s %s' % ref[f]) if f in ref else ''])
    with open(w('config-surface.tsv'), 'w') as fh:
        fh.write('\t'.join(cols) + '\n')
        for r in rows:
            fh.write('\t'.join(str(x) for x in r) + '\n')
    by = collections.Counter()
    for r in rows:
        by[(r[1], r[4] or 'not in Crush')] += 1
    for (where, m), n in sorted(by.items()):
        print('  %-58s %-24s %d' % (where, m, n))
    # near misses: names that are not the same and share most of their words
    stem = {'directory': 'dir', 'directories': 'dir', 'dirs': 'dir', 'compatible': 'compat', 'disabled': 'disable', 'disables': 'disable',
            'enabled': 'enable', 'models': 'model', 'providers': 'provider', 'tokens': 'token', 'tasks': 'task', 'jobs': 'job',
            'files': 'file', 'paths': 'path', 'skills': 'skill', 'tools': 'tool', 'hooks': 'hook', 'levels': 'level', 'headers': 'header',
            'timeouts': 'timeout', 'urls': 'url', 'uri': 'url', 'endpoint': 'url', 'maximum': 'max', 'minimum': 'min', 'config': 'cfg',
            'configuration': 'cfg', 'ctx': 'context', 'auto': 'auto', 'thinking': 'think', 'reasoning': 'think', 'reason': 'think'}

    def wset(s):
        return frozenset(stem.get(x, x) for x in words_of(s))
    cw = collections.defaultdict(list)
    for n, e in N.by.items():
        if e['kinds'] & {'json', 'schema', 'rc', 'catwalk', 'discover', 'shellconfig'}:
            ws = wset(n)
            if ws:
                cw[ws].append(n)
    near = []
    for g in G:
        if g['where'].startswith(('a Claude Code key', 'another string', 'hook event', 'environment')):
            continue
        f = fold(g['name'])
        if N.folded.get(f):
            continue
        gw = wset(g['name'])
        if len(gw) < 2:
            continue
        for ws, ns in cw.items():
            if len(ws) < 2:
                continue
            common = gw & ws
            if ws == gw or (len(common) >= 2 and len(gw ^ ws) <= 1):
                near.append((g['shown'], g['where'], ' '.join(sorted(ns)[:6]), 'same words' if ws == gw else 'one word apart',
                             ', '.join(TIER_WORDS.get(t, t) for t in tiers_of(ns, N))))
    with open(w('config-near.tsv'), 'w') as fh:
        fh.write('name\twhat_it_is\tcrush_names\thow_near\tcrush_tiers\n')
        for r in sorted(set(near)):
            fh.write('\t'.join(r) + '\n')
    print('near misses (not the same name; the same words in another order or form, or one word apart): %d' % len(set(near)))


# ---- the command line -------------------------------------------------------

COBRA = re.compile(r'Flags\(\)\.(\w+)\(\s*(?:&[\w.]+,\s*)?"([a-z][\w-]*)"(?:,\s*"([^"]?)")?')
USE = re.compile(r'\bUse:\s*"([\w-]+)')


def flags():
    """gnosh's commands and flags (internal/cli/table.go) against the ones
    Crush's command line declares (the cobra commands and flags of its
    internal/cmd, in every version), with whether Claude Code's documented
    command line has the flag (spec/cc). -> config-flags.tsv"""
    tree = X.NEW_TREE
    cc = spec_cc(tree)
    theirs_long = collections.defaultdict(lambda: dict(tiers=set(), short=set(), where=''))
    theirs_cmd = collections.defaultdict(lambda: dict(tiers=set(), where=''))
    for disk, tier, path in crush_files():
        if not (path.startswith('internal/cmd/') or path in ('main.go', 'cmd/root.go') or path.startswith('cmd/')) or not path.endswith('.go') or path.endswith('_test.go'):
            continue
        for i, l in enumerate(X.read_text(disk) or []):
            for meth, long_, short in COBRA.findall(l):
                e = theirs_long[long_]
                e['tiers'].add(tier)
                e['where'] = e['where'] or '%s:%s:%d' % (tier, path, i + 1)
                if meth.endswith('P') and short:
                    e['short'].add(short)
            for name in USE.findall(l):
                e = theirs_cmd[name]
                e['tiers'].add(tier)
                e['where'] = e['where'] or '%s:%s:%d' % (tier, path, i + 1)
    order = ['base', 'up', 'now', 'hist', 'nowhist']

    def tw(t):
        return ', '.join(TIER_WORDS[x] for x in order if x in t)
    rows = []
    fn = 'internal/cli/table.go'
    seen = set()
    for i, l in enumerate(X.read_text(os.path.join(tree, fn)) or []):
        m = re.search(r'&flag\{names: ' + QS, l)
        if m:
            idm = re.search(r'\bid: ' + QS, l)
            sp = m.group(1).split()
            longs = [x[2:] for x in sp if x.startswith('--')]
            shorts = [x[1:] for x in sp if not x.startswith('--')]
            for lg in longs:
                e = theirs_long.get(lg)
                if e is None:
                    e2 = [k for k in theirs_long if fold(k) == fold(lg)]
                    e = theirs_long[e2[0]] if e2 else None
                    spelled = e2[0] if e2 else ''
                else:
                    spelled = lg
                key = ('flag', lg, idm.group(1) if idm else '')
                if key in seen:
                    continue
                seen.add(key)
                ccf = in_cc('--' + lg, cc)
                rows.append(('flag', '--' + lg, ' '.join('-' + x for x in shorts), (idm.group(1) if idm else 'a session'), '%s:%d' % (fn, i + 1),
                             ('--' + spelled) if e else '', ' '.join('-' + x for x in sorted(e['short'])) if e else '',
                             ' '.join('-' + x for x in shorts if e and x in e['short']), tw(e['tiers']) if e else '', e['where'] if e else '',
                             ('%d files' % len(ccf)) if ccf else ''))
        m = re.search(r'&command\{name: ' + QS, l)
        if m:
            name = m.group(1)
            e = theirs_cmd.get(name)
            ccf = in_cc(name, [(r, t) for r, t in cc if r.endswith('cli.md')])
            rows.append(('command', name, '', '', '%s:%d' % (fn, i + 1), name if e else '', '', '', tw(e['tiers']) if e else '', e['where'] if e else '',
                         ('%d files' % len(ccf)) if ccf else ''))
    with open(w('config-flags.tsv'), 'w') as fh:
        fh.write('what\tname\tshort\tof\twhere_in_gnosh\tcrush_has\tcrush_short\tsame_short\tcrush_tiers\tcrush_example\tin_spec_cc\n')
        for r in rows:
            fh.write('\t'.join(r) + '\n')
    for what in ('flag', 'command'):
        sel = [r for r in rows if r[0] == what]
        both = [r for r in sel if r[5]]
        print('%ss of gnosh: %d; that Crush\'s command line also has: %d, of which Claude Code\'s documentation has: %d' % (
            what, len(sel), len(both), sum(1 for r in both if r[10])))
        print('   in Crush and not in Claude Code\'s documentation: %s' % ' '.join(r[1] + ('/' + r[7] if r[7] else '') for r in both if not r[10]))
    print('Crush\'s command line: %d flags, %d commands' % (len(theirs_long), len(theirs_cmd)))


# ---- declared Go names ------------------------------------------------------

def decl(list_rows, name):
    """Run the idents tool over a list of files; {name: {kind: (files, first)}}."""
    lst = w('lists/%s.tsv' % name)
    with open(lst, 'w') as f:
        for disk, shown in list_rows:
            f.write('%s\t%s\n' % (disk, shown))
    out = w('decl-%s.tsv' % name)
    subprocess.run([w('idents'), '-decl', lst, '-out', out], check=True)
    res = collections.defaultdict(dict)
    for l in open(out, encoding='utf-8', errors='replace'):
        p = l.rstrip('\n').split('\t')
        if len(p) == 4 and p[2].isdigit():
            res[p[1]][p[0]] = (int(p[2]), p[3])
    return res


def idents():
    tree = X.NEW_TREE
    mine_rows, mine_test = [], []
    for p in X.walk_files(os.path.join(tree, 'internal/config'), exts=('.go',)):
        (mine_test if p.endswith('_test.go') else mine_rows).append((p, os.path.relpath(p, tree)))
    mine = decl(mine_rows, 'cfg-new')
    mine_t = decl(mine_test, 'cfg-new-tests')
    theirs = {}
    for label, tiers in (('fork point or upstream', ('base', 'up')), ('upstream since', ('now', 'nowhist')), ('history only', ('hist',))):
        rows = [(disk, '%s:%s' % (tier, path)) for disk, tier, path in crush_files()
                if tier in tiers and path.startswith(CFG_DIRS) and path.endswith('.go')]
        theirs[label] = decl(rows, 'cfg-crush-' + label.replace(' ', '-'))
    ref = collections.defaultdict(dict)
    for l in open(w('decl-ref.tsv'), encoding='utf-8', errors='replace'):
        p = l.rstrip('\n').split('\t')
        if len(p) == 4 and p[2].isdigit():
            ref[p[1]][p[0]] = (int(p[2]), p[3])
    kinds = ('type', 'func', 'method', 'const', 'var', 'field')
    rows = []
    n_mine = collections.Counter()
    for src, d in (('code', mine), ('tests', mine_t)):
        for name, ks in sorted(d.items()):
            mk = [k for k in kinds if k in ks]
            if not mk:
                continue
            exported = name[:1].isupper()
            n_mine[(src, 'exported' if exported else 'unexported')] += 1
            for label, t in theirs.items():
                tk = [k for k in kinds if k in t.get(name, {})]
                if not tk:
                    continue
                rk = [k for k in kinds if k in ref.get(name, {})]
                rows.append((name, 'exported' if exported else 'unexported', src, ','.join(mk), ks[mk[0]][1], label, ','.join(tk),
                             t[name][tk[0]][1], sum(v[0] for k, v in t[name].items() if k in kinds),
                             'yes' if rk else '', (ref[name][rk[0]][1] if rk else '')))
                break  # the earliest tier that has it
    with open(w('config-idents.tsv'), 'w') as fh:
        fh.write('name\texported\tin_gnosh\tgnosh_kinds\tgnosh_file\tcrush_tier\tcrush_kinds\tcrush_file\tcrush_files_declaring\tunrelated_go_declares_it\twhere\n')
        for r in sorted(rows, key=lambda r: (r[1] != 'unexported', r[2] != 'code', r[9] != '', r[0].lower())):
            fh.write('\t'.join(str(x) for x in r) + '\n')
    print('names internal/config declares: %s' % dict(sorted(n_mine.items())))
    c = collections.Counter((r[1], r[2], r[5], 'unrelated Go has it too' if r[9] else 'not in unrelated Go') for r in rows)
    for k, n in sorted(c.items()):
        print('  %-11s %-6s %-24s %-26s %d' % (k + (n,)))
    for r in sorted(rows, key=lambda r: r[0].lower()):
        if r[1] == 'unexported' and not r[9]:
            print('    %s [%s, %s] <- %s (%s; %d Crush files)' % (r[0], r[3], r[2], r[7], r[6], r[8]))


# ---- the brand in a name ----------------------------------------------------

def brand():
    """Names in strings of the tree's Go code that hold gnosh, klash or crash
    and that stand in Crush's Go code with crush in its place; and the same
    for file names. (crash is an earlier name of the fork and also a word:
    a row for it is the word.)"""
    theirs = collections.defaultdict(set)
    for fn in ('strings-ext-all.tsv', 'strings-since-all.tsv'):
        if not os.path.exists(w(fn)):
            continue
        for l in open(w(fn), encoding='utf-8', errors='replace'):
            p = l.rstrip('\n').split('\t')
            if len(p) < 3:
                continue
            try:
                s = json.loads(p[2])
            except ValueError:
                s = p[2].strip('"')
            if re.search('crush', s, re.I) and len(s) <= 80:
                theirs[T.BRAND.sub('@', s.lower())].add((s, p[0]))
    their_files = collections.defaultdict(set)
    for disk, tier, path in crush_files():
        base = path.rsplit('/', 1)[-1]
        if re.search('crush', base, re.I):
            their_files[T.BRAND.sub('@', base.lower())].add((base, '%s:%s' % (tier, path)))
    rows = []
    seen = set()
    for rel, group, kind in X.new_tree_files():
        base = rel.rsplit('/', 1)[-1]
        if re.search('gnosh|klash|crash', base, re.I):
            k = T.BRAND.sub('@', base.lower())
            if k in their_files and ('file', base) not in seen:
                seen.add(('file', base))
                ex = sorted(their_files[k])[0]
                rows.append(('file name', base, group, rel, ex[0], ex[1], len(their_files[k])))
        if not rel.endswith('.go') or group == 'third-party':
            continue
        strs, tags = literals(os.path.join(X.NEW_TREE, rel))
        for line, s in strs:
            # names only: a string with a space in it is a phrase, and what
            # Crush's code says in phrases is not written into a file here
            if len(s) > 80 or ' ' in s.strip() or not re.search('gnosh|klash|crash', s, re.I):
                continue
            k = T.BRAND.sub('@', s.lower())
            label = group + (' (test)' if rel.endswith('_test.go') else '')
            if k in theirs and (label, s) not in seen:
                seen.add((label, s))
                ex = sorted(theirs[k])[0]
                rows.append(('string', s, label, '%s:%d' % (rel, line), ex[0], ex[1], len(theirs[k])))
    with open(w('config-brand.tsv'), 'w') as fh:
        fh.write('what\tin_gnosh\tgroup\twhere\tin_crush\tcrush_example\tcrush_spellings\n')
        for r in sorted(rows, key=lambda r: (r[0], r[2], r[1].lower())):
            fh.write('\t'.join(str(x) for x in r) + '\n')
    c = collections.Counter((r[0], r[2]) for r in rows)
    print('names of gnosh\'s that Crush has with its own name in place: %s' % dict(sorted(c.items())))


if __name__ == '__main__':
    cmds = dict(names=names, flags=flags, idents=idents, brand=brand)
    for c in sys.argv[1:] or list(cmds):
        cmds[c]()
