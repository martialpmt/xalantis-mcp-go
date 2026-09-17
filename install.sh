#!/bin/sh
# Installe xalantis-mcp-go depuis les GitHub Releases (macOS, Linux).
#
#   curl -fsSL https://raw.githubusercontent.com/martialpmt/xalantis-mcp-go/main/install.sh | sh
#
# Variables facultatives :
#   VERSION      version à installer : latest (défaut) ou vX.Y.Z
#   INSTALL_DIR  dossier d'installation (défaut : $HOME/.local/bin)
#   BASE_URL     URL des releases (tests locaux uniquement)
set -eu

BINARY=xalantis-mcp-go
RELEASES_URL=https://github.com/martialpmt/xalantis-mcp-go/releases
VERSION=${VERSION:-latest}
INSTALL_DIR=${INSTALL_DIR:-$HOME/.local/bin}
BASE_URL=${BASE_URL:-$RELEASES_URL}

fail() {
	echo "install.sh : $*" >&2
	exit 1
}

case $(uname -s) in
Darwin) os=darwin ;;
Linux) os=linux ;;
*) fail "système non pris en charge : $(uname -s). Téléchargez l'archive sur $RELEASES_URL" ;;
esac

case $(uname -m) in
x86_64 | amd64) arch=amd64 ;;
arm64 | aarch64) arch=arm64 ;;
*) fail "architecture non prise en charge : $(uname -m). Téléchargez l'archive sur $RELEASES_URL" ;;
esac

case $VERSION in
latest) url=$BASE_URL/latest/download ;;
v[0-9]*) url=$BASE_URL/download/$VERSION ;;
*) fail "VERSION invalide : $VERSION (attendu : latest ou vX.Y.Z)" ;;
esac

if command -v curl >/dev/null 2>&1; then
	download() { curl -fsSL -o "$2" "$1"; }
elif command -v wget >/dev/null 2>&1; then
	download() { wget -qO "$2" "$1"; }
else
	fail "curl ou wget est nécessaire"
fi

if command -v sha256sum >/dev/null 2>&1; then
	sha256() { sha256sum "$1" | awk '{print $1}'; }
elif command -v shasum >/dev/null 2>&1; then
	sha256() { shasum -a 256 "$1" | awk '{print $1}'; }
else
	fail "sha256sum ou shasum est nécessaire"
fi

archive=${BINARY}_${os}_${arch}.tar.gz
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

echo "Téléchargement de $archive ($VERSION)…"
download "$url/$archive" "$tmp/$archive" || fail "téléchargement impossible : $url/$archive"
download "$url/checksums.txt" "$tmp/checksums.txt" || fail "téléchargement impossible : $url/checksums.txt"

expected=$(awk -v f="$archive" '$2 == f {print $1}' "$tmp/checksums.txt")
[ -n "$expected" ] || fail "$archive absent de checksums.txt"
[ "$(sha256 "$tmp/$archive")" = "$expected" ] || fail "somme de contrôle invalide pour $archive, rien n'a été installé"

tar -xzf "$tmp/$archive" -C "$tmp" "$BINARY"
mkdir -p "$INSTALL_DIR"
cp "$tmp/$BINARY" "$INSTALL_DIR/$BINARY.tmp"
chmod 0755 "$INSTALL_DIR/$BINARY.tmp"
mv -f "$INSTALL_DIR/$BINARY.tmp" "$INSTALL_DIR/$BINARY"

echo "Installé : $INSTALL_DIR/$BINARY ($("$INSTALL_DIR/$BINARY" --version))"
case :$PATH: in
*:"$INSTALL_DIR":*) ;;
*)
	echo "Attention : $INSTALL_DIR n'est pas dans PATH. Ajoutez par exemple à votre profil :"
	echo "  export PATH=\"$INSTALL_DIR:\$PATH\""
	;;
esac
