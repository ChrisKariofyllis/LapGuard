#!/bin/sh
# Resolve the version string embedded in the binary.
#
#   ./scripts/version.sh              # see default rules below
#   ./scripts/version.sh "$VERSION"   # normalize an explicit value
#
# Default (no argument):
#   - exact git tag on HEAD when the working tree is clean
#   - otherwise "dev"
#
# Never walks to an older tag and never appends "-dirty".
# Explicit values strip a leading "v" and a trailing "-dirty".
set -eu

normalize() {
	v=$1
	v=${v#v}
	case $v in
	*-dirty) v=${v%-dirty} ;;
	esac
	if [ -z "$v" ]; then
		v=dev
	fi
	printf '%s\n' "$v"
}

if [ "${1+set}" = "set" ] && [ -n "$1" ]; then
	normalize "$1"
	exit 0
fi

if git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
	if tag=$(git describe --tags --exact-match HEAD 2>/dev/null); then
		if [ -z "$(git status --porcelain)" ]; then
			normalize "$tag"
			exit 0
		fi
	fi
fi

printf '%s\n' dev
