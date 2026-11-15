//go:build linux

package permission

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// The oracle's knobs. The ordinary test run judges a few hundred lines;
// the long run is started by hand:
//
//	go test ./internal/permission -run 'TestOracle$' -oracle.lines=300000 -timeout 6h
var (
	oracleLines   = flag.Int("oracle.lines", 0, "how many generated lines the oracle judges (0: a few hundred)")
	oracleSeed    = flag.Int64("oracle.seed", 1, "the seed of the oracle's generators")
	oracleWorkers = flag.Int("oracle.workers", 0, "how many jails the oracle runs at once (0: half the processors)")
	oracleShow    = flag.Int("oracle.show", 300, "how many distinct findings the oracle shrinks and prints")
	oracleOnly    = flag.String("oracle.only", "", "one generator for every line of the long run: list, smuggle, readonly, edits, tricks or soup")
)

// randomSets makes rule sets out of a line's own commands, so that the
// engine says yes to it as often as it can and the shell is asked whether
// that yes was earned.
func (g *lineGen) randomSets(e *Engine, line string) []ruleSet {
	v := e.viewBash(line)
	var names, exacts []string
	for _, u := range v.units {
		ws := u.cmd.words
		if len(ws) == 0 {
			continue
		}
		if raw := rawJoined(ws); !strings.ContainsAny(raw, "\n") && len(raw) < 200 {
			exacts = append(exacts, raw)
		}
		if n, ok := e.known(ws[0]); ok && plainName(ws[0]) {
			names = append(names, n)
		}
	}
	if len(names) == 0 && len(exacts) == 0 {
		return nil
	}
	guards := []string{"Bash(rm *)", "Bash(curl *)", "Bash(git push *)", "Bash(cat *)", "Bash(touch *)", "Bash(make *)",
		"Bash(python *)", "Bash(sh *)", "Bash(tee *)", "Read(./.env)", "Edit(./build/**)", "Read(~/.ssh/**)", "Bash(* --force)",
		"Bash(git *)", "Bash(echo x)", "Read(.env)", "Edit(*.txt)", "Read(//etc/**)", "Bash(ls)"}
	var out []ruleSet
	for k := 0; k < 2; k++ {
		var r rules
		for n := 1 + g.r.Intn(3); n > 0; n-- {
			switch {
			case len(names) > 0 && g.chance(0.6):
				r.allow = append(r.allow, "Bash("+g.one(names)+" *)")
			case len(exacts) > 0:
				r.allow = append(r.allow, "Bash("+strings.ReplaceAll(g.one(exacts), "*", "\\*")+")")
			}
		}
		for g.chance(0.4) {
			r.deny = append(r.deny, g.one(guards))
		}
		if g.chance(0.15) {
			r.ask = append(r.ask, g.one(guards))
		}
		mode := g.one([]string{"default", "default", "acceptEdits", "dontAsk", "bypassPermissions"})
		out = append(out, ruleSet{
			name: fmt.Sprintf("%s; allow %q; deny %q; ask %q", mode, r.allow, r.deny, r.ask),
			mode: Mode(mode), r: r, glob: g.chance(0.7),
		})
	}
	return out
}

// progress appends to the file named in GNOSH_ORACLE_PROGRESS, if any:
// go test holds a test's output back until the test ends, and a long run
// should say how far it is before that.
func progress(msg string) {
	p := os.Getenv("GNOSH_ORACLE_PROGRESS")
	if p == "" {
		return
	}
	if f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644); err == nil {
		f.WriteString(msg)
		f.Close()
	}
}

