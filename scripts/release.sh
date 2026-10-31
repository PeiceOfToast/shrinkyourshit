# SPDX-License-Identifier: AGPL-3.0-only
#
# Release build and packaging (task 16). One place decides the release
# layout: .github/workflows/release.yml, `make release-cross` and
# TestReleaseArtifacts (internal/update) all call this script, so a tagged
# release and the local dry run produce the same artifact names.
#
# Subcommands:
#   build <version> [dist-dir]  cross compile all five targets, embedding
#                               version, commit and build date via ldflags.
#                               COMMIT and BUILD_DATE env vars override the
#                               git/clock defaults so the image gets the
#                               exact same identity (task 26).
#   package [dist-dir]          archive each binary, then write checksums.txt
#   checksums [dist-dir]        write checksums.txt over the archives present
#   verify [dist-dir]           re-hash every archive against checksums.txt
#
# Artifact names (STACK.md release section):
#   shrinkyourshit-<os>-<arch>          the binaries
#   shrinkyourshit-<os>-<arch>.tar.gz   unix archives
#   shrinkyourshit-<os>-<arch>.zip      the windows archive
#   checksums.txt                       sha256sum format: "<digest>  <name>"
#
# The windows zip is written with python3 (stdlib zipfile) because `zip`
# is not installed on every build machine; python3 is (CI uses it too).

set -eu

BINARY=shrinkyourshit
TARGETS="linux/amd64 linux/arm64 windows/amd64 darwin/amd64 darwin/arm64"

usage() {
	printf 'usage: release.sh build <version> [dist] | package [dist] | verify [dist]\n' >&2
	exit 2
}

# sha256 prints the hex digest of $1.
sha256() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | cut -d' ' -f1
	else
		shasum -a 256 "$1" | cut -d' ' -f1
	fi
}

# zip_one writes $2 (basename kept) into the zip archive $1.
zip_one() {
	python3 - "$1" "$2" <<'PYEOF'
import os, sys, zipfile
archive, path = sys.argv[1], sys.argv[2]
with zipfile.ZipFile(archive, "w", zipfile.ZIP_DEFLATED) as z:
    z.write(path, arcname=os.path.basename(path))
PYEOF
}

cmd_build() {
	[ $# -ge 1 ] || usage
	version="$1"
	dist="${2:-dist}"
	# COMMIT and BUILD_DATE can be injected by the workflow so the binaries
	# and the container image carry the exact same values (task 26).
	commit="${COMMIT:-$(git rev-parse HEAD 2>/dev/null || echo unknown)}"
	build_date="${BUILD_DATE:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"
	mkdir -p "$dist"
	for target in $TARGETS; do
		os="${target%/*}"
		arch="${target#*/}"
		ext=""
		[ "$os" = windows ] && ext=".exe"
		echo "== $os/$arch"
		CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" \
			go build -trimpath \
			-ldflags "-s -w -X main.version=$version -X main.commit=$commit -X main.date=$build_date" \
			-o "$dist/$BINARY-$os-$arch$ext" ./cmd/shrinkyourshit
	done
}

cmd_package() {
	dist="${1:-dist}"
	[ -d "$dist" ] || { printf 'release.sh: %s is not a directory\n' "$dist" >&2; exit 1; }
	# Archive every binary. tar.gz for unix targets, zip for windows.
	for f in "$dist/$BINARY-"*; do
		[ -f "$f" ] || { printf 'release.sh: no binaries in %s\n' "$dist" >&2; exit 1; }
		case "$f" in
			*.exe)
				zip_one "${f%.exe}.zip" "$f"
				;;
			*)
				tar -czf "$f.tar.gz" -C "$dist" "$(basename "$f")"
				;;
		esac
	done
	cmd_checksums "$dist"
}

cmd_checksums() {
	dist="${1:-dist}"
	# checksums.txt over every archive, sha256sum format (two spaces).
	{
		for a in "$dist"/*.tar.gz "$dist"/*.zip; do
			[ -f "$a" ] || continue
			printf '%s  %s\n' "$(sha256 "$a")" "$(basename "$a")"
		done
	} >"$dist/checksums.txt"
}

cmd_verify() {
	dist="${1:-dist}"
	list="$dist/checksums.txt"
	[ -f "$list" ] || { printf 'release.sh: %s missing\n' "$list" >&2; exit 1; }
	status=0
	while IFS= read -r line; do
		case "$line" in
			"" | "#"*) continue ;;
		esac
		digest="$(printf '%s' "$line" | cut -d' ' -f1)"
		name="$(printf '%s' "$line" | cut -d' ' -f3-)"
		f="$dist/$name"
		if [ ! -f "$f" ]; then
			printf 'release.sh: listed file missing: %s\n' "$name" >&2
			status=1
			continue
		fi
		actual="$(sha256 "$f")"
		if [ "$actual" != "$digest" ]; then
			printf 'release.sh: checksum mismatch: %s\n' "$name" >&2
			status=1
		fi
	done <"$list"
	# Every archive present must also be listed, or the updater could pick
	# an asset no checksum covers.
	for a in "$dist"/*.tar.gz "$dist"/*.zip; do
		[ -f "$a" ] || continue
		name="$(basename "$a")"
		if ! cut -d' ' -f3- "$list" | grep -qxF "$name"; then
			printf 'release.sh: archive not listed in checksums.txt: %s\n' "$name" >&2
			status=1
		fi
	done
	[ "$status" -eq 0 ] || exit "$status"
}

case "${1:-}" in
	build) shift; cmd_build "$@" ;;
	package) shift; cmd_package "$@" ;;
	checksums) shift; cmd_checksums "$@" ;;
	verify) shift; cmd_verify "$@" ;;
	*) usage ;;
esac
