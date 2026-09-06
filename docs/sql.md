# CRUD with SQL

Every write terndb accepts is a SQL statement, and every statement is a
transaction: it commits whole or leaves nothing, its types are checked before a
byte is written, and `sync_all` returns before success does. One writer, many
readers - see [what a statement guarantees](#what-a-statement-guarantees).

Two surfaces run the same statements against the same store - the embedded
library and the CLI over it. The store lives in the process that opens it;
there is no server.

## The table

```sql
CREATE TABLE users (name TEXT, email TEXT, score FLOAT, active BOOL)
CREATE INDEX ON users (name)
```

`CREATE TABLE IF NOT EXISTS` and `CREATE INDEX IF NOT EXISTS` answer for a
table or an index already there instead of refusing, so a program may declare
its schema at every open. `DROP TABLE users` removes a table, its rows and its
index declarations in one commit, and `DROP INDEX ON users (name)` removes a
declaration and leaves the rows; both take `IF EXISTS`.

Types: `TEXT` `BOOL` `INT8` `INT16` `INT32` `INT64` `FLOAT32` `FLOAT64`
`DATETIME`. The usual SQL spellings alias onto them - `INT` and `INTEGER` are
`INT32`, `BIGINT` is `INT64`, `FLOAT` and `DOUBLE` are `FLOAT64`, `REAL` is
`FLOAT32`, `VARCHAR` and `STRING` are `TEXT`, `TIMESTAMP` is `DATETIME` - and
type names are case-insensitive, as keywords are.

Every table carries an implicit `rowid`, assigned in insertion order and never
reused: delete row 2 and the next insert still takes row 3. It is the primary
key, and it is what the key-value read path in [kv.md](kv.md) addresses a row
by. A `WHERE` may compare against it; a projection may not name it, because it
is the row's identity rather than one of its columns.

An index is declared in the store and rebuilt from the rows at every open, so
nothing on disk can go stale. Declare one for a column a `WHERE` names often:
`WHERE name = 'ada'` is a map lookup with an index and a table scan without one.

## Create, read, update, delete

```sql
INSERT INTO users VALUES ('ada', 'ada@example.com', 1.5, true)
INSERT INTO users VALUES ('grace', 'grace@example.com', 2.5, true), ('alan', 'alan@example.com', 3.5, false)
INSERT INTO users (name, email) VALUES ('edsger', 'edsger@example.com')

SELECT * FROM users
SELECT name, score FROM users WHERE name = 'ada'
SELECT COUNT(*) FROM users WHERE score > 2.0
SELECT name FROM users WHERE active = true ORDER BY score DESC LIMIT 10
SELECT name FROM users ORDER BY name LIMIT 10 OFFSET 20

UPDATE users SET score = 9.5 WHERE name = 'ada'

DELETE FROM users WHERE score < 2.0

COMPACT
```

An `INSERT` naming its columns supplies those and leaves every other column
`NULL`; one that names none supplies every column in declaration order.

One table per statement. `WHERE` is an `AND` of `=` `!=` (or `<>`) `<` `>` `<=`
`>=` against a column or against `rowid`. `LIMIT` takes an `OFFSET` beside it,
which skips that many matching rows first. There are no joins, no `OR`, no
subqueries and no multi-statement transactions.

`INSERT` answers the number of rows written, `UPDATE` and `DELETE` the number
they touched, `SELECT` its rows. `COMPACT` merges the log and drops what is
superseded; a commit whose log is half superseded already merges the generations
that are mostly dead, so the statement is for reclaiming space on demand rather
than for keeping the store healthy.

## From the command line

```sh
terndb exec /var/lib/users \
  "CREATE TABLE users (name TEXT, email TEXT, score FLOAT, active BOOL)" \
  "INSERT INTO users VALUES ('ada', 'ada@example.com', 1.5, true)" \
  "SELECT name, score FROM users WHERE name = 'ada'"
```

```
OK 0
OK 1
name|score
ada|1.5
```

A statement returning no rows prints `OK <n>`; one returning rows prints the
column names and then a pipe-separated line per row. A rejected statement goes
to stderr as `ERR <message>` and the statements beside it still run, so a
pipeline reading stdout sees only results. The exit code is 1 when any statement
was rejected, 2 when the command line itself was not one terndb takes.

`terndb shell <dir>` reads the same statements from stdin, one per line.

## Embedded

The store runs in the caller's own process and the library is one module:

```gossamer
use terndb::codec::Value
use terndb::engine

let mut db = engine::open("/var/lib/users")?

let created = db.exec_args("INSERT INTO users VALUES (?, ?, ?, ?)"
    #[Value::text(name), Value::text(email), Value::float(score), Value::bool(true)])?

let found = db.exec_args("SELECT name, score FROM users WHERE name = ?", #[Value::text(name)])?
println("{}", found.cols.join("|"))
for line in found.render() { println("{}", line) }

let updated = db.exec_args("UPDATE users SET score = ? WHERE name = ?"
    #[Value::float(9.5), Value::text(name)])?
let deleted = db.exec("DELETE FROM users WHERE score < 2.0")?

db.sync()?
```

`engine::open(dir)` answers an `Engine`; `engine::open(dir, read_only: true)`
answers a reader, which may not write. `exec` answers a `QueryResult` carrying
`cols`, `rows` and `count` - the rows an `INSERT` / `UPDATE` / `DELETE` touched
or a `COUNT(*)` answered. `result.render()` turns the rows into the
pipe-separated lines the CLI prints; `result.rows[i].vals` holds the typed
`codec::Value`s for a caller that wants them, and `{}` renders one as the CLI
prints it.

`db.table_names()` lists the tables. `db.sync()` flushes; `db.set_durable(false)`
trades the per-commit `sync_all` for speed on a store that can be rebuilt.

### Values a caller supplies

A `?` stands anywhere the grammar takes a value - the rows of an `INSERT`, the
assignments of an `UPDATE`, either side of a `WHERE` comparison - and
`db.exec_args(src, args)` fills the placeholders from `args` in the order they
were written.

```gossamer
let name = "ada'); DELETE FROM users; --"
let found = db.exec_args("SELECT score FROM users WHERE name = ?", #[Value::text(name)])?
```

That name is a name: a bound value is narrowed to its column's type the way a
literal is, and the text it carries never reaches the parser, so a statement
built once is the whole statement whatever a caller was handed. Building one by
pasting text together is what this replaces.

The arguments are `codec::Value`s - `Value::text`, `Value::bool`, `Value::int`,
`Value::float`, `Value::datetime`, `Value::nil`, and `Value::int8` through
`Value::int32` and `Value::float32` for the narrower widths - each narrowed to
the column it lands in, so a `DATETIME` column takes `Value::datetime` or the
text spelling a literal would carry, and an `INT32` refuses a value outside its
range. A statement whose placeholder count differs from `args` is refused before
it reads or writes, and so is a `?` reaching `exec`, which binds nothing.

### A statement run more than once

Parse and plan it once:

```gossamer
let mut plan = engine::plan::prepare(&mut db, "SELECT name, score FROM users WHERE score > ?")?
let res = plan.run(&mut db, #[Value::float(2.0)])?
```

A `?` is a placeholder filled from `args` in order. `prepare` takes reads;
writes are planned by the write itself, where the log append and the flush dwarf
the parse.

For a caller answering many requests, `engine::cache::Plans::new()` holds a
per-caller cache and `cache.exec(&mut db, src)` reuses the plan for a statement
whose shape it has seen. A cache belongs to whoever runs statements -
one worker, one request handler - so reaching a plan costs no lock.

## What a statement guarantees

- **Atomic.** A statement's records are appended and then a commit record; the
  index moves only once that commit is durable. A rebuild applies records as far
  as the last commit it saw, so a statement interrupted by a crash leaves
  nothing behind and its bytes are overwritten by the next append. A write that
  fails part-way rewinds the log, and a rewind that itself fails is reported
  beside the failure that asked for it.
- **Consistent.** Types, column counts and value ranges are checked before a
  byte is written, and a comparison against a value its column cannot hold is
  refused rather than quietly matching nothing. Every record carries a checksum
  over its key and value, which a read verifies: a record that fails it is
  reported, never answered as a row that is not there.
- **Isolated.** One writer, enforced by an exclusive lock on the directory for
  as long as the store is open, so a second process opening the same store for
  writing is refused. A statement's changes are staged rather than applied, so
  nothing observes one part-way, and a read-only engine - which takes no lock,
  and which any number of workers may open over one store - rebuilds to a commit
  boundary, which no statement straddles. It answers from the store as it stood
  when it was opened, so a worker that wants later writes opens it again.
- **Durable.** A commit is flushed with `sync_all` before it reports success,
  and a file the store creates, renames or removes is followed by a flush of the
  directory itself, so an entry naming a synced file is as durable as the file.
  `db.set_durable(false)` trades the per-commit flush for
  bulk-loading speed and keeps the other three; it is not how a store that
  answers requests is run.

The unit of atomicity is the statement. There are no multi-statement
transactions, no savepoints, and no `BEGIN`/`COMMIT` in the grammar.

A log that ends in a record it cannot verify is read up to its last good commit
and opened, since a store that recovers is more use than one that refuses to.
What was read past is not passed over: `db.faults()` lists it, and
the CLI prints it before it runs a statement.