// noteFinding appends one finding to the file named in
// GNOSH_ORACLE_FINDINGS, if any, as it was found: not shrunk, and every
// one of them. The report at the end of a run shows one line for each
// kind of finding, the shortest; several holes can stand behind one kind,
// and this file is where they can be told apart.
func noteFinding(j *jail, f finding) {
	p := os.Getenv("GNOSH_ORACLE_FINDINGS")
	if p == "" {
		return
	}
	unfill := strings.NewReplacer(j.work, "@WORK@", j.home, "@HOME@", j.outside, "@OUT@", j.root+"/bin", "@BIN@")
	msg := fmt.Sprintf("[%s] %q | %s | %s %s | %s\n", f.kind, unfill.Replace(f.line), unfill.Replace(f.set), f.shell, f.mode, unfill.Replace(f.detail))
	if file, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644); err == nil {
		file.WriteString(msg)
		file.Close()
	}
}

// moveAll rewrites the paths in a list of rules.
func moveAll(move *strings.Replacer, in []string) []string {
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = move.Replace(s)
	}
	return out
}

// oracleStats counts what a run did.
type oracleStats struct {
	lines, shellRuns, allowed, findings int64
}

// runOracle judges n lines on several jails at once and reports every
// distinct disagreement, shrunk to its shortest form.
func runOracle(t *testing.T, n int, seed int64, each bool, lineAt func(i int) string) {
	t.Helper()
	if why := oracleSkip(); why != "" {
		t.Skip(why)
	}
	stub, err := buildStub(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	workers := *oracleWorkers
	if workers <= 0 {
		workers = max(2, runtime.NumCPU()/2)
	}
	workers = min(workers, max(1, n))
	jails := make([]*jail, workers)
	for i := range jails {
		j, err := newJail(stub, i)
		if err != nil {
			t.Fatal(err)
		}
		jails[i] = j
		defer j.close()
	}
	fixed := fixedRuleSets()
	type kept struct {
		f    finding
		sets []ruleSet
		n    int
	}
	var (
		mu    sync.Mutex
		found = map[string]*kept{}
		stats oracleStats
		next  atomic.Int64
		wg    sync.WaitGroup
	)
	start := time.Now()
	// A long run says how far it is, now and then.
	stopTicker := make(chan struct{})
	defer close(stopTicker)
	go func() {
		tick := time.NewTicker(time.Minute)
		defer tick.Stop()
		for {
			select {
			case <-stopTicker:
				return
			case <-tick.C:
				msg := fmt.Sprintf("oracle: %d of %d lines, %d findings, %s\n", atomic.LoadInt64(&stats.lines), n,
					atomic.LoadInt64(&stats.findings), time.Since(start).Round(time.Second))
				progress(msg)
				fmt.Fprint(os.Stderr, msg)
			}
		}
	}()
	for w := range jails {
		wg.Add(1)
		go func(j *jail) {
			defer wg.Done()
			for {
				i := int(next.Add(1)) - 1
				if i >= n {
					return
				}
				line := j.fill(lineAt(i))
				g := newLineGen(seed*1_000_003 + int64(i) + 7)
				sets := append(append([]ruleSet(nil), fixed...), g.randomSets(j.engine(fixed[0]), line)...)
				before := j.runs
				fs := j.checkLine(line, sets, nil)
				atomic.AddInt64(&stats.lines, 1)
				atomic.AddInt64(&stats.shellRuns, int64(j.runs-before))
				for _, rs := range sets {
					if e := j.engine(rs); e != nil && !rs.bare() && rs.mode != ModeBypass && bash(e, line).Behavior == Allow {
						atomic.AddInt64(&stats.allowed, 1)
						break
					}
				}
				if len(fs) == 0 {
					continue
				}
				atomic.AddInt64(&stats.findings, int64(len(fs)))
				mu.Lock()
				for _, f := range fs {
					noteFinding(j, f)
					// Findings are grouped by what went wrong; with each set,
					// by the line as well, so that every line is reported.
					key := f.key()
					if each {
						key = lineAt(i) + "\x00" + f.kind
					}
					k := found[key]
					if k == nil {
						k = &kept{}
						found[key] = k
						// A long run says what it found as it finds it.
						progress(f.String() + "\n")
					}
					k.n++
					if k.n == 1 || len(f.line) < len(k.f.line) {
						k.f, k.sets = f, sets
					}
				}
				mu.Unlock()
			}
		}(jails[w])
	}
	wg.Wait()
	elapsed := time.Since(start)
	summary := fmt.Sprintf("oracle: %d lines, %d shell runs, %d lines allowed under some rule set, in %s (%.0f lines a second, seed %d, %d jails), %d kinds of finding",
		stats.lines, stats.shellRuns, stats.allowed, elapsed.Round(time.Millisecond), float64(stats.lines)/elapsed.Seconds(), seed, workers, len(found))
	t.Log(summary)
	// (A run that finds nothing prints nothing: its size is in the
	// progress file.)
	progress(summary + "\n")
	if len(found) == 0 {
		return
	}
	keys := make([]string, 0, len(found))
	for k := range found {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if len(keys) > *oracleShow {
		keys = keys[:*oracleShow]
	}
	// Each finding is shrunk in whichever jail is free, with the paths of
	// the jail it was found in rewritten to that one's.
	reports := make([]string, len(keys))
	var nextKey atomic.Int64
	for _, j := range jails {
		wg.Add(1)
		go func(j *jail) {
			defer wg.Done()
			unfill := strings.NewReplacer(j.work, "@WORK@", j.home, "@HOME@", j.outside, "@OUT@", j.root+"/bin", "@BIN@")
			for {
				i := int(nextKey.Add(1)) - 1
				if i >= len(keys) {
					return
				}
				k := found[keys[i]]
				f := k.f
				sets := append([]ruleSet(nil), k.sets...)
				for _, o := range jails {
					move := strings.NewReplacer(o.root+"/", j.root+"/")
					f.line, f.detail, f.set = move.Replace(f.line), move.Replace(f.detail), move.Replace(f.set)
					for n := range sets {
						rs := sets[n]
						rs.name = move.Replace(rs.name)
						rs.r = rules{allow: moveAll(move, rs.r.allow), ask: moveAll(move, rs.r.ask), deny: moveAll(move, rs.r.deny)}
						sets[n] = rs
					}
				}
				small := f
				if !each {
					small = j.shrink(f, sets)
				}
				reports[i] = fmt.Sprintf("%dx [%s] %q | %s | %s %s | %s", k.n, small.kind, unfill.Replace(small.line),
					unfill.Replace(small.set), small.shell, small.mode, unfill.Replace(small.detail))
			}
		}(j)
	}
	wg.Wait()
	shown := len(keys)
	for _, r := range reports {
		t.Error(r)
	}
	t.Errorf("oracle: %d findings of %d kinds (%d shown)", stats.findings, len(found), shown)
}

// The long runs that were made, for the record (a 16-core Linux box,
// bash 5.2.21 and dash 0.5.12, 14 jails, about 4.7 shell runs a line):
//
//	seed  lines    shell runs  time      kinds of finding
//	7     400,000  1,614,238   43m22s    30
//	28    400,000  1,894,773   52m41s    9
//	41    400,000  1,891,780   1h03m10s  3
//	45    400,000  1,889,838   1h08m20s  2
//	46    100,000    471,618   16m44s    0
//
// and, between them, a dozen runs of 60,000 to 150,000 lines. Every kind
// found was fixed in the engine, or in the oracle where the oracle was
// the one that was wrong, and is a line of attackLines and a row of
// attack_test.go. Seed 46 was run on the code as it stood after the
// last of those fixes.
//
// The second round, which added the other commands that destroy a
// directory (mv, chmod -R, git clean, shred, truncate, dd, rsync) and the
// generator of tricks (a cd that may not happen, values made of
// variables set before, after and beside them, loops over a pattern):
//
//	seed  lines    shell runs  time      kinds of finding
//	48    400,000  1,883,421   1h07m06s  9
//	49     80,000    378,541   13m56s    6 (17 different lines)
//	50     39,000  stopped                3
//	51     28,000  stopped                0
//	52     74,000  stopped                1
//	53     11,000  stopped                1
//	56     46,000  stopped                1
//	60     57,000  stopped                1
//	66     10,000  stopped                1
//	70     31,000  stopped                1 (the oracle's)
//	72      3,600  the reader crashed on "${}"
//	74     29,000  stopped                3
//	77      3,300  stopped                1
//	78    316,000  stopped                1 (the oracle's)
//	79     60,000    284,350   1h11m35s  0 (4 jails)
//	80     17,000  stopped                2
//	84    400,000  1,874,393   1h31m21s  5 (17 findings)
//
// Seed 84's five kinds were four holes: the words with an = that env takes
// for variables of its own after "--" (env -- 0=1 rm x), what a command
// prints beside text where a wrapper takes a flag's value (env -u $(echo X
// rm x)m), a pattern that is all mv is handed (mv .*, which in dash is
// also "." and ".."), and a cp that lands on a link inside the directory
// it is handed (cp db/s da in acceptEdits, with da/s a link that leads
// out). Each was then judged by hand against the shells with
// TestOracleProbe, and stands in attack_test.go and danger_test.go.
//
// (A run was stopped as soon as it had shown something new, the hole was
// closed, and the next one started with another seed; the seeds between
// those listed were stopped within minutes for the same reason.) Beside
// them ran passes of one generator only (-oracle.only=tricks), which
// found three more kinds in some 100,000 lines of their own, and the
// whole cross product of the joiners with the payloads, 13,118 lines,
// judged one by one with TestOracleProbe: nothing.
//
// One kind is one line of the report, the shortest of its group, and
// several holes can stand behind it: the 193 findings seed 48 filed under
// mv were the {name} that a plain sh takes for an argument, the words
// xargs adds and more. From seed 49 on every finding was written out as
// it was found (GNOSH_ORACLE_FINDINGS) and read one by one; what those
// showed, and what reading the engine against them turned up beyond what
// the generators made then (aliases with words after them, name
// references, the words a function is handed, the forms bash adds to
// ${...}, options written short, lines nobody can read), is closed in the
// engine and stands in attackLines, in the generators, and as rows of
// attack_test.go and danger_test.go.
//
// TestOracle asks real shells what generated command lines do and holds
// the engine's answers against it. With no flag it judges every line of
// attackLines and a few hundred generated ones; -oracle.lines sets how
// many generated lines the long run judges.
func TestOracle(t *testing.T) {
	n := *oracleLines
	if n == 0 {
		n = 400
		if testing.Short() {
			n = 100
		}
	}
	seed := *oracleSeed
	if *oracleLines > 0 && seed == 1 {
		if s := os.Getenv("GNOSH_ORACLE_SEED"); s == "time" {
			seed = time.Now().UnixNano() % 1_000_000_007
		}
	}
	runOracle(t, n, seed, false, func(i int) string {
		g := newLineGen(seed*1_000_003 + int64(i))
		if *oracleOnly != "" {
			// One generator's lines only: a run that goes deep into one
			// kind of line, where the usual mix would touch it now and then.
			return strings.ToValidUTF8(g.only(*oracleOnly), "\uFFFD")
		}
		return g.line()
	})
}

// TestOracleAttacks runs the hand-written lines.
func TestOracleAttacks(t *testing.T) {
	runOracle(t, len(attackLines), 1, true, func(i int) string { return attackLines[i] })
}

// TestOracleProbe is for looking at single lines by hand. It judges the
// lines of the file named in GNOSH_ORACLE_PROBE, one JSON object to a
// line of the file:
//
//	{"line": "cd s&&ls;cat *"}
//	{"line": "for f in *;do mv i$f;done", "mode": "bypassPermissions", "allow": ["Bash(mv if$f)"], "ask": ["Edit(*.txt)"]}
//
// under the fixed rule sets and under the one the object gives, if it
// gives one ("glob": false takes the way to expand patterns away from
// it), and says for each what the engine read, what it decided under
// every set, what each shell ran and every disagreement. A line and a
// rule may hold the jail's placeholders (@WORK@, @HOME@, @OUT@, @BIN@).
// Run it with -v.
func TestOracleProbe(t *testing.T) {
	file := os.Getenv("GNOSH_ORACLE_PROBE")
	if file == "" {
		t.Skip("GNOSH_ORACLE_PROBE names no file of lines to look at")
	}
	if why := oracleSkip(); why != "" {
		t.Skip(why)
	}
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	stub, err := buildStub(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	j, err := newJail(stub, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer j.close()
	unfill := strings.NewReplacer(j.work, "@WORK@", j.home, "@HOME@", j.outside, "@OUT@", j.root+"/bin", "@BIN@")
	for _, ln := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(ln) == "" {
			continue
		}
		var p struct {
			Line, Mode       string
			Allow, Deny, Ask []string
			Glob             *bool
		}
		if err := json.Unmarshal([]byte(ln), &p); err != nil {
			t.Fatalf("%s: %v", ln, err)
		}
		line := j.fill(p.Line)
		sets := fixedRuleSets()
		if p.Mode != "" || len(p.Allow)+len(p.Deny)+len(p.Ask) > 0 {
			mode := Mode(p.Mode)
			if p.Mode == "" {
				mode = ModeDefault
			}
			sets = append(sets, ruleSet{name: "the line's own rules", mode: mode,
				r: rules{allow: p.Allow, ask: p.Ask, deny: p.Deny}, glob: p.Glob == nil || *p.Glob})
		}
		var b strings.Builder
		fmt.Fprintf(&b, "%q\n", p.Line)
		plain := j.engine(ruleSet{name: "plain", mode: ModeDefault})
		v := plain.viewBash(line)
		fmt.Fprintf(&b, "  read: %s (followed to the end: %v; not known: %q)\n", unfill.Replace(unitsText(v)), v.sc.ok, v.opaque)
		for _, rs := range sets {
			e := j.engine(rs)
			if e == nil {
				fmt.Fprintf(&b, "  %s: a rule of the set cannot be applied\n", rs.name)
				continue
			}
			d := bash(e, line)
			critical := ""
			if d.Critical {
				critical = ", critical"
			}
			fmt.Fprintf(&b, "  %s: %s at %s%s: %s\n", rs.name, d.Behavior, d.Stage, critical, unfill.Replace(d.Reason))
		}
		passes := [][2]string{{"bash", "p0"}, {"dash", "p0"}}
		if mentions(line, realNames) {
			passes = append(passes, [2]string{"bash", "r0"})
		}
		for _, ps := range passes {
			r := j.run(ps[0], ps[1], line)
			if r.broken != "" {
				fmt.Fprintf(&b, "  %s %s: %s\n", ps[0], ps[1], r.broken)
				continue
			}
			var ran []string
			for _, rc := range j.ran(r) {
				where := ""
				if rc.cwd != "" && rc.cwd != j.work {
					where = " (in " + rc.cwd + ")"
				}
				ran = append(ran, "["+j.render(rc)+where+"]")
			}
			var changed []string
			for _, ch := range r.changes {
				changed = append(changed, ch.path+" "+ch.what)
			}
			fmt.Fprintf(&b, "  %s %s ran: %s; exit %d; changed: %s\n", ps[0], ps[1], unfill.Replace(strings.Join(ran, " ")), r.exit, unfill.Replace(strings.Join(changed, ", ")))
		}
		fs := j.checkLine(line, sets, nil)
		sortFindings(fs)
		for _, f := range fs {
			fmt.Fprintf(&b, "  FINDING [%s] %s | %s %s | %s\n", f.kind, f.set, f.shell, f.mode, unfill.Replace(f.detail))
		}
		if len(fs) > 0 {
			t.Error(b.String())
		} else {
			t.Log(b.String())
		}
	}
}
