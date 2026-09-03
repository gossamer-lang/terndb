# Changelog

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
