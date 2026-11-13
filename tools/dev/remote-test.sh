#!/bin/sh
# Vet and test packages of this repo on the devbox. Go builds and tests never
# run on the Mac.
#
#   tools/dev/remote-test.sh [-race] [-v] [-short] [-run REGEX] [-timeout D]
#                            [-fuzz NAME -fuzztime D] DIR...
#
# DIR is a package directory relative to the repo root, such as internal/llm.
# The script copies go.mod, go.sum, internal/core and each DIR to the devbox
# (inside those directories, files gone here are deleted there; nothing else
# on the devbox is touched), then runs gofmt -l, go vet and go test for
# ./DIR/... there. Its exit status is the remote one.
#
# A package under test may import only internal/core and packages inside the
# DIRs named in the same run.
set -eu
cd "$(dirname "$0")/../.."

HOST=${GNOSH_DEV_HOST:-root@157.245.185.86}
KEY=${GNOSH_DEV_KEY:-$HOME/.ssh/id_ed25519_kourt}
REMOTE=${GNOSH_DEV_DIR:-/root/src/gnosh}

flags=""
dirs=""
while [ $# -gt 0 ]; do
	case "$1" in
	-race | -v | -short)
		flags="$flags $1"
		shift
		;;
	-run | -timeout | -count | -bench | -fuzz | -fuzztime)
		flags="$flags $1 '$2'"
		shift 2
		;;
	-*)
		echo "remote-test: unknown flag $1" >&2
		exit 2
		;;
	*)
		d=${1%/}
		[ -d "$d" ] || {
			echo "remote-test: no such directory: $d" >&2
			exit 2
		}
		dirs="$dirs $d"
		shift
		;;
	esac
done
[ -n "$dirs" ] || {
	echo "usage: tools/dev/remote-test.sh [-race] [-v] [-run REGEX] DIR..." >&2
	exit 2
}

SSH="ssh -i $KEY -o IdentitiesOnly=yes -o BatchMode=yes -o ConnectTimeout=20"

paths="./go.mod ./internal/core"
[ -f go.sum ] && paths="$paths ./go.sum"
pkgs=""
for d in $dirs; do
	paths="$paths ./$d"
	pkgs="$pkgs ./$d/..."
done

# shellcheck disable=SC2086
rsync -az --delete --relative -e "$SSH" $paths "$HOST:$REMOTE/"

# shellcheck disable=SC2086,SC2029
$SSH "$HOST" "cd $REMOTE && export PATH=/usr/local/go/bin:\$PATH && bad=\$(gofmt -l $dirs) && { [ -z \"\$bad\" ] || { echo 'gofmt -w needed for:'; echo \"\$bad\"; exit 1; }; } && go vet $pkgs && go test -count=1 $flags $pkgs"
