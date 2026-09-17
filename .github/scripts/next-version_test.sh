#!/bin/sh
# Tests de next-version.sh. Usage : sh .github/scripts/next-version_test.sh
set -u

script="$(dirname "$0")/next-version.sh"
fail=0

# expect <attendu> <arguments…> ; "ERREUR" attend un code de sortie non nul.
expect() {
	want=$1
	shift
	got=$(sh "$script" "$@" 2>/dev/null) || got=ERREUR
	if [ "$got" != "$want" ]; then
		echo "ÉCHEC : next-version.sh $* => $got, attendu $want"
		fail=1
	fi
}

expect v0.0.1 "" patch
expect v0.1.0 "" minor
expect v1.0.0 "" major
expect v0.1.1 v0.1.0 patch
expect v0.2.0 v0.1.3 minor
expect v1.0.0 v0.9.9 major
expect v1.10.0 v1.9.4 minor
expect v1.2.10 v1.2.9 patch
expect ERREUR v1.2.0 major
expect ERREUR v0.1.0 bogus
expect ERREUR v0.1.0
expect ERREUR 1.2.3 patch
expect ERREUR v1.2.3-rc.1 patch
expect ERREUR v1.2.08 patch
expect ERREUR v1.2.010 patch
expect ERREUR v01.2.3 patch
expect v1.0.1 v1.0.0 patch

if [ "$fail" -eq 0 ]; then
	echo OK
fi
exit "$fail"
