# /// script
# requires-python = ">=3.11"
# dependencies = []
# ///
"""Seeds the shared dataset into an terndb directory.

Statements are fed to `terndb shell` in batches: one statement is one transaction,
so a row-at-a-time load would pay a flush per row.
"""

import shutil
import subprocess
import sys
from pathlib import Path

terndb, path, rows = sys.argv[1], sys.argv[2], int(sys.argv[3])
batch = 500

if Path(path).exists():
    shutil.rmtree(path)

lines = [
    "CREATE TABLE users (name TEXT, email TEXT, score FLOAT64, active BOOL)"
]
for start in range(1, rows + 1, batch):
    stop = min(start + batch, rows + 1)
    values = ", ".join(
        f"('user-{i}', 'user-{i}@example.com', 1.5, true)" for i in range(start, stop)
    )
    lines.append(f"INSERT INTO users VALUES {values}")
lines.append("COMPACT")
lines.append("QUIT")

proc = subprocess.run(
    [terndb, "shell", path],
    input="\n".join(lines) + "\n",
    text=True,
    capture_output=True,
)
if proc.returncode != 0:
    sys.stderr.write(proc.stdout + proc.stderr)
    sys.exit(proc.returncode)
bad = [l for l in proc.stdout.splitlines() if l.startswith("ERR")]
if bad:
    sys.stderr.write("\n".join(bad[:5]) + "\n")
    sys.exit(1)
print(f"seeded {rows} rows into {path}")
