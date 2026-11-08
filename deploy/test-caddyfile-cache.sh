#!/bin/sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
caddyfile="$repo_root/deploy/Caddyfile"
active_config=$(sed 's/[[:space:]]*#.*$//' "$caddyfile")
normalized_config=$(printf '%s\n' "$active_config" | awk '
	NF > 0 {
		for (field = 1; field <= NF; field++) {
			token = $field
			sub(/^["`]/, "", token)
			sub(/["`]$/, "", token)
			printf "%s%s", (field == 1 ? "" : " "), token
		}
		print ""
	}
')

# Keep one canonical encode block instead of reimplementing Caddy matcher semantics.
expected_encode_block=$(cat <<'EOF'
encode {
zstd
gzip 6
minimum_length 256
match {
header Content-Type text/css*
header Content-Type text/csv*
header Content-Type text/html*
header Content-Type text/javascript*
header Content-Type text/markdown*
header Content-Type text/plain*
header Content-Type text/xml*
header Content-Type application/json*
header Content-Type application/javascript*
header Content-Type application/xml*
header Content-Type application/rss+xml*
header Content-Type image/svg+xml*
}
}
EOF
)

if printf '%s\n' "$normalized_config" | grep -Eiq 'cache-control.*immutable'; then
	echo "Caddyfile must not force immutable caching; the backend owns asset cache policy" >&2
	exit 1
fi

if ! printf '%s\n' "$normalized_config" | grep -Eq '^reverse_proxy localhost:8080([[:space:]]|$)'; then
	echo "Caddyfile must continue proxying all application routes to localhost:8080" >&2
	exit 1
fi

if printf '%s\n' "$normalized_config" | grep -Eq '^import([[:space:]]|$)'; then
	echo "Caddyfile must not import configuration outside this canonical policy check" >&2
	exit 1
fi

if printf '%s\n' "$normalized_config" | grep -Eiq '^flush_interval([[:space:]]|$)'; then
	echo "Caddyfile must leave flush_interval unset so SSE auto-flushing and client cancellation remain intact" >&2
	exit 1
fi

encode_directive_count=$(printf '%s\n' "$normalized_config" | awk '$1 == "encode" { count++ } END { print count + 0 }')
if [ "$encode_directive_count" -ne 1 ]; then
	echo "Caddyfile must contain exactly one explicit encode block" >&2
	exit 1
fi

actual_encode_block=$(printf '%s\n' "$normalized_config" | awk '
	$1 == "encode" { in_block = 1 }
	in_block {
		print
		for (field = 1; field <= NF; field++) {
			if ($field == "{") depth++
			if ($field == "}") depth--
		}
		if (depth == 0) exit
	}
')
if [ "$actual_encode_block" != "$expected_encode_block" ]; then
	echo "Caddyfile encode block must keep the canonical non-SSE compression policy" >&2
	exit 1
fi

echo "Caddyfile preserves backend cache policy, SSE streaming, and non-SSE compression"
