# terndb 0.1.0

[![CI](https://github.com/danpozmanter/terndb/actions/workflows/ci.yml/badge.svg)](https://github.com/danpozmanter/terndb/actions/workflows/ci.yml)

An embedded structured key-value store with a light SQL front, in pure
Gossamer. Append-only log, in-memory index, and it runs inside the process that
uses it: there is no server, no socket and no wire protocol.

```sql
CREATE TABLE users (name TEXT, email TEXT, score FLOAT, active BOOL)
CREATE INDEX ON users (name)
INSERT INTO users VALUES ('ada', 'ada@example.com', 1.5, true)
SELECT name, score FROM users WHERE name = 'ada'
```

## Usage

### Embedded

```gossamer
let mut db = engine::open("data", false)?
engine::exec_args(&mut db, "INSERT INTO users VALUES (?, ?, ?, ?)",
    #[codec::v_text(name), codec::v_text(email), codec::v_f64(score), codec::v_bool(true)])?
let mut plan = engine::plan::prepare(&mut db, "SELECT * FROM users WHERE rowid = ?")?
let row = engine::plan::read_json(&mut db, &mut plan, 1)?
```

### From a terminal

The binary is the same library with a command line over it, for looking at a
store the program that owns it is not holding open.

```sh
gos build --release                       # -> target/release/terndb

terndb exec  <dir> "<sql>"...               # run statements, print rows
terndb shell <dir>                          # one statement per line, from stdin
terndb bench <dir> [keys] [size]            # reads against fs::read of the same bytes
```

## Documentation

- **[docs/sql.md](docs/sql.md)** - create, read, update and delete with SQL,
  embedded and from the command line.
- **[docs/kv.md](docs/kv.md)** - fetching a row by its id with no statement
  layer above it: `cache::row_json` and `rows::read`.
- **[examples/](examples)** - three programs that run: CRUD through the
  statement layer, reads four ways, and one row by its identity.

The two read what each other writes. SQL is how data changes; the key-value path
is how a row already identified is fetched.

## Grammar

`CREATE TABLE` · `CREATE INDEX` · `INSERT` · `SELECT` (`*`, columns, `COUNT(*)`,
`WHERE`, `ORDER BY`, `LIMIT`) · `UPDATE` · `DELETE` · `COMPACT`

One table per statement. `WHERE` is an `AND` of `=` `!=` `<` `>` `<=` `>=`.
A `?` stands anywhere a value does, filled from the arguments a caller bound.
Types: `TEXT` `BOOL` `INT8`..`INT64` `FLOAT32` `FLOAT64` `DATETIME`.
No joins, no `OR`, no subqueries, no multi-statement transactions.

## How it works

An append-only log of `[crc][kind][klen][vlen][key][value]` records, one
in-memory index, and a positional read per lookup. Rows live under
`<table>/r/<rowid>` and are addressed by rowid in a dense `Vec` - three words a
row - so a keyed read builds no key and hashes nothing. Sealed generations carry
hint files, so a restart reads keys without touching values. A secondary index
is declared in the store and rebuilt from the rows at every open, so nothing on
disk can go stale. A commit whose log is half superseded merges the generations
that are mostly dead, leaving the rest where they are.

Every statement is a transaction: atomic (a batch commits or leaves nothing),
consistent (types checked before a byte is written, checksums verified on the
way out), isolated (one writer, held by an exclusive lock on the directory),
durable (`sync_all` and a directory barrier before success).

One writer, many readers: the writer holds an exclusive lock on the directory
for as long as it is open, and a read-only engine takes none and answers from
the store as it stood when it was opened. A fault the store cannot read past is
reported rather than answered as an absence.

