//go:build ignore

// Tokscan measures how much of a Go file's token stream also stands in a
// corpus of Go files. It is the part of the similarity audit that line
// matching cannot do: code that was re-typed, re-wrapped or renamed shares no
// line with its source and still shares its tokens.
//
// Two streams are made from every file with go/scanner. Comments are dropped,
// and so are the semicolons the scanner adds at line ends, so that the way
// lines are broken does not matter.
//
//   - exact: every string literal becomes one placeholder (its contents are
//     stripped); identifiers, numbers, character literals, keywords and
//     operators stay as written.
//   - abstract: every identifier becomes one placeholder too (predeclared
//     names like nil, string and len included), and every literal becomes
//     its kind (INT, FLOAT, IMAG, CHAR, STRING). Keywords and operators stay.
//     A copy whose names were all changed has the same abstract stream.
//
// A window is K consecutive tokens (-k for exact, -ka for abstract). A window
// of a target file is shared when the same window stands anywhere in the
// corpus. No sampling and no winnowing: every window of every target is
// looked up against every window of every corpus file (64-bit rolling hash,
// and the tokens are compared again before a match is believed).
//
// For each target file and corpus it writes:
//
//	ntok      tokens in the file
//	nwin      shared windows
//	dwin      distinct shared windows (a table of a thousand like rows has a
//	          thousand shared windows and a handful of distinct ones)
//	cov       tokens that lie in at least one shared window
//	chain     longest run of tokens in which every window is shared, whichever
//	          corpus file each window was found in: an upper bound on the next
//	single    longest run of tokens that stands, token for token, in one
//	          corpus file, with that file and both line ranges
//	single_dw distinct windows inside that run: far below single-K+1 means the
//	          run is one short pattern repeated
//	vwin      shared windows that are varied
//	vcov      tokens that lie in at least one varied shared window
//	vrun      longest run of tokens, standing in one corpus file, in which
//	          every window is varied, with that file and both line ranges
//
// A window is varied when it holds at least -div (exact) or -diva (abstract)
// different tokens. With string contents stripped, any list of twenty strings
// is the same forty tokens as any other, and so is any table of like rows; a
// window of ordinary code holds far more different tokens than such a list
// does. The plain counts take every window; the varied ones leave lists and
// tables out, and are the ones that tell code from code.
//
// Where one window stands at more than -rep places of the same target file,
// the code is a repeated pattern; such windows are counted (nwin, cov, chain)
// but are not followed token by token, so single is a lower bound there.
//
// Usage:
//
//	go run tokscan.go -targets targets.tsv -corpus core=core.tsv [-corpus ...] -out tok.tsv -matches matches.tsv
//	go run tokscan.go -strings list.tsv -out strings.tsv
//
// targets.tsv has five tab-separated columns: set, group, kind, path on disk,
// name to show. A corpus list has two: path on disk, name to show. Only names
// ending in .go or .gno are read.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"go/scanner"
	"go/token"
	"os"
	"sort"
	"strconv"
	"strings"
)

type corpusFlag []string

func (c *corpusFlag) String() string     { return strings.Join(*c, ",") }
func (c *corpusFlag) Set(s string) error { *c = append(*c, s); return nil }

// A stream is one file's tokens in one of the two forms, with the line each
// token stands on.
type stream struct {
	tok  []uint32
	line []uint32
}

// A file is a target file with both of its streams.
type file struct {
	set, group, kind, path, shown string
	s                             [2]stream
	varied                        [2][]bool // by window start
}

// A posting is one place a window stands in the targets.
type posting struct {
	f   uint32
	pos uint32
}

// A match is a run of tokens a target shares with one corpus file.
type match struct {
	ts, te uint32 // token range in the target, end exclusive
	src    string // the corpus file
	cl0    uint32 // first and last line in the corpus file
	cl1    uint32
	nsrc   int // how many corpus files hold this same run
}

// result is what one corpus found in one target file, in one form.
type result struct {
	hit   []bool  // by window start
	best  []match // the longest runs
	vbest []match // the longest runs of varied windows
}

var (
	intern   = map[string]uint32{}
	internAb = map[string]uint32{}
)

func id(m map[string]uint32, s string) uint32 {
	if v, ok := m[s]; ok {
		return v
	}
	v := uint32(len(m) + 1)
	m[s] = v
	return v
}

