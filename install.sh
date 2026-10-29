#!/bin/sh
# Tower of Doom installer.
#
#   curl -fsSL https://raw.githubusercontent.com/lckrugel/tower-of-doom/main/install.sh | sh
#
# Downloads the release archive for this machine, checks its sha256 against the
# release's checksums.txt and installs `tower-of-doom` (the game) plus
# `tower-validate` (the asset checker) into $BIN_DIR.
#
# Environment overrides:
#   VERSION  release to install, with or without the leading "v" (default: latest)
#   BIN_DIR  where to install (default: $HOME/.local/bin)
#   REPO     GitHub slug to fetch from (default: lckrugel/tower-of-doom)
#
# Needs sh, tar and curl (or wget), plus sha256sum or shasum to verify. Files
# only appear in $BIN_DIR once the download is verified and fully extracted.

set -eu

REPO=${REPO:-lckrugel/tower-of-doom}
VERSION=${VERSION:-}
BIN_DIR=${BIN_DIR:-}
work=''
staged=''

# die reports a fatal error on stderr and exits non-zero.
die() {
	printf 'install: %s\n' "$*" >&2
	exit 1
}

# cleanup removes the temporary directory and any staged (not yet renamed)
# binary, preserving the script's exit status.
cleanup() {
	status=$?
	[ -n "$staged" ] && rm -f "$staged" 2>/dev/null || true
	[ -n "$work" ] && rm -rf "$work" 2>/dev/null || true
	exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

# fetch downloads a URL into a file, or to stdout when dest is "-".
fetch() {
	if command -v curl >/dev/null 2>&1; then
		if [ "$2" = '-' ]; then curl -fsSL "$1"; else curl -fsSL -o "$2" "$1"; fi
	elif command -v wget >/dev/null 2>&1; then
		if [ "$2" = '-' ]; then wget -qO- "$1"; else wget -q -O "$2" "$1"; fi
	else
		die "need curl or wget to download $1"
	fi
}

# install_bin copies a verified binary into BIN_DIR atomically: it is staged
# under a temporary name on the same filesystem and renamed into place, so a
# failure never leaves a half-written or non-executable binary behind.
install_bin() {
	src=$1
	dest=$2
	staged="$BIN_DIR/.${dest##*/}.partial.$$"
	if command -v install >/dev/null 2>&1; then
		install -m 0755 "$src" "$staged" || die "cannot stage $dest"
	else
		cp "$src" "$staged" || die "cannot stage $dest"
		chmod 0755 "$staged" || die "cannot make $dest executable"
	fi
	mv -f "$staged" "$dest" || {
		rm -f "$staged" 2>/dev/null || true
		staged=''
		die "cannot install $dest"
	}
	staged=''
}

# --- platform ---------------------------------------------------------------

os=$(uname -s)
case $os in
	Linux) os=linux ;;
	Darwin) os=darwin ;;
	MINGW* | MSYS* | CYGWIN*)
		die "Windows ships .zip archives: download tower-of-doom_<version>_windows_<arch>.zip from https://github.com/$REPO/releases and unzip it manually"
		;;
	*) die "unsupported operating system: $os (supported: linux, darwin)" ;;
esac

arch=$(uname -m)
case $arch in
	x86_64 | amd64) arch=amd64 ;;
	aarch64 | arm64) arch=arm64 ;;
	*) die "unsupported architecture: $arch (supported: x86_64, aarch64)" ;;
esac

if [ -z "$BIN_DIR" ]; then
	[ -n "${HOME:-}" ] || die "HOME is unset; set BIN_DIR to choose where to install"
	BIN_DIR=$HOME/.local/bin
fi
# Honour a literal "~/" even when the calling shell did not expand it.
case $BIN_DIR in
	'~/'*)
		[ -n "${HOME:-}" ] || die "cannot expand ~ without HOME; set BIN_DIR to an absolute path"
		BIN_DIR=$HOME/${BIN_DIR#\~/}
		;;
esac

# --- version ----------------------------------------------------------------

if [ -z "$VERSION" ]; then
	json=$(fetch "https://api.github.com/repos/$REPO/releases/latest" -) ||
		die "cannot read https://api.github.com/repos/$REPO/releases/latest (network or rate limit? set VERSION=x.y.z)"
	VERSION=$(printf '%s\n' "$json" |
		sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\(v\{0,1\}[^"]*\)".*/\1/p' |
		head -n 1)
	[ -n "$VERSION" ] || die "no published release found for $REPO; set VERSION"
fi
VERSION=${VERSION#v}
case $VERSION in
	'' | *[!0-9A-Za-z.+-]*) die "invalid VERSION: $VERSION" ;;
esac

# --- download ---------------------------------------------------------------

work=$(mktemp -d) || die "cannot create a temporary directory"
archive=tower-of-doom_${VERSION}_${os}_${arch}.tar.gz
base="https://github.com/$REPO/releases/download/v$VERSION"

printf 'downloading %s/%s\n' "$base" "$archive"
fetch "$base/$archive" "$work/$archive" ||
	die "download failed: is v$VERSION published for $os/$arch?"
fetch "$base/checksums.txt" "$work/checksums.txt" ||
	die "download failed: $base/checksums.txt"

# --- verify -----------------------------------------------------------------

expected=''
while read -r hash name || [ -n "$hash" ]; do
	if [ "$name" = "$archive" ]; then
		expected=$hash
		break
	fi
done <"$work/checksums.txt"
[ -n "$expected" ] || die "checksums.txt lists no entry for $archive; refusing to install"

if command -v sha256sum >/dev/null 2>&1; then
	actual=$(sha256sum "$work/$archive" | cut -d' ' -f1)
elif command -v shasum >/dev/null 2>&1; then
	actual=$(shasum -a 256 "$work/$archive" | cut -d' ' -f1)
else
	die "need sha256sum or shasum to verify the download"
fi
[ "$actual" = "$expected" ] ||
	die "sha256 mismatch for $archive (expected $expected, got $actual); nothing installed"

# Both binaries sit at the archive root; extract only those, so no README or
# other archive content can end up anywhere.
(cd "$work" && tar -xzf "$archive" tower-of-doom tower-validate) ||
	die "cannot extract $archive"
for bin in tower-of-doom tower-validate; do
	[ -f "$work/$bin" ] || die "archive did not contain $bin"
done

# --- install ----------------------------------------------------------------

mkdir -p "$BIN_DIR" || die "cannot create $BIN_DIR"
install_bin "$work/tower-of-doom" "$BIN_DIR/tower-of-doom"
install_bin "$work/tower-validate" "$BIN_DIR/tower-validate"

# --- report -----------------------------------------------------------------

printf '\ninstalled tower-of-doom %s:\n  %s\n  %s\n\n' "$VERSION" \
	"$BIN_DIR/tower-of-doom" "$BIN_DIR/tower-validate"
"$BIN_DIR/tower-of-doom" --version || true
"$BIN_DIR/tower-validate" --version || true

case ":${PATH:-}:" in
	*":$BIN_DIR:"*) ;;
	*)
		printf '\n%s is not on your PATH. Add it with:\n\n  export PATH="%s:$PATH"\n' \
			"$BIN_DIR" "$BIN_DIR"
		;;
esac
