#!/usr/bin/env bash
# Queries per second with the store in the caller's own process and nothing
# above it: no socket, no HTTP, no load generator. One thread asks for a row by
# primary key and renders it as JSON, for a fixed window, against the same
# seeded dataset run.sh builds. Writes embedded.json.
set -uo pipefail
cd "$(dirname "$0")"

ROWS="${ROWS:-50000}"
SECONDS_PER="${SECONDS_PER:-5}"
ONLY="${ONLY:-}"
DATA="${DATA:-$PWD/data}"
BIN="$PWD/bin"
# Overridable for the same reason DATA is: a smoke run writes somewhere else.
RESULTS="${RESULTS:-$PWD/embedded.json}"

mkdir -p "$DATA" "$BIN" logs
: > logs/embedded-build.log

# Peak memory comes from the kernel's own accounting for the finished child,
# which GNU time reports and the shell has no way to ask for.
[ -x /usr/bin/time ] || { echo "/usr/bin/time is needed for the memory column (apt install time)"; exit 1; }

say() { printf '\033[1m==> %s\033[0m\n' "$*"; }
wanted() { [ -z "$ONLY" ] && return 0; case ",$ONLY," in *,"$1",*) return 0;; *) return 1;; esac; }

# ----------------------------------------------------------------- sync ----
# The Gossamer benchmark is a project of its own over the same store library,
# and reaches it as hard links to the files under `src/`, exactly as the servers
# do. Re-linking is what keeps that true across an editor that writes through a
# rename.
#
# The library is a tree of module directories, so the links mirror it; the entry
# file and the parts the benchmark has no use for are what it leaves behind.
say "linking library sources"
for module in $(cd ../src && ls -d */ 2>/dev/null | tr -d /); do
  rm -rf "embedded/gos/src/$module"
done
for rel in $(cd ../src && find . -name '*.gos' ! -name 'main.gos' ! -name 'cli.gos' \
    ! -name 'bench.gos' ! -name 'integration.gos' | sed 's|^\./||'); do
  mkdir -p "embedded/gos/src/$(dirname "$rel")"
  ln -f "../src/$rel" "embedded/gos/src/$rel" || exit 1
done

# ---------------------------------------------------------------- build ----
say "building"
( cd embedded/gos && gos build --release ) >> logs/embedded-build.log 2>&1 || { echo "embedded/gos build failed"; exit 1; }
( cd servers/rust-sqlite && cargo build --release --bin embedded ) >> logs/embedded-build.log 2>&1 || { echo "servers/rust-sqlite embedded build failed"; exit 1; }
( cd servers/rust-redb && cargo build --release --bins ) >> logs/embedded-build.log 2>&1 || { echo "servers/rust-redb embedded build failed"; exit 1; }
( cd servers/go-sqlite && go build -o "$BIN/go-embedded" ./cmd/embedded ) >> logs/embedded-build.log 2>&1 || { echo "servers/go-sqlite embedded build failed"; exit 1; }

# ----------------------------------------------------------------- data ----
# One benchmark seeds the stores and this one reads them, so there is a single
# definition of what the dataset is: `run.sh` writes it, both measure it, and
# neither can drift from the other's idea of the rows.
for store in bench.sqlite bench.redb bench.terndb; do
  [ -e "$DATA/$store" ] && continue
  echo "$DATA/$store is missing - run ./run.sh first, which seeds the dataset both benchmarks read"
  exit 1
done

# ------------------------------------------------------------- measure -----
FIRST=1
: > "$RESULTS.parts"

# Peak resident set of the target, in MiB. The store is linked into the process
# that queries it, so the whole process is what the store costs to run; the
# kernel reports that high-water mark to the parent through wait4, which is what
# `/usr/bin/time -f %M` reads.
RSS_FILE="$PWD/logs/embedded-rss.txt"

measure() { # name command...
  local name="$1"; shift
  local out rss
  : > "$RSS_FILE"
  out=$(/usr/bin/time -f '%M' -o "$RSS_FILE" "$@" 2>> "logs/embedded-$name.log")
  rss=$(awk 'END { printf "%.1f", $1 / 1024 }' "$RSS_FILE")
  if [ -z "$out" ] || ! printf '%s' "$out" | jq empty 2>/dev/null; then
    printf '  %-32s FAILED (see logs/embedded-%s.log)\n' "$name" "$name"
    [ "$FIRST" = 1 ] || echo "," >> "$RESULTS.parts"
    jq -nc --arg t "$name" '{target: $t, failed: true, reason: "the target produced no measurement"}' >> "$RESULTS.parts"
    FIRST=0
    return
  fi
  # The id is what results, logs and charts are keyed by, whatever label the
  # binary prints for itself.
  out=$(printf '%s' "$out" | jq -c --arg t "$name" --argjson rss "$rss" '. + {target: $t, rss_peak_mb: $rss}')
  printf '  %-32s %14s queries/s  %8.1f MiB\n' "$name" \
      "$(printf '%s' "$out" | jq -r '.qps | floor')" "$rss"
  [ "$FIRST" = 1 ] || echo "," >> "$RESULTS.parts"
  printf '%s' "$out" >> "$RESULTS.parts"
  FIRST=0
}

say "measuring: one thread, ${SECONDS_PER}s each, $ROWS rows"
printf '  %-32s %24s  %12s\n' target queries memory

wanted gos-terndb-embedded-sql && measure gos-terndb-embedded-sql \
    embedded/gos/target/release/bench-embedded sql "$DATA/bench.terndb" "$ROWS" "$SECONDS_PER"
wanted gos-terndb-embedded-kv && measure gos-terndb-embedded-kv \
    embedded/gos/target/release/bench-embedded kv "$DATA/bench.terndb" "$ROWS" "$SECONDS_PER"
wanted rust-sqlite-embedded-sql && measure rust-sqlite-embedded-sql \
    servers/rust-sqlite/target/release/embedded "$DATA/bench.sqlite" "$ROWS" "$SECONDS_PER"
wanted rust-redb-embedded-kv && measure rust-redb-embedded-kv \
    servers/rust-redb/target/release/embedded "$DATA/bench.redb" "$ROWS" "$SECONDS_PER"
# Runs after the plain redb target because it seeds a second table into the same
# file: the plain one is then measured against the store it was seeded with.
wanted rust-redb-embedded-kv-typed && measure rust-redb-embedded-kv-typed \
    servers/rust-redb/target/release/embedded_typed "$DATA/bench.redb" "$ROWS" "$SECONDS_PER"
wanted go-sqlite-embedded-sql && measure go-sqlite-embedded-sql \
    "$BIN/go-embedded" "$DATA/bench.sqlite" "$ROWS" "$SECONDS_PER"

# ---------------------------------------------------------------- write ----
{
  printf '{\n  "config": {"rows": %s, "seconds": %s, "threads": 1},\n' "$ROWS" "$SECONDS_PER"
  printf '  "host": {"cores": %s, "kernel": "%s"},\n' "$(nproc)" "$(uname -sr)"
  printf '  "runs": [\n'
  cat "$RESULTS.parts"
  printf '\n  ]\n}\n'
} > "$RESULTS"
rm -f "$RESULTS.parts"
jq empty "$RESULTS" && say "wrote $RESULTS"
