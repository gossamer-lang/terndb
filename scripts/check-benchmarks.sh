#!/bin/bash
# Type-checks the benchmark programs against the library they link.
#
# Each benchmark project reaches `src/` as hard links and carries a `main.gos`
# of its own, so a change to a library signature compiles here and nowhere else
# until the benchmarks are next run. This is what says so at the same time as
# the rest of the gate.
set -euo pipefail
cd "$(dirname "$0")/.."

projects="benchmarks/servers/gos-sql benchmarks/servers/gos-kv benchmarks/embedded/gos"

for project in $projects; do
  # The module directories go first: a link left behind for a module the
  # library no longer has is a file nothing updates and everything compiles.
  for module in $(cd src && ls -d */ 2>/dev/null | tr -d /); do
    rm -rf "${project:?}/src/$module"
  done
  for rel in $(cd src && find . -name '*.gos' ! -name 'main.gos' ! -name 'cli.gos' \
      ! -name 'bench.gos' ! -name 'integration.gos' | sed 's|^\./||'); do
    mkdir -p "$project/src/$(dirname "$rel")"
    ln -f "src/$rel" "$project/src/$rel"
  done
done

for project in $projects; do
  ( cd "$project" && gos check >/dev/null ) || { echo "$project does not check"; exit 1; }
  echo "$project: ok"
done
