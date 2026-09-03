#!/usr/bin/env bash
# Builds every server, seeds one dataset into each store, then drives the same
# concurrent GET load at each in turn and writes results.json.
#
# Every target is the store embedded in the web server's own process, which is
# the only deployment terndb has.
set -uo pipefail
cd "$(dirname "$0")"

ROWS="${ROWS:-50000}"
WORKERS="${WORKERS:-8}"
SECONDS_PER="${SECONDS_PER:-5}"
WARMUP="${WARMUP:-2}"
# Seconds a single pre-load probe may take before the target is called unresponsive.
PROBE_TIMEOUT="${PROBE_TIMEOUT:-10}"
ONLY="${ONLY:-}"
DATA="${DATA:-$PWD/data}"
BIN="$PWD/bin"
TERNDB="$PWD/../target/release/terndb"
# Where the run is written. Overridable so a smoke run can leave the recorded
# results alone, the way DATA leaves the seeded stores alone.
RESULTS="${RESULTS:-$PWD/results.json}"

mkdir -p "$DATA" "$BIN" logs
: > logs/build.log

say() { printf '\033[1m==> %s\033[0m\n' "$*"; }
wanted() { [ -z "$ONLY" ] && return 0; case ",$ONLY," in *,"$1",*) return 0;; *) return 1;; esac; }