// tokenize reads a Go source file into its exact and abstract streams.
func tokenize(src []byte) [2]stream {
	var out [2]stream
	fset := token.NewFileSet()
	f := fset.AddFile("", fset.Base(), len(src))
	var s scanner.Scanner
	s.Init(f, src, func(token.Position, string) {}, 0)
	for {
		pos, tok, lit := s.Scan()
		if tok == token.EOF {
			break
		}
		var ex, ab string
		switch {
		case tok == token.SEMICOLON:
			if lit != ";" {
				continue // added by the scanner at a line end
			}
			ex, ab = ";", ";"
		case tok == token.IDENT:
			ex, ab = "i:"+lit, "I"
		case tok == token.STRING:
			ex, ab = "S", "STRING"
		case tok == token.INT || tok == token.FLOAT || tok == token.IMAG || tok == token.CHAR:
			ex, ab = "n:"+lit, tok.String()
		case tok == token.ILLEGAL || tok == token.COMMENT:
			continue
		default:
			ex = tok.String()
			ab = ex
		}
		ln := uint32(f.Line(pos))
		out[0].tok = append(out[0].tok, id(intern, ex))
		out[0].line = append(out[0].line, ln)
		out[1].tok = append(out[1].tok, id(internAb, ab))
		out[1].line = append(out[1].line, ln)
	}
	return out
}

const base = 0x9E3779B97F4A7C15

// hashes returns the rolling hash of every window of k tokens.
func hashes(t []uint32, k int) []uint64 {
	if len(t) < k {
		return nil
	}
	out := make([]uint64, len(t)-k+1)
	var pow uint64 = 1
	for i := 0; i < k-1; i++ {
		pow *= base
	}
	var h uint64
	for i := 0; i < k; i++ {
		h = h*base + uint64(t[i]) + 1
	}
	out[0] = h
	for i := 1; i < len(out); i++ {
		h -= (uint64(t[i-1]) + 1) * pow
		h = h*base + uint64(t[i+k-1]) + 1
		out[i] = h
	}
	return out
}

// variedWindows says, for every window of k tokens, whether it holds at least
// min different tokens.
func variedWindows(t []uint32, k, min int) []bool {
	if len(t) < k {
		return nil
	}
	out := make([]bool, len(t)-k+1)
	count := map[uint32]int{}
	for i, x := range t {
		count[x]++
		if i >= k {
			old := t[i-k]
			if count[old]--; count[old] == 0 {
				delete(count, old)
			}
		}
		if i >= k-1 {
			out[i-k+1] = len(count) >= min
		}
	}
	return out
}

// isGo says whether a file is Go or Gno source, by the name it is shown
// under. A corpus file's name may end in @ and the start of its blob id.
func isGo(name string) bool {
	if i := strings.LastIndexByte(name, '@'); i > 0 && !strings.ContainsAny(name[i:], "/.") {
		name = name[:i]
	}
	return strings.HasSuffix(name, ".go") || strings.HasSuffix(name, ".gno")
}

func readList(fn string, cols int) [][]string {
	fh, err := os.Open(fn)
	if err != nil {
		fmt.Fprintln(os.Stderr, "tokscan:", err)
		os.Exit(1)
	}
	defer fh.Close()
	var out [][]string
	sc := bufio.NewScanner(fh)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		p := strings.Split(sc.Text(), "\t")
		if len(p) < cols {
			continue
		}
		out = append(out, p)
	}
	return out
}

