# /// script
# requires-python = ">=3.11"
# dependencies = []
# ///
"""Seeds the shared dataset into a SQLite file used by three of the servers."""

import os
import sqlite3
import sys

path = sys.argv[1]
rows = int(sys.argv[2])
if os.path.exists(path):
    os.remove(path)
for suffix in ("-wal", "-shm"):
    if os.path.exists(path + suffix):
        os.remove(path + suffix)

conn = sqlite3.connect(path)
conn.execute("PRAGMA journal_mode=WAL")
conn.execute(
    "CREATE TABLE users ("
    "id INTEGER PRIMARY KEY, name TEXT NOT NULL, email TEXT NOT NULL,"
    " score REAL NOT NULL, active INTEGER NOT NULL)"
)
conn.executemany(
    "INSERT INTO users (id, name, email, score, active) VALUES (?, ?, ?, ?, ?)",
    ((i, f"user-{i}", f"user-{i}@example.com", 1.5, 1) for i in range(1, rows + 1)),
)
conn.commit()
conn.execute("ANALYZE")
conn.commit()
conn.close()
print(f"seeded {rows} rows into {path}")