# ----------------------------------------------------------------- sync ----
# Each bench server is a project of its own with its own `main.gos` over the
# same store library, and reaches that library as hard links to the files under
# `src/`. Re-linking here is what keeps that true: an editor that writes through
# a rename leaves a copy behind, and a copy drifts silently.
#
# The library is a tree of module directories, so the links mirror it; the
# entry file and the parts a server has no use for are what it leaves behind.
link_library() {
  dest="$1"
  for module in $(cd ../src && ls -d */ 2>/dev/null | tr -d /); do
    rm -rf "${dest:?}/$module"
  done
  rm -f "$dest"/*.gos.lib
  for rel in $(cd ../src && find . -name '*.gos' ! -name 'main.gos' ! -name 'cli.gos' \
      ! -name 'bench.gos' ! -name 'integration.gos' | sed 's|^\./||'); do
    mkdir -p "$dest/$(dirname "$rel")"
    ln -f "../src/$rel" "$dest/$rel" || return 1
  done
}

say "linking library sources"
for server in gos-sql gos-kv; do
  link_library "servers/$server/src" || exit 1
done

# ---------------------------------------------------------------- build ----
say "building"
( cd .. && gos build --release ) >> logs/build.log 2>&1 || { echo "terndb release build failed; see logs/build.log"; exit 1; }
( cd servers/gos-sql && gos build --release ) >> logs/build.log 2>&1 || { echo "servers/gos-sql build failed"; exit 1; }
( cd servers/gos-kv  && gos build --release ) >> logs/build.log 2>&1 || { echo "servers/gos-kv build failed"; exit 1; }
( cd servers/rust-sqlite && cargo build --release ) >> logs/build.log 2>&1 || { echo "servers/rust-sqlite build failed"; exit 1; }
( cd servers/rust-redb   && cargo build --release ) >> logs/build.log 2>&1 || { echo "servers/rust-redb build failed"; exit 1; }
( cd servers/go-sqlite   && go build -o "$BIN/go-sqlite" . ) >> logs/build.log 2>&1 || { echo "servers/go-sqlite build failed"; exit 1; }
( cd loadgen && go build -o "$BIN/loadgen" . ) >> logs/build.log 2>&1 || { echo "loadgen build failed"; exit 1; }

# ----------------------------------------------------------------- seed ----
say "seeding $ROWS rows"
uv run seed/seed_sqlite.py "$DATA/bench.sqlite" "$ROWS" || exit 1
uv run seed/seed_terndb.py "$TERNDB" "$DATA/bench.terndb" "$ROWS" || exit 1
servers/rust-redb/target/release/seed "$DATA/bench.redb" "$ROWS" || exit 1

# ------------------------------------------------------------ harness ------
PID=""
# A launcher (`uv run`, a shell wrapper) execs the real server as a child, so
# the tree is signalled, not just the pid the shell captured - a survivor keeps
# the port bound and the next target never becomes healthy.
kill_tree() {
  local pid="$1" kid
  [ -n "$pid" ] || return 0
  for kid in $(pgrep -P "$pid" 2>/dev/null); do kill_tree "$kid"; done
  kill "$pid" 2>/dev/null
}
stop() {
  kill_tree "$PID"; [ -n "$PID" ] && wait "$PID" 2>/dev/null
  PID=""
}
trap stop EXIT

await() {
  for _ in $(seq 1 600); do
    curl -fsS -m "$PROBE_TIMEOUT" "http://127.0.0.1:$1/health" >/dev/null 2>&1 && return 0
    sleep 0.2
  done
  return 1
}

# Peak resident set of a process and everything it spawned, in MiB. VmHWM is
# the kernel's own high-water mark since exec, so the stack is measured while
# it is still up rather than sampled on a timer that could miss the peak.
descendants() {
  local pid="$1" kid
  printf '%s\n' "$pid"
  for kid in $(pgrep -P "$pid" 2>/dev/null); do descendants "$kid"; done
}
peak_rss_mb() {
  local total=0 root proc kb
  for root in "$@"; do
    [ -n "$root" ] || continue
    for proc in $(descendants "$root"); do
      kb=$(awk '/^VmHWM:/ {print $2}' "/proc/$proc/status" 2>/dev/null || true)
      [ -n "$kb" ] && total=$((total + kb))
    done
  done
  awk -v kb="$total" 'BEGIN { printf "%.1f", kb / 1024 }'
}

# Resident set of a process tree right now, in KiB. Sampled either side of the
# measured load, the difference is what a request left behind.
rss_now_kb() {
  local total=0 root proc kb
  for root in "$@"; do
    [ -n "$root" ] || continue
    for proc in $(descendants "$root"); do
      kb=$(awk '/^VmRSS:/ {print $2}' "/proc/$proc/status" 2>/dev/null || true)
      [ -n "$kb" ] && total=$((total + kb))
    done
  done
  printf '%s' "$total"
}

# CPU a process tree has used, in clock ticks. The difference across the load,
# divided by the requests served, is what one request costs - which is the
# comparison the throughput column cannot make, because at these rates the load
# generator saturates before the servers do.
cpu_ticks() {
  local total=0 root proc t
  for root in "$@"; do
    [ -n "$root" ] || continue
    for proc in $(descendants "$root"); do
      t=$(awk '{print $14 + $15}' "/proc/$proc/stat" 2>/dev/null || true)
      [ -n "$t" ] && total=$((total + t))
    done
  done
  printf '%s' "$total"
}

CLK_TCK=$(getconf CLK_TCK 2>/dev/null || echo 100)

# A request that is still leaving more than this many bytes behind in the second
# measured window is leaking, and the numbers beside it describe a process that
# cannot serve for long. Two windows are what separates that from a cache
# filling: a cache's growth falls away once it is warm, a leak's does not.
LEAK_LIMIT_BYTES="${LEAK_LIMIT_BYTES:-8}"

FIRST=1
: > "$RESULTS.parts"

measure() { # name port
  local name="$1" port="$2"
  if ! await "$port"; then
    echo "  $name: never became healthy, see logs/$name.log"
    stop; return
  fi
  # Two checks before the load, both bounded: a wrong answer makes a fast number
  # meaningless, and a probe that never returns would otherwise stall the whole
  # run with no result and no diagnostic.
  local got want
  got=$(curl -fsS -m "$PROBE_TIMEOUT" "http://127.0.0.1:$port/user/1")
  want='"name":"user-1"'
  case "$got" in *"$want"*) ;; *) echo "  $name: unexpected body for a row it holds: $got"; stop; return;; esac
  # The miss path is exercised before the load rather than during it: a server
  # that answers a hit and stalls on a miss is a pass on every request the
  # load generator sends.
  local miss
  miss=$(curl -sS -m "$PROBE_TIMEOUT" -o /dev/null -w '%{http_code}' \
      "http://127.0.0.1:$port/user/$((ROWS + 1))" 2>/dev/null)
  if [ "$miss" != "404" ]; then
    echo "  $name: a missing row answered '$miss', want 404 (see logs/$name.log)"
    stop; return
  fi
  # The warmup is its own run, and then the load runs twice: the first window
  # finishes filling whatever caches the stack keeps, and the second is the one
  # reported, so its CPU and its growth are a warm server's.
  "$BIN/loadgen" -url "http://127.0.0.1:$port" -name "$name" -workers "$WORKERS" \
      -seconds "$WARMUP" -warmup 0 -rows "$ROWS" > /dev/null
  local r0 r1 r2 t1 t2 first_requests
  r0=$(rss_now_kb "$PID")
  first_requests=$("$BIN/loadgen" -url "http://127.0.0.1:$port" -name "$name" -workers "$WORKERS" \
      -seconds "$SECONDS_PER" -warmup 0 -rows "$ROWS" | jq -r .requests)
  r1=$(rss_now_kb "$PID"); t1=$(cpu_ticks "$PID")
  "$BIN/loadgen" -url "http://127.0.0.1:$port" -name "$name" -workers "$WORKERS" \
      -seconds "$SECONDS_PER" -warmup 0 -rows "$ROWS" > "logs/$name.json"
  r2=$(rss_now_kb "$PID"); t2=$(cpu_ticks "$PID")
  # Read the footprint while the stack is still up: the server holds the store,
  # so one process is the whole deployment.
  local rss requests cpu_us growth warming
  rss=$(peak_rss_mb "$PID")
  requests=$(jq -r .requests "logs/$name.json")
  cpu_us=$(awk -v t=$((t2 - t1)) -v n="$requests" -v hz="$CLK_TCK" \
      'BEGIN { printf "%.2f", (n > 0) ? t * 1000000 / hz / n : 0 }')
  growth=$(awk -v kb=$((r2 - r1)) -v n="$requests" \
      'BEGIN { g = (n > 0) ? kb * 1024 / n : 0; if (g > -0.05 && g < 0.05) g = 0; printf "%.1f", g }')
  warming=$(awk -v kb=$((r1 - r0)) -v n="$first_requests" \
      'BEGIN { g = (n > 0) ? kb * 1024 / n : 0; if (g > -0.05 && g < 0.05) g = 0; printf "%.1f", g }')
  jq --argjson rss "$rss" --argjson cpu "$cpu_us" --argjson growth "$growth" --argjson warming "$warming" \
      '. + {rss_peak_mb: $rss, cpu_us_per_request: $cpu, rss_growth_bytes_per_request: $growth,
            rss_growth_bytes_per_request_first_window: $warming}' \
      "logs/$name.json" > "logs/$name.json.tmp" \
      && mv "logs/$name.json.tmp" "logs/$name.json"
  # A server that died mid-run leaves a result of zero requests and a wall of
  # connection errors. That is not a measurement, so it is recorded as a
  # failure rather than charted as if it were slow.
  local got_rps
  got_rps=$(jq -r .rps "logs/$name.json")
  if [ "$(jq -r '.requests == 0' "logs/$name.json")" = "true" ]; then
    jq --arg why "no successful requests; the server exited during the run" \
       '. + {failed: true, reason: $why}' "logs/$name.json" > "logs/$name.json.tmp" \
       && mv "logs/$name.json.tmp" "logs/$name.json"
    printf '  %-32s FAILED (server exited during the run, see logs/%s.log)\n' "$name" "$name"
  else
    local leak_mark=""
    if [ "$(awk -v g="$growth" -v lim="$LEAK_LIMIT_BYTES" 'BEGIN { print (g > lim) ? 1 : 0 }')" = "1" ]; then
      jq --arg why "resident set grows $growth bytes per request" \
         '. + {failed: true, reason: $why}' "logs/$name.json" > "logs/$name.json.tmp" \
         && mv "logs/$name.json.tmp" "logs/$name.json"
      leak_mark="  LEAKING"
    fi
    printf '  %-32s %10.0f req/s  %8.1f us cpu/req  %8.1f MiB%s\n' "$name" \
        "$got_rps" "$cpu_us" "$(jq -r .rss_peak_mb "logs/$name.json")" "$leak_mark"
  fi
  [ "$FIRST" = 1 ] || echo "," >> "$RESULTS.parts"
  cat "logs/$name.json" >> "$RESULTS.parts"
  FIRST=0
  stop
}

say "measuring: $WORKERS workers, ${SECONDS_PER}s each, $ROWS rows"
printf '  %-32s %16s  %19s  %12s\n' target "response rate" "CPU" memory

if wanted gos-terndb-embedded-sql; then
  ( servers/gos-sql/target/release/bench-sql 127.0.0.1:8085 "$DATA/bench.terndb" ) > logs/gos-terndb-embedded-sql.log 2>&1 & PID=$!
  measure gos-terndb-embedded-sql 8085
fi

if wanted gos-terndb-embedded-kv; then
  ( servers/gos-kv/target/release/bench-kv serve 127.0.0.1:8088 "$DATA/bench.terndb" ) > logs/gos-terndb-embedded-kv.log 2>&1 & PID=$!
  measure gos-terndb-embedded-kv 8088
fi

if wanted gos-terndb-embedded-kv-gos-run; then
  ( cd servers/gos-kv && gos run . serve 127.0.0.1:8089 "$DATA/bench.terndb" ) > logs/gos-terndb-embedded-kv-gos-run.log 2>&1 & PID=$!
  measure gos-terndb-embedded-kv-gos-run 8089
fi

if wanted gos-terndb-embedded-sql-gos-run; then
  ( cd servers/gos-sql && gos run . 127.0.0.1:8086 "$DATA/bench.terndb" ) > logs/gos-terndb-embedded-sql-gos-run.log 2>&1 & PID=$!
  measure gos-terndb-embedded-sql-gos-run 8086
fi

if wanted rust-sqlite-embedded-sql; then
  ( ADDR=127.0.0.1:8081 DB_PATH="$DATA/bench.sqlite" servers/rust-sqlite/target/release/rust-sqlite ) > logs/rust-sqlite-embedded-sql.log 2>&1 & PID=$!
  measure rust-sqlite-embedded-sql 8081
fi

if wanted rust-redb-embedded-kv; then
  ( ADDR=127.0.0.1:8082 DB_PATH="$DATA/bench.redb" servers/rust-redb/target/release/rust-redb ) > logs/rust-redb-embedded-kv.log 2>&1 & PID=$!
  measure rust-redb-embedded-kv 8082
fi

if wanted go-sqlite-embedded-sql; then
  ( ADDR=127.0.0.1:8083 DB_PATH="$DATA/bench.sqlite" "$BIN/go-sqlite" ) > logs/go-sqlite-embedded-sql.log 2>&1 & PID=$!
  measure go-sqlite-embedded-sql 8083
fi

if wanted python-sqlite-embedded-sql; then
  ( cd servers/python-fastapi && DB_PATH="$DATA/bench.sqlite" \
      uv run --with fastapi --with 'uvicorn[standard]' \
      uvicorn server:app --host 127.0.0.1 --port 8084 --log-level warning ) > logs/python-sqlite-embedded-sql.log 2>&1 & PID=$!
  measure python-sqlite-embedded-sql 8084
fi

# ---------------------------------------------------------------- write ----
{
  printf '{\n  "config": {"rows": %s, "workers": %s, "seconds": %s, "warmup": %s},\n' \
      "$ROWS" "$WORKERS" "$SECONDS_PER" "$WARMUP"
  printf '  "host": {"cores": %s, "kernel": "%s"},\n' "$(nproc)" "$(uname -sr)"
  printf '  "runs": [\n'
  cat "$RESULTS.parts"
  printf '\n  ]\n}\n'
} > "$RESULTS"
rm -f "$RESULTS.parts"
jq empty "$RESULTS" && say "wrote $RESULTS"
