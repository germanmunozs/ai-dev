#!/bin/sh
# scripts/install.sh: install ai-dev from a GitHub release, without Go or
# Git. It needs only curl, tar and sha256sum or shasum, which macOS and
# Linux provide. It verifies the archive against the release's SHA256SUMS
# and, when the GitHub CLI is installed, its signed provenance
# attestation. It changes only the installed binary: no shell startup
# file, configuration or credential.
#
#   sh install.sh [--version vX.Y.Z] [--prefix DIR]   install (default: latest release)
#   sh install.sh --uninstall [--prefix DIR]          remove the binary
#
# Everything runs inside main, called on the last line, so a download cut
# short by "curl … | sh" runs nothing.
set -eu

repo="germanmunozs/ai-dev"
releases="${AI_DEV_RELEASES_URL:-https://github.com/$repo/releases}"

die() { printf 'ai-dev install: %s\n' "$*" >&2; exit 1; }

platform() {
	case "$(uname -s)" in
	Darwin) os=darwin ;;
	Linux) os=linux ;;
	*) die "unsupported system $(uname -s); on Windows use WSL2 (see the README)" ;;
	esac
	case "$(uname -m)" in
	x86_64 | amd64) arch=amd64 ;;
	arm64 | aarch64) arch=arm64 ;;
	*) die "unsupported architecture $(uname -m)" ;;
	esac
}

sha256() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | cut -d ' ' -f 1
	elif command -v shasum >/dev/null 2>&1; then
		shasum -a 256 "$1" | cut -d ' ' -f 1
	else
		die "neither sha256sum nor shasum is installed"
	fi
}

valid_version() {
	printf '%s\n' "$1" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+$'
}

# latest_version follows GitHub's redirect from /releases/latest to the
# tag page, without the rate-limited API.
latest_version() {
	url="$(curl -fsSL -o /dev/null -w '%{url_effective}' "$releases/latest")" ||
		die "cannot reach $releases"
	tag="${url##*/tag/}"
	[ "$tag" != "$url" ] && valid_version "$tag" ||
		die "no release is published yet at $releases"
	printf '%s\n' "$tag"
}

uninstall() {
	target="$prefix/ai-dev"
	[ -e "$target" ] || { echo "Nothing to remove: $target does not exist."; return 0; }
	"$target" version --format json 2>/dev/null | grep -q '"schema_version":"ai-dev.version/v1"' ||
		die "$target does not look like ai-dev; not removing it"
	rm -f "$target"
	echo "Removed $target."
}

install_release() {
	command -v curl >/dev/null 2>&1 || die "curl is not installed"
	command -v tar >/dev/null 2>&1 || die "tar is not installed"
	platform
	[ -n "$version" ] || version="$(latest_version)"
	valid_version "$version" || die "version $version is not vMAJOR.MINOR.PATCH"
	asset="ai-dev_${version}_${os}_${arch}.tar.gz"

	work="$(mktemp -d)"
	trap 'rm -rf "$work"' EXIT INT TERM
	# Braces and ASCII only: under a UTF-8 locale, macOS sh reads a
	# multibyte character after $arch as part of the name.
	echo "Downloading ai-dev ${version} for ${os}/${arch}..."
	curl -fsSL -o "$work/$asset" "$releases/download/$version/$asset" ||
		die "cannot download $asset from release $version"
	curl -fsSL -o "$work/SHA256SUMS" "$releases/download/$version/SHA256SUMS" ||
		die "cannot download SHA256SUMS from release $version"

	expected="$(awk -v name="$asset" '$2 == name || $2 == "*" name { print $1 }' "$work/SHA256SUMS")"
	[ -n "$expected" ] || die "SHA256SUMS has no entry for $asset"
	actual="$(sha256 "$work/$asset")"
	[ "$actual" = "$expected" ] || die "checksum mismatch for $asset; nothing was installed"
	echo "Checksum verified."

	if command -v gh >/dev/null 2>&1; then
		gh attestation verify "$work/$asset" --repo "$repo" >/dev/null ||
			die "the provenance attestation of $asset does not verify; nothing was installed"
		echo "Provenance attestation verified."
	else
		echo "Note: the GitHub CLI (gh) is not installed, so only the checksum was verified, not the signed provenance."
	fi

	tar -xzf "$work/$asset" -C "$work" ai-dev || die "the archive has no ai-dev binary"
	[ -f "$work/ai-dev" ] && [ ! -L "$work/ai-dev" ] || die "the archive's ai-dev is not a regular file"
	"$work/ai-dev" version --format json 2>/dev/null | grep -q "\"version\":\"$version\"" ||
		die "the downloaded binary does not report $version"

	mkdir -p "$prefix"
	# Copy next to the target, then rename: the replacement is atomic.
	cp "$work/ai-dev" "$prefix/.ai-dev.new"
	chmod 0755 "$prefix/.ai-dev.new"
	mv -f "$prefix/.ai-dev.new" "$prefix/ai-dev"
	echo "Installed $prefix/ai-dev"
	"$prefix/ai-dev" version

	case ":$PATH:" in
	*":$prefix:"*) ;;
	*) printf '\n%s is not on your PATH. Add it, for example:\n  export PATH="%s:$PATH"\n' "$prefix" "$prefix" ;;
	esac
	printf '\nShell completion (optional): eval "$(ai-dev completion zsh)" or eval "$(ai-dev completion bash)"\n'
	echo "Git (2.45 or later) is needed only for ai-dev init and the repository checks."
}

main() {
	version=""
	prefix="$HOME/.local/bin"
	action=install
	while [ $# -gt 0 ]; do
		case "$1" in
		--version) [ $# -ge 2 ] || die "--version needs vX.Y.Z"; version="$2"; shift 2 ;;
		--prefix) [ $# -ge 2 ] || die "--prefix needs a directory"; prefix="$2"; shift 2 ;;
		--uninstall) action=uninstall; shift ;;
		-h | --help) sed -n '2,10p' "$0" 2>/dev/null || true; exit 0 ;;
		*) die "unknown option: $1" ;;
		esac
	done
	case "$action" in
	install) install_release ;;
	uninstall) uninstall ;;
	esac
}

main "$@"
