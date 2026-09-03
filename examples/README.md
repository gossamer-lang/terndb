# Examples

Three programs, all in Gossamer, each a project of its own that reaches the
library as a path dependency:

```toml
[dependencies]
"terndb.db/terndb" = { path = "../.." }
```

Run one from its own directory. With no argument it writes to a temporary
directory and removes it on the way out; give it a path and it leaves the store
behind for you to open again.

```sh
cd examples/crud && gos run
cd examples/crud && gos run . /tmp/users
```

| Example | What it shows |
|---|---|
| [crud](crud) | `CREATE TABLE`, `CREATE INDEX`, `INSERT`, `SELECT`, `UPDATE`, `DELETE`, `COMPACT`, and a reopen proving the writes are there |
| [sql-read](sql-read) | reads four ways: parsed each time, with bound values, prepared once, and cached by shape |
| [kv-read](kv-read) | one row by its identity: through the plan cache, through a prepared `WHERE rowid = ?`, and straight at the store |

`scripts/check-examples.sh` formats, checks and runs all three, and CI runs it
beside the tests.

## crud

Every statement terndb accepts is a transaction, and this walks the grammar in
the order a table is usually built: create it, index the column a `WHERE` will
name, insert both by writing values and by binding them, read, update, delete,
compact, and reopen the directory to show that what was reported durable is.

The bound `INSERT` writes a name spelling a SQL injection. It is stored and read
back as a name: a bound value is narrowed to its column's type the way a literal
is, and the text it carries never reaches the parser.

## sql-read

The same rows read four ways, so the difference between them is where the parse
and the plan happen and nothing else:

- `engine::query` / `engine::exec` parse and plan the statement each time. This
  is what the CLI does, and what a program running a statement once should do.
- `engine::query_args` fills a statement's `?` placeholders from values the
  caller supplies, in the order they were written.
- `engine::plan::prepare` settles the whole plan once - the table, the
  projection, the order, the limit, every comparison resolved to a column index
  - and `engine::plan::run` does the reading and nothing else.
- `engine::cache::plans` holds a per-caller cache, and `engine::cache::exec`
  normalises a statement to its shape, so three statements differing only in a
  literal share one plan.

It also shows what each layer refuses: a `run` whose argument count does not
match the plan's arity, and a `prepare` of a statement that is not a `SELECT`.

## kv-read

A row written by SQL lives under `<table>/r/<rowid>` in the storage layer, and
that key is reachable without a statement over it. The example reads the same
rows at three levels:

- `engine::cache::row_json` resolves the table once and renders every later row
  straight from the store.
- `engine::plan::prepare` on a `SELECT ... WHERE rowid = ?` is recognised as a
  keyed read: `read_json` renders, `read_row` fills a buffer the caller owns
  with typed values, and `read_bytes` hands back the encoded record. A statement
  that would need a walk is refused rather than quietly becoming one.
- `kv::rows` addresses a row by table id and rowid, and answers `Value`,
  `Absent` or `Fault`. The three are distinct: a store that holds a record and
  cannot produce it has not answered "not there".

It ends with `kv` used as a plain key-value store for data that is not a table
row. Rows and plain keys share one log, one index and one commit.
