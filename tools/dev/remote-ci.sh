#!/bin/sh
# Build, vet and test what is committed, from a clean copy on the devbox.
#
#   tools/dev/remote-ci.sh [-race] [-short] [-gnovm] [-tidy] [REF]
#
# REF is a commit (default HEAD). Its tree is sent with git archive, so files
# that are not committed are not part of the run: this is the check that the
# repository as recorded builds and passes, whatever is lying in the working
# tree. The copy goes to a directory named after the commit and replaces any
# earlier copy of the same commit.
#
#   -race    run the tests with the race detector
#   -short   pass -short to the tests
#   -gnovm   also run internal/gnovm's own suite, which takes minutes (twenty
#            with -race) and has a timing test that wants a quiet machine;
#            without this flag that one package is built and vetted only
#   -tidy    run go mod tidy in the copy first and print the go.mod that
#            results, without touching this tree
#
# The exit status is the remote one.
set -eu
cd "$(dirname "$0")/../.."

HOST=${GNOSH_DEV_HOST:-root@157.245.185.86}
KEY=${GNOSH_DEV_KEY:-$HOME/.ssh/id_ed25519_kourt}
ROOT=${GNOSH_CI_ROOT:-/root/ci}

flags=""
gnovm=0
tidy=0
ref=HEAD
while [ $# -gt 0 ]; do
	case "$1" in
	-race | -short) flags="$flags $1" ;;
	-gnovm) gnovm=1 ;;
	-tidy) tidy=1 ;;
	-*)
		echo "remote-ci: unknown flag $1" >&2
		exit 2
		;;
	*) ref=$1 ;;
	esac
	shift
done

sha=$(git rev-parse --short=12 "$ref^{commit}")
dir="$ROOT/gnosh-$sha"
SSH="ssh -i $KEY -o IdentitiesOnly=yes -o BatchMode=yes -o ConnectTimeout=20"

# shellcheck disable=SC2086,SC2029
git archive "$sha" | $SSH "$HOST" "rm -rf $dir && mkdir -p $dir && tar -x -C $dir"

run="cd $dir && export PATH=/usr/local/go/bin:\$PATH && echo \"commit $sha in $dir\""
if [ "$tidy" = 1 ]; then
	run="$run && go mod tidy && echo '--- go.mod after tidy:' && cat go.mod && echo '--- end go.mod'"
fi
run="$run && bad=\$(gofmt -l \$(git -C . ls-files 2>/dev/null || find . -name '*.go' -not -path './internal/gnovm/stdlibs/*') 2>/dev/null | grep -v '^internal/gnovm/' || true) && { [ -z \"\$bad\" ] || { echo 'gofmt -w needed for:'; echo \"\$bad\"; exit 1; }; }"
run="$run && go build ./... && go vet ./..."
if [ "$gnovm" = 1 ]; then
	run="$run && go test -count=1 -timeout 60m $flags ./..."
else
	run="$run && go test -count=1 -timeout 30m $flags \$(go list ./... | grep -v '/internal/gnovm\$')"
fi

# shellcheck disable=SC2086,SC2029
$SSH "$HOST" "$run"
