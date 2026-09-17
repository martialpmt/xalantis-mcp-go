#!/bin/sh
# Calcule la prochaine version vX.Y.Z.
# Usage : next-version.sh <dernier tag, vide si aucun> <patch|minor|major>
set -eu

latest=${1:-v0.0.0}
bump=${2:-}

if ! printf '%s\n' "$latest" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+$'; then
	echo "tag invalide : $latest (attendu vX.Y.Z)" >&2
	exit 1
fi

rest=${latest#v}
major=${rest%%.*}
rest=${rest#*.}
minor=${rest%%.*}
patch=${rest#*.}

case $bump in
patch) patch=$((patch + 1)) ;;
minor)
	minor=$((minor + 1))
	patch=0
	;;
major)
	major=$((major + 1))
	minor=0
	patch=0
	;;
*)
	echo "bump invalide : '$bump' (attendu patch, minor ou major)" >&2
	exit 1
	;;
esac

if [ "$major" -ge 2 ]; then
	echo "v$major.$minor.$patch refusée : une version v2+ exige le suffixe /v$major dans le chemin du module" >&2
	exit 1
fi

echo "v$major.$minor.$patch"
