# Tern DB

[![CI](https://github.com/gossamer-lang/terndb/actions/workflows/ci.yml/badge.svg)](https://github.com/gossamer-lang/terndb/actions/workflows/ci.yml)

A small database that lives inside your program. You talk to it with simple
SQL, and it saves everything to a folder on disk. There is no separate server
to install or run. Written in [Gossamer](https://github.com/gossamer-lang/gossamer).

```sql
CREATE TABLE IF NOT EXISTS users (name TEXT, email TEXT, score FLOAT, active BOOL)
INSERT INTO users VALUES ('ada', 'ada@example.com', 1.5, true)
SELECT name, score FROM users WHERE name = 'ada'
```

## Use it from a program

Add it as a dependency in `project.toml`:

```toml
[dependencies]
"terndb.db/terndb" = { path = "path/to/terndb" }
```

```gossamer
use terndb::codec::Value
use terndb::engine

let mut db = engine::open("data")?
db.exec("CREATE TABLE IF NOT EXISTS users (name TEXT, email TEXT, score FLOAT, active BOOL)")?

// `?` placeholders keep user input out of the SQL text.
db.exec_args("INSERT INTO users VALUES (?, ?, ?, ?)", #[
    Value::text("ada")
    Value::text("ada@example.com")
    Value::float(1.5)
    Value::bool(true)
])?

let result = db.exec("SELECT name, score FROM users WHERE name = 'ada'")?
```

## Use it from a terminal

```sh
gos build --release                      # builds target/release/terndb

terndb exec  <dir> "<sql>"...            # run statements and print the results
terndb shell <dir>                       # type statements one per line
terndb bench <dir> [keys] [size]         # measure read speed (use an empty folder)
```

## What SQL it understands

- `CREATE TABLE`, `DROP TABLE`, `CREATE INDEX`, `DROP INDEX`
- `INSERT`, `UPDATE`, `DELETE`
- `SELECT` with `*` or named columns, `COUNT(*)`, `WHERE`, `ORDER BY`, `LIMIT`, `OFFSET`
- `COMPACT` to reclaim disk space

`WHERE` compares columns with `=`, `!=`, `<`, `>`, `<=`, `>=`, joined by `AND`.
Column types are `TEXT`, `BOOL`, `INT8`, `INT16`, `INT32`, `INT64`, `FLOAT32`,
`FLOAT64` and `DATETIME`. Common aliases such as `INT`, `FLOAT` and `VARCHAR`
also work.

Not supported: joins, `OR`, subqueries, or transactions spanning more than one
statement.

## Guarantees

- **Each statement is all-or-nothing.** It either fully saves or leaves no trace.
- **Saved means saved.** By default, a statement reports success only after its
  data is on disk.
- **Damage is detected.** Every record carries a checksum, and a record that
  fails it is reported as an error, never silently returned as missing.
- **One writer at a time.** A program writing to a folder locks it. Other
  programs can still open it read-only, and see the data as it was when they
  opened it.

## How it works

New data is always added to the end of a log file, never written over. An
in-memory index remembers where each row sits in that log, so reading a row
takes at most one disk read. Once half the log is old, replaced data, it is
compacted automatically.

## More

- [docs/sql.md](docs/sql.md) - the SQL guide, from a program and from the terminal
- [docs/kv.md](docs/kv.md) - reading a row directly by its id, without SQL
- [examples/](examples) - three runnable programs
- [benchmarks/](benchmarks) - performance measurements
