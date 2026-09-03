#!/bin/bash
# Formats, type-checks and runs every example against the library it links.
#
# Each example is a project of its own reaching `src/` as a path dependency, so
# a change to a library signature compiles here and nowhere else until the
# examples are next run. This is what says so at the same time as the rest of
# the gate. An example writes to a temporary directory it removes on the way
# out, so running them leaves nothing behind.
set -euo pipefail
cd "$(dirname "$0")/.."

for project in examples/*/; do
  project=${project%/}
  [ -f "$project/project.toml" ] || continue
  name=$(basename "$project")
  ( cd "$project" && gos fmt --check >/dev/null ) || { echo "$name is not formatted"; exit 1; }
  ( cd "$project" && gos check >/dev/null ) || { echo "$name does not check"; exit 1; }
  ( cd "$project" && gos run >/dev/null ) || { echo "$name does not run"; exit 1; }
  echo "$name: ok"
done
