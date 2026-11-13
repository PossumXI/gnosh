#!/bin/sh
# run.sh - the similarity audit, from the start.
#
# Compares a copy of the new gnosh with Charm's code and writes the measures
# to $WORK. Run it on a machine that may hold Crush's source (the devbox),
# never inside the new repository's working tree: $WORK ends up holding
# every version of every Crush file.
#
#   CRUSH_REPO=/path/to/clone NEW_TREE=/path/to/copy WORK=/path/to/work ./run.sh
#
# What it needs:
#
#   CRUSH_REPO   a git clone whose remote "upstream" is
#                github.com/charmbracelet/crush with every branch and tag
#                fetched (git fetch upstream '+refs/heads/*:refs/remotes/upstream/*' --tags).
#                If the earlier gnosh's history is in the same clone, set
#                OLD_MAIN to its last commit: the calibration sets and the
#                "earlier gnosh" corpus come from it. Leave OLD_MAIN empty
#                to do without them.
#   FORK_POINT   the commit of Crush the earlier gnosh started from
#   UPSTREAM     the commit of upstream's main branch to compare against
#   NEW_TREE     a clean copy of the commit of the new gnosh to audit
#                (git archive COMMIT | tar -x -C DIR)
#   WORK         an empty directory with a few gigabytes free
#   MODCACHE     a Go module cache; what is missing from it (Charm's libraries
#                at the versions the earlier gnosh pinned, the control
#                programs) is downloaded into CTLCACHE
#   INVENTORY    the earlier inventory's output folder (classified.json for
#                two of the calibration sets, and beside it corpus.pkl for the
#                check of the carried files' numbers); may be empty
#
# and python3, git, and Go 1.27 on the PATH. The line measures also read the
# generic-Go reference whose folders are named at the top of taintscan.py
# (Go's own source and a dozen modules of the module cache); a folder that is
# missing is reported and left out, which makes a few more lines count as
# significant.
#
# It takes about half an hour on sixteen cores, most of it the token scan.
set -eu
HERE=$(cd "$(dirname "$0")" && pwd)

: "${CRUSH_REPO:=/root/src/gnosh-old}"
: "${FORK_POINT:=52c75da8acbcd11dc269ae582f3ed7947ba3cff6}"
: "${UPSTREAM:=7ef47d5c85f3b2b55eee234b51ba5275d61bab4a}"
: "${OLD_MAIN=1c77d2bd58482c37dd858b24326a6e1829117904}"
: "${NEW_TREE:=/root/src/gnosh-new-audit/tree}"
: "${WORK:=/root/src/gnosh-new-audit/work}"
: "${GOROOT_SRC:=$(go env GOROOT)/src}"
: "${MODCACHE:=$(go env GOMODCACHE)}"
: "${CTLCACHE:=$(dirname "$WORK")/modcache}"
: "${INVENTORY=/root/src/gnosh-old-scan/out}"
export CRUSH_REPO FORK_POINT UPSTREAM OLD_MAIN NEW_TREE WORK GOROOT_SRC MODCACHE CTLCACHE INVENTORY

case "$WORK" in "$NEW_TREE"/*) echo "run.sh: WORK must not be inside NEW_TREE" >&2; exit 2 ;; esac
mkdir -p "$WORK" "$CTLCACHE"
cd "$WORK"
L=lists

echo "== 0. the modules that are not at hand"
need=$(python3 "$HERE/build_corpus.py" need)
if [ -n "$need" ]; then
	mkdir -p "$WORK/mod"
	(cd "$WORK/mod" && { [ -f go.mod ] || go mod init audit/mod >/dev/null 2>&1; } &&
		for m in $need; do
			echo "   downloading $m"
			GOMODCACHE="$CTLCACHE" GOFLAGS=-mod=mod go mod download "$m"
		done)
fi

echo "== 1. the corpora and the lists of files"
python3 "$HERE/build_corpus.py" all

echo "== 2. the two Go tools"
go build -o "$WORK/tokscan" "$HERE/tokscan.go"
go build -o "$WORK/idents" "$HERE/idents.go"

corpora="-corpus core=$L/crush-core.tsv -corpus ext=$L/crush-ext.tsv -corpus libs=$L/libs.tsv"
texts="core ext libs"
if [ -n "$OLD_MAIN" ]; then
	corpora="$corpora -corpus old=$L/old.tsv -corpus deps=$L/deps.tsv"
	texts="$texts old"
fi

echo "== 3. lines"
python3 "$HERE/linescan.py" scan

echo "== 4. tokens"
# shellcheck disable=SC2086
./tokscan -targets $L/targets.tsv $corpora -out tok.tsv -matches tok-matches.tsv

echo "== 5. words"
./tokscan -strings $L/targets.tsv -out strings-targets.tsv
./tokscan -strings $L/crush-core.tsv -uniq -out strings-core.tsv
./tokscan -strings $L/crush-ext.tsv -uniq -out strings-ext.tsv
./tokscan -strings $L/libs.tsv -uniq -out strings-libs.tsv
[ -z "$OLD_MAIN" ] || ./tokscan -strings $L/old.tsv -uniq -out strings-old.tsv
# shellcheck disable=SC2086
python3 "$HERE/textscan.py" $texts

echo "== 6. names and whole files"
./idents -decl $L/crush-core.tsv -out decl-core.tsv
./idents -decl $L/crush-ext.tsv -out decl-ext.tsv
./idents -decl $L/libs.tsv -out decl-libs.tsv
./idents -decl $L/ref.tsv -out decl-ref.tsv
./idents -use $L/targets.tsv -out use-targets.tsv
./tokscan -strings $L/crush-ext.tsv -uniq -minstr 3 -out strings-ext-all.tsv
./tokscan -strings $L/ref.tsv -uniq -minstr 3 -out strings-ref-all.tsv
python3 "$HERE/names.py" gitsim basenames idents api rc terms gomod

echo "== 7. the third-party files and the carried files"
python3 "$HERE/thirdparty.py" ${OFFLINE:+--offline}
[ -z "$OLD_MAIN" ] || python3 "$HERE/carried_check.py"

echo "== 8. the tables"
python3 "$HERE/report.py"
python3 "$HERE/review.py"

echo "done: $WORK/results.tsv, summary.md, review.md, and the working files beside them"