func main() {
	var corpora corpusFlag
	targets := flag.String("targets", "", "list of target files")
	flag.Var(&corpora, "corpus", "name=list of corpus files (may be given more than once)")
	k := flag.Int("k", 40, "window, exact tokens")
	ka := flag.Int("ka", 60, "window, abstract tokens")
	rep := flag.Int("rep", 8, "a window standing at more places of one target file than this is a repeated pattern")
	outFn := flag.String("out", "", "per-file results")
	matchFn := flag.String("matches", "", "the longest runs of each file, with their sources")
	top := flag.Int("top", 6, "runs kept per file, corpus and form")
	div := flag.Int("div", 12, "a window of exact tokens is varied when it holds this many different tokens")
	diva := flag.Int("diva", 9, "a window of abstract tokens is varied when it holds this many different tokens")
	strs := flag.String("strings", "", "instead: list the string literals of the files in this list")
	minStr := flag.Int("minstr", 12, "shortest string listed, in bytes")
	uniq := flag.Bool("uniq", false, "with -strings: list a string once, where it is first seen")
	flag.Parse()

	if *strs != "" {
		listStrings(*strs, *outFn, *minStr, *uniq)
		return
	}
	ks := [2]int{*k, *ka}
	divs := [2]int{*div, *diva}
	forms := [2]string{"exact", "abstract"}

	var files []*file
	for _, p := range readList(*targets, 5) {
		if !isGo(p[4]) {
			continue
		}
		src, err := os.ReadFile(p[3])
		if err != nil {
			fmt.Fprintln(os.Stderr, "tokscan:", err)
			continue
		}
		files = append(files, &file{set: p[0], group: p[1], kind: p[2], path: p[3], shown: p[4], s: tokenize(src)})
	}
	// Where every window of every target stands.
	var index [2]map[uint64][]posting
	var hs [2][][]uint64
	for m := 0; m < 2; m++ {
		index[m] = map[uint64][]posting{}
		hs[m] = make([][]uint64, len(files))
		for fi, f := range files {
			h := hashes(f.s[m].tok, ks[m])
			hs[m][fi] = h
			f.varied[m] = variedWindows(f.s[m].tok, ks[m], divs[m])
			for pos, x := range h {
				index[m][x] = append(index[m][x], posting{uint32(fi), uint32(pos)})
			}
		}
	}
	fmt.Fprintf(os.Stderr, "targets: %d files; windows indexed: %d exact, %d abstract\n", len(files), len(index[0]), len(index[1]))

	out := bufio.NewWriterSize(mustCreate(*outFn), 1<<20)
	defer out.Flush()
	fmt.Fprintln(out, "set\tgroup\tkind\tpath\tcorpus\tform\tntok\tnwin\tdwin\tcov\tchain\tsingle\tsingle_dw\ttarget_lines\tsource\tsource_lines\tsources\tvwin\tvcov\tvrun\tvrun_target_lines\tvrun_source\tvrun_source_lines")
	var mout *bufio.Writer
	if *matchFn != "" {
		mout = bufio.NewWriterSize(mustCreate(*matchFn), 1<<20)
		defer mout.Flush()
		fmt.Fprintln(mout, "set\tpath\tcorpus\tform\twhat\ttokens\tdistinct_windows\ttarget_lines\tsource\tsource_lines\tsources")
	}

	for _, c := range corpora {
		name, list, ok := strings.Cut(c, "=")
		if !ok {
			fmt.Fprintln(os.Stderr, "tokscan: -corpus wants name=list")
			os.Exit(2)
		}
		var res [2][]result
		for m := 0; m < 2; m++ {
			res[m] = make([]result, len(files))
			for fi := range files {
				res[m][fi].hit = make([]bool, len(hs[m][fi]))
			}
		}
		nfiles, ntok := 0, 0
		for _, p := range readList(list, 2) {
			if !isGo(p[1]) {
				continue
			}
			src, err := os.ReadFile(p[0])
			if err != nil {
				continue
			}
			cs := tokenize(src)
			nfiles++
			ntok += len(cs[0].tok)
			for m := 0; m < 2; m++ {
				scanOne(files, index[m], cs[m], ks[m], *rep, *top, p[1], res[m], m)
			}
		}
		fmt.Fprintf(os.Stderr, "corpus %s: %d Go files, %d tokens\n", name, nfiles, ntok)
		for m := 0; m < 2; m++ {
			kk := ks[m]
			for fi, f := range files {
				r := &res[m][fi]
				h := hs[m][fi]
				nwin, cov, chain := 0, 0, 0
				vwin, vcov := 0, 0
				distinct := map[uint64]struct{}{}
				covered, vcovered := -1, -1 // last covered token
				run := 0
				for w, hit := range r.hit {
					if !hit {
						run = 0
						continue
					}
					nwin++
					distinct[h[w]] = struct{}{}
					run++
					if run+kk-1 > chain {
						chain = run + kk - 1
					}
					lo := w
					if covered+1 > lo {
						lo = covered + 1
					}
					if w+kk-1 >= lo {
						cov += w + kk - 1 - lo + 1
						covered = w + kk - 1
					}
					if f.varied[m][w] {
						vwin++
						lo = w
						if vcovered+1 > lo {
							lo = vcovered + 1
						}
						if w+kk-1 >= lo {
							vcov += w + kk - 1 - lo + 1
							vcovered = w + kk - 1
						}
					}
				}
				sort.SliceStable(r.best, func(a, b int) bool {
					return r.best[a].te-r.best[a].ts > r.best[b].te-r.best[b].ts
				})
				single, sdw := 0, 0
				tl, sl, src, nsrc := "", "", "", 0
				ln := f.s[m].line
				for i, b := range r.best {
					dw := map[uint64]struct{}{}
					for w := b.ts; w+uint32(kk) <= b.te; w++ {
						dw[h[w]] = struct{}{}
					}
					t := fmt.Sprintf("%d-%d", ln[b.ts], ln[b.te-1])
					s := fmt.Sprintf("%d-%d", b.cl0, b.cl1)
					if i == 0 {
						single, sdw, tl, sl, src, nsrc = int(b.te-b.ts), len(dw), t, s, b.src, b.nsrc
					}
					if mout != nil {
						fmt.Fprintf(mout, "%s\t%s\t%s\t%s\trun\t%d\t%d\t%s\t%s\t%s\t%d\n", f.set, f.shown, name, forms[m], b.te-b.ts, len(dw), t, b.src, s, b.nsrc)
					}
				}
				sort.SliceStable(r.vbest, func(a, b int) bool {
					return r.vbest[a].te-r.vbest[a].ts > r.vbest[b].te-r.vbest[b].ts
				})
				vrun, vtl, vsl, vsrc := 0, "", "", ""
				for i, b := range r.vbest {
					dw := map[uint64]struct{}{}
					for w := b.ts; w+uint32(kk) <= b.te; w++ {
						dw[h[w]] = struct{}{}
					}
					t := fmt.Sprintf("%d-%d", ln[b.ts], ln[b.te-1])
					s := fmt.Sprintf("%d-%d", b.cl0, b.cl1)
					if i == 0 {
						vrun, vtl, vsl, vsrc = int(b.te-b.ts), t, s, b.src
					}
					if mout != nil {
						fmt.Fprintf(mout, "%s\t%s\t%s\t%s\tvaried\t%d\t%d\t%s\t%s\t%s\t%d\n", f.set, f.shown, name, forms[m], b.te-b.ts, len(dw), t, b.src, s, b.nsrc)
					}
				}
				fmt.Fprintf(out, "%s\t%s\t%s\t%s\t%s\t%s\t%d\t%d\t%d\t%d\t%d\t%d\t%d\t%s\t%s\t%s\t%d\t%d\t%d\t%d\t%s\t%s\t%s\n",
					f.set, f.group, f.kind, f.shown, name, forms[m], len(f.s[m].tok), nwin, len(distinct), cov, chain, single, sdw, tl, src, sl, nsrc,
					vwin, vcov, vrun, vtl, vsrc, vsl)
			}
		}
	}
}

