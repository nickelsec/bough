#!/bin/sh
# Installs bough on macOS and Linux.
#
#   curl -fsSL https://www.bough.run/install.sh | sh
#
# This is piped into a shell, which means nobody reads it before it runs. So it
# verifies the checksum of what it downloaded before extracting anything, and
# it touches nothing outside the install directory and a temp dir it removes on
# the way out.
#
# BOUGH_VERSION pins a release (BOUGH_VERSION=v0.5.0), BOUGH_DIR chooses where
# the binary lands. Both are optional.

set -eu

REPO="nickelsec/bough"
API="https://api.github.com/repos/$REPO/releases"
DOWNLOAD="https://github.com/$REPO/releases/download"

say() { printf '%s\n' "$*"; }
die() { printf 'bough: %s\n' "$*" >&2; exit 1; }

need() {
	command -v "$1" >/dev/null 2>&1 || die "$1 is needed and was not found"
}

# curl or wget, whichever is here. Alpine ships neither by default, so the
# message has to say what to do about it rather than just naming the gap.
if command -v curl >/dev/null 2>&1; then
	fetch() { curl -fsSL "$1"; }
	fetch_to() { curl -fsSL -o "$2" "$1"; }
elif command -v wget >/dev/null 2>&1; then
	fetch() { wget -qO- "$1"; }
	fetch_to() { wget -qO "$2" "$1"; }
else
	die "needs curl or wget to download anything"
fi

need tar

# What are we on.
os=$(uname -s)
arch=$(uname -m)

case "$os" in
	Darwin) os=darwin ;;
	Linux)  os=linux ;;
	*) die "no build for $os. The releases page lists what there is: https://github.com/$REPO/releases" ;;
esac

case "$arch" in
	x86_64|amd64)  arch=amd64 ;;
	arm64|aarch64) arch=arm64 ;;
	*) die "no build for $arch. The releases page lists what there is: https://github.com/$REPO/releases" ;;
esac

# Which release. Reading the tag from the API rather than hardcoding one is
# what keeps this script working after a release without anybody editing it.
if [ -n "${BOUGH_VERSION:-}" ]; then
	tag=$BOUGH_VERSION
else
	tag=$(fetch "$API/latest" | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -1)
	[ -n "$tag" ] || die "could not work out the latest version. GitHub may be rate limiting; try again, or set BOUGH_VERSION"
fi

archive="bough_${tag}_${os}_${arch}.tar.gz"

tmp=$(mktemp -d 2>/dev/null || mktemp -d -t bough)
trap 'rm -rf "$tmp"' EXIT INT TERM

say "Downloading bough $tag for $os/$arch"
fetch_to "$DOWNLOAD/$tag/$archive" "$tmp/$archive" \
	|| die "could not download $archive. Is $tag a real release?"
fetch_to "$DOWNLOAD/$tag/checksums.txt" "$tmp/checksums.txt" \
	|| die "could not download the checksums for $tag"

# The integrity check. Everything above this point came off the network.
want=$(sed -n "s/^\([0-9a-f]\{64\}\)  *$archive\$/\1/p" "$tmp/checksums.txt" | head -1)
[ -n "$want" ] || die "$archive is not listed in checksums.txt"

if command -v sha256sum >/dev/null 2>&1; then
	got=$(sha256sum "$tmp/$archive" | cut -d' ' -f1)
elif command -v shasum >/dev/null 2>&1; then
	got=$(shasum -a 256 "$tmp/$archive" | cut -d' ' -f1)
else
	die "needs sha256sum or shasum to check the download"
fi

[ "$want" = "$got" ] || die "checksum mismatch. Expected $want, got $got. Not installing."

tar -xzf "$tmp/$archive" -C "$tmp"
binary="$tmp/bough_${tag}_${os}_${arch}/bough"
[ -f "$binary" ] || die "the archive did not contain the binary where expected"
chmod +x "$binary"

# Where it goes. A directory the user already owns beats sudo, so /usr/local/bin
# is used only when it is writable without one.
if [ -n "${BOUGH_DIR:-}" ]; then
	dir=$BOUGH_DIR
elif [ -w /usr/local/bin ] 2>/dev/null; then
	dir=/usr/local/bin
else
	dir="$HOME/.local/bin"
fi

mkdir -p "$dir" || die "could not create $dir"
mv "$binary" "$dir/bough" || die "could not write to $dir"

say "Installed to $dir/bough"

# Saying the binary is installed while the shell cannot find it is the most
# common way one of these scripts wastes somebody's afternoon.
case ":$PATH:" in
	*":$dir:"*) say "Run: bough" ;;
	*)
		say ""
		say "$dir is not on your PATH. Add it:"
		say "  export PATH=\"\$PATH:$dir\""
		say ""
		say "Or run it directly: $dir/bough"
		;;
esac
