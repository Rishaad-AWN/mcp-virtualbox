#!/bin/sh
# Keep the version in sync across every file that hard-codes it.
#
#   scripts/version.sh check <version>   # exit non-zero if any file disagrees
#   scripts/version.sh set   <version>   # rewrite all files to <version>
#
# Files: main.go (var version), .claude-plugin/plugin.json (version),
# .claude-plugin/marketplace.json (metadata.version + plugins[0].version).
set -eu

cmd="${1:-}"
ver="${2:-}"
if [ -z "$cmd" ] || [ -z "$ver" ]; then
	echo "usage: $0 check|set <version>" >&2
	exit 2
fi
ver="${ver#v}" # tolerate a leading v (e.g. from a git tag)

root=$(cd -- "$(dirname -- "$0")/.." && pwd)
main="$root/main.go"
plugin="$root/.claude-plugin/plugin.json"
market="$root/.claude-plugin/marketplace.json"

main_version() { sed -n 's/.*var version = "\([^"]*\)".*/\1/p' "$main"; }
json_versions() { sed -n 's/.*"version": *"\([^"]*\)".*/\1/p' "$1"; }

case "$cmd" in
check)
	rc=0
	check_one() {
		got="$1"
		where="$2"
		if [ "$got" != "$ver" ]; then
			echo "version mismatch in $where: found '$got', expected '$ver'" >&2
			rc=1
		fi
	}
	check_one "$(main_version)" "main.go"
	# `for` over command substitution runs in this shell, so rc propagates
	# (a piped `while read` would not).
	for v in $(json_versions "$plugin"); do check_one "$v" ".claude-plugin/plugin.json"; done
	for v in $(json_versions "$market"); do check_one "$v" ".claude-plugin/marketplace.json"; done
	if [ "$rc" -eq 0 ]; then
		echo "version $ver is consistent across main.go, plugin.json, marketplace.json"
	fi
	exit "$rc"
	;;
set)
	sed -i "s/var version = \"[^\"]*\"/var version = \"$ver\"/" "$main"
	sed -i "s/\"version\": *\"[^\"]*\"/\"version\": \"$ver\"/g" "$plugin" "$market"
	echo "set version to $ver in main.go, plugin.json, marketplace.json"
	;;
*)
	echo "unknown command: $cmd (use check|set)" >&2
	exit 2
	;;
esac
