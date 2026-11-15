#!/bin/sh
# Copy package directories of this repo to the devbox without testing them.
#
#   tools/dev/remote-sync.sh [-root] DIR...
#
# DIR is a package directory relative to the repo root, such as internal/llm.
# Each is copied onto its own place on the devbox, one at a time, exactly as
# remote-test.sh copies them: inside a copied directory files gone here are
# deleted there, and nothing outside those directories is touched. go.mod,
# go.sum and internal/core go along.
#
# It is for the wiring, which imports a dozen finished packages and tests
# only itself: the packages it builds against are sent once with this, and
# then remote-test.sh is given internal/app alone.
#
#   -root   also copy the .go files at the repo root (main.go and what stands
#           beside it), which belong to no package directory
#
# internal/gnovm is copied without its tools directory, which has an owner of
# its own who copies it.
set -eu
cd "$(dirname "$0")/../.."

HOST=${GNOSH_DEV_HOST:?set GNOSH_DEV_HOST to the devbox, as user@address (and GNOSH_DEV_KEY to its ssh key, when one is needed)}
KEY=${GNOSH_DEV_KEY:-}
REMOTE=${GNOSH_DEV_DIR:-/root/src/gnosh}

root=0
dirs=""
while [ $# -gt 0 ]; do
	case "$1" in
	-root) root=1 ;;
	-*)
		echo "remote-sync: unknown flag $1" >&2
		exit 2
		;;
	*)
		d=${1%/}
		d=${d#./}
		[ -d "$d" ] || {
			echo "remote-sync: no such directory: $d" >&2
			exit 2
		}
		dirs="$dirs $d"
		;;
	esac
	shift
done

SSH="ssh ${KEY:+-i $KEY -o IdentitiesOnly=yes} -o BatchMode=yes -o ConnectTimeout=20"

mk=""
for d in internal/core $dirs; do
	mk="$mk $REMOTE/$d"
done
# shellcheck disable=SC2086,SC2029
$SSH "$HOST" "mkdir -p $mk"

mods="go.mod"
[ -f go.sum ] && mods="$mods go.sum"
# shellcheck disable=SC2086
rsync -az -e "$SSH" $mods "$HOST:$REMOTE/"

for d in internal/core $dirs; do
	case "$d" in
	internal/gnovm) rsync -az --delete --exclude '/tools/' -e "$SSH" "$d/" "$HOST:$REMOTE/$d/" ;;
	*) rsync -az --delete -e "$SSH" "$d/" "$HOST:$REMOTE/$d/" ;;
	esac
done

if [ "$root" = 1 ]; then
	# shellcheck disable=SC2046
	rsync -az -e "$SSH" $(ls ./*.go) "$HOST:$REMOTE/"
fi
