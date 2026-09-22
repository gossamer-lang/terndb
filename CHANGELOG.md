# Changelog

## 0.3.0

- Requires Gossamer 0.63.1.
- A text column renders through `String::push_json_quoted`, so a row's JSON escapes text exactly as `std::encoding::json` does: `0x08` and `0x0c` are written `\b` and `\f` rather than `\u0008` and `\u000c`, and `<`, `>`, `&`, U+2028, and U+2029 are written as `\u` escapes.
- A statement's records reach the log in one write at commit: a 2,000-row `INSERT` makes one write where it made 2,003.
- `ORDER BY rowid` is accepted, `ASC` or `DESC`, and costs no sort.
- `ORDER BY <col> LIMIT n` holds only the leading rows while it walks and fully decodes only the rows it returns; rows with equal values come back in row-id order, with or without a `LIMIT`.
- Table walks (a non-indexed `WHERE`, `ORDER BY`, `UPDATE` and `DELETE` with a filter, building an index) read the log in blocks instead of one read per row.
- `COMPACT` copies each live record as it lies, checked against its checksum, writes in large blocks, and lays each table out in row-id order.
- Hint files are written in sections with a trailing checksum, and a hint that fails it is replaced by reading the data file; hints written by 0.2.0 still open.
- Opening a store places rows from hints and from the log without spelling out their keys.
- Readers and writers hold as many sealed generations in memory as `Engine::set_resident_budget` allows (64 MiB by default), and every read path uses them.
- Values built with `Value::int8`, `int16`, `int32`, `float32` and `datetime` are accepted by columns of their own type, and any whole number is accepted as a row id.
- A writer cuts away a batch a crash left unfinished before it appends, so a later, shorter write no longer makes the next open report a damaged record.
- A writer opening a log that stops at a damaged record writes to a new generation and leaves the damaged one on disk.
- `terndb bench` also measures inserts, walks, ordered reads, `COMPACT` and reopening, and times each phase in nanoseconds so a fast phase no longer reads as 0.

## 0.2.0

- Requires Gossamer 0.58.13: a library method and a caller's free function of the same name shared one entry in the toolchain's parameter table, so a program with its own `fn run` or `fn get` received a reference where it declared a value.
- Statements run as methods on the values they belong to: `db.exec(sql)`, `db.exec_args(sql, args)`, `db.sync()`, `db.table_names()`, `db.faults()`, `db.set_durable(d)`, `plan.run(&mut db, args)`, `plan.read_json(&mut db, id)`, `cache.exec(&mut db, sql)`, `cache.row_json(&mut db, table, id)`, and `store.put` / `get` / `delete` / `contains` / `len` / `sync`.
- `exec` answers one `QueryResult` carrying `cols`, `rows` and `count`, so a caller binds one value rather than a pair; `query` and `query_args` are gone, and `render` is `result.render()`.
- `engine::open(dir)` and `kv::open(dir)` open for writing; a reader is `engine::open(dir, read_only: true)`.
- Values are built on the type they belong to: `Value::text`, `Value::bool`, `Value::int`, `Value::float`, `Value::datetime`, `Value::nil`, `Value::int8` / `int16` / `int32` / `float32`, replacing `codec::v_*`.
- A `Value` renders through `Display`, so `{}` and `to_string()` print what the CLI prints and `codec::value_to_string` is gone.
- `CREATE TABLE IF NOT EXISTS` and `CREATE INDEX IF NOT EXISTS` answer for what is already there, so a program may declare its schema at every open.
- `DROP TABLE [IF EXISTS] t` removes a table, its rows and its index declarations in one commit; `DROP INDEX [IF EXISTS] ON t (col)` removes a declaration and leaves the rows.
- `INSERT INTO t (col, ...) VALUES (...)` supplies the columns it names and leaves the rest NULL.
- `LIMIT` takes an `OFFSET` beside it, on a walk, an ordered read and a `COUNT(*)` alike.
- `<>` is accepted wherever `!=` is.

## 0.1.0 - Initial release

- Embedded structured key-value store with a light SQL front, in pure Gossamer: it runs in the process that opens it, with no server, socket or wire protocol.
- `CREATE TABLE` / `CREATE INDEX` / `INSERT` / `SELECT` / `UPDATE` / `DELETE` / `COMPACT`, one table per statement.
- `SELECT` takes `*`, named columns, `COUNT(*)`, `WHERE`, `ORDER BY` and `LIMIT`; `WHERE` is an `AND` of `=` `!=` `<` `>` `<=` `>=`.
- Types `TEXT` `BOOL` `INT8`..`INT64` `FLOAT32` `FLOAT64` `DATETIME`, with the usual SQL spellings as aliases.
- Every statement is a transaction: atomic, type-checked before a byte is written, one writer, `sync_all` before success.
- One writer per directory, held by an exclusive lock the writer takes at open; a read-only engine takes none, so any number of workers read one store while it is written.
- A file the store creates, renames or removes is followed by a flush of the directory itself, so an entry naming a synced file is as durable as the file.
- Writes go above every sealed generation, so a crash part-way through a merge cannot leave the active file below a seal that would be replayed over it.
- A record that fails its checksum is reported rather than answered as a row that is not there, in the statement layer, the keyed read and compaction alike: `kv::get` and `kv::rows::read` answer `kv::Read`, whose arms are the value, the absence of one, and the fault that is neither.
- A comparison against a value its column cannot hold is refused instead of matching nothing.
- A log read past a damaged record is reported: `engine::faults` lists what the open had to skip, and the CLI prints it before it runs a statement.
- A hint file that stops short of describing its generation falls back to the data file rather than opening a store with keys missing.
- Append-only log with an in-memory index, hint files on sealed generations, and compaction of the generations that are mostly dead.
- Secondary indexes declared in the store and rebuilt from the rows at every open.
- Keyed reads that skip the statement layer: `engine::cache::row_json` and `kv::rows::read`.
- Bound values: a `?` stands anywhere a value does, filled from the arguments given to `engine::exec_args` / `engine::query_args`, so a caller never builds a statement out of text it was handed.
- Prepared statements, and a per-caller plan cache.
- CLI over the same library: `exec`, `shell`, `bench`.
- `examples/`: three runnable programs reaching the library as a path dependency - CRUD through the statement layer, reads parsed, bound, prepared and cached, and one row by its identity through the cache, a prepared statement and the store.
- Runs identically on the bytecode VM, the Cranelift JIT and LLVM AOT.