func mustCreate(fn string) *os.File {
	if fn == "" {
		return os.Stdout
	}
	f, err := os.Create(fn)
	if err != nil {
		fmt.Fprintln(os.Stderr, "tokscan:", err)
		os.Exit(1)
	}
	return f
}

// scanOne runs one corpus file's stream past the index of target windows.
func scanOne(files []*file, index map[uint64][]posting, cs stream, k, rep, top int, shown string, res []result, m int) {
	ch := hashes(cs.tok, k)
	if ch == nil {
		return
	}
	// For each target file and diagonal (target position minus corpus
	// position), where the run last followed on it ends: a later window on
	// the same diagonal inside that run is the same run again.
	done := map[uint64]uint32{}
	for cpos, x := range ch {
		ps := index[x]
		if ps == nil {
			continue
		}
		for i := 0; i < len(ps); {
			j := i
			for j < len(ps) && ps[j].f == ps[i].f {
				j++
			}
			fi := ps[i].f
			tt := files[fi].s[m].tok
			r := &res[fi]
			if j-i > rep {
				// A repeated pattern: count the windows, do not follow them.
				for _, p := range ps[i:j] {
					if same(tt[p.pos:int(p.pos)+k], cs.tok[cpos:cpos+k]) {
						r.hit[p.pos] = true
					}
				}
				i = j
				continue
			}
			for _, p := range ps[i:j] {
				tpos := int(p.pos)
				key := uint64(fi)<<32 | uint64(uint32(tpos-cpos+(1<<30)))
				if end, ok := done[key]; ok && uint32(tpos) < end {
					continue
				}
				if !same(tt[tpos:tpos+k], cs.tok[cpos:cpos+k]) {
					continue
				}
				ts, c0 := tpos, cpos
				for ts > 0 && c0 > 0 && tt[ts-1] == cs.tok[c0-1] {
					ts--
					c0--
				}
				te, c1 := tpos+k, cpos+k
				for te < len(tt) && c1 < len(cs.tok) && tt[te] == cs.tok[c1] {
					te++
					c1++
				}
				done[key] = uint32(te)
				varied := files[fi].varied[m]
				run, vn, va := 0, 0, 0 // the longest stretch of varied windows in the run
				for w := ts; w+k <= te; w++ {
					r.hit[w] = true
					if !varied[w] {
						run = 0
						continue
					}
					if run++; run > vn {
						vn, va = run, w-run+1
					}
				}
				r.best = keep(r.best, match{ts: uint32(ts), te: uint32(te), src: shown, cl0: cs.line[c0], cl1: cs.line[c1-1], nsrc: 1}, top)
				if vn > 0 {
					vb := va + vn - 1 + k
					r.vbest = keep(r.vbest, match{ts: uint32(va), te: uint32(vb), src: shown, cl0: cs.line[c0+va-ts], cl1: cs.line[c0+vb-ts-1], nsrc: 1}, top)
				}
			}
			i = j
		}
	}
}

