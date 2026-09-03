#!/bin/bash
# Runs the demo transcript on all three tiers and fails if any two differ.
set -euo pipefail
cd "$(dirname "$0")/.."
gos build >/dev/null
gos build --release >/dev/null
out=$(mktemp -d)
trap 'rm -rf "$out"' EXIT
./scripts/demo.sh gos run .              > "$out/vm.txt"
./scripts/demo.sh ./target/debug/terndb    > "$out/cranelift.txt"
./scripts/demo.sh ./target/release/terndb  > "$out/llvm.txt"
diff "$out/vm.txt" "$out/cranelift.txt"
diff "$out/vm.txt" "$out/llvm.txt"
echo "tier parity: VM, Cranelift and LLVM agree"

# The examples reach the library as callers rather than through the CLI, so
# they cover the surface the demo transcript does not: prepared plans, the plan
# cache, and the keyed read. Each writes to a temporary directory of its own and
# prints no path, so its transcript is the same on every tier or the tiers
# disagree.
for project in examples/*/; do
  project=${project%/}
  [ -f "$project/project.toml" ] || continue
  name=$(basename "$project")
  ( cd "$project" && gos build >/dev/null && gos build --release >/dev/null )
  ( cd "$project" && gos run ) > "$out/$name-vm.txt"
  "$project/target/debug/example-$name" > "$out/$name-cranelift.txt"
  "$project/target/release/example-$name" > "$out/$name-llvm.txt"
  diff "$out/$name-vm.txt" "$out/$name-cranelift.txt"
  diff "$out/$name-vm.txt" "$out/$name-llvm.txt"
  echo "tier parity: $name agrees on all three tiers"
done