func same(a, b []uint32) bool {
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// keep holds on to the longest runs of a file. A run over the same tokens of
// the target as one already kept is the same run found in another corpus
// file: it is counted, not kept twice.
func keep(best []match, x match, top int) []match {
	for i := range best {
		b := &best[i]
		if b.ts == x.ts && b.te == x.te {
			b.nsrc++
			return best
		}
	}
	if len(best) < top {
		return append(best, x)
	}
	short := 0
	for i := range best {
		if best[i].te-best[i].ts < best[short].te-best[short].ts {
			short = i
		}
	}
	if x.te-x.ts > best[short].te-best[short].ts {
		best[short] = x
	}
	return best
}

// listStrings writes every string literal of the listed Go files: name, line,
// and the string, quoted. Literals joined with + are written as one.
func listStrings(list, outFn string, min int, uniq bool) {
	out := bufio.NewWriterSize(mustCreate(outFn), 1<<20)
	defer out.Flush()
	seen := map[string]struct{}{}
	for _, p := range readList(list, 2) {
		path, shown := p[0], p[1]
		if len(p) >= 5 {
			path, shown = p[3], p[0]+"\t"+p[4]
		}
		if !isGo(shown) {
			continue
		}
		src, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		fset := token.NewFileSet()
		f := fset.AddFile("", fset.Base(), len(src))
		var s scanner.Scanner
		s.Init(f, src, func(token.Position, string) {}, 0)
		var cur strings.Builder
		line, open, plus := 0, false, false
		flush := func() {
			if open && cur.Len() >= min {
				v := cur.String()
				if _, dup := seen[v]; !dup || !uniq {
					fmt.Fprintf(out, "%s\t%d\t%s\n", shown, line, strconv.Quote(v))
				}
				if uniq {
					seen[v] = struct{}{}
				}
			}
			cur.Reset()
			open, plus = false, false
		}
		for {
			pos, tok, lit := s.Scan()
			if tok == token.EOF {
				break
			}
			switch {
			case tok == token.STRING:
				v, err := strconv.Unquote(lit)
				if err != nil {
					v = lit
				}
				if open && plus {
					cur.WriteString(v)
					plus = false
				} else {
					flush()
					cur.WriteString(v)
					line, open = f.Line(pos), true
				}
			case tok == token.ADD && open && !plus:
				plus = true
			case tok == token.SEMICOLON && lit != ";":
				// a line end between the parts of a joined string
			default:
				flush()
			}
		}
		flush()
	}
}
