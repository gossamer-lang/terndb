# Retrieval through the key-value path

A row written by SQL lives under `<table>/r/<rowid>` in the storage layer, and
that key is reachable without a statement above it. The key-value path answers
one row by its identity: no parse, no plan, no prepared-statement lookup, no
result set built around the answer.

It reads what SQL writes. There is one store, one copy of every row, and the two
front ends differ by the statement layer and by nothing below it. Use SQL to
change data ([sql.md](sql.md)); use this to fetch a row you already have the id
of, which is what a request handler usually has.

## One table, read repeatedly

`engine::cache::plans()` holds a cache; `cache::row_json` resolves the table
once and reads every later row straight from the store.

```gossamer
let mut db = engine::open("/var/lib/users", true)?
let mut cache = engine::cache::plans()

let row = engine::cache::row_json(&mut db, &mut cache, "users", 1)?
// {"id":1,"name":"ada","email":"ada@example.com","score":9.5,"active":true}

if row.len() == 0 { /* no such row */ }
```

An empty string is the absent row. A rendered row always begins with `{`, so
the empty answer names the absent row and nothing else.

### Through a prepared statement

A `SELECT ... WHERE rowid = ?` is recognised as a keyed read and takes the same
path, with the projection the statement asks for:

```gossamer
let mut plan = engine::plan::prepare(&mut db, "SELECT * FROM users WHERE rowid = ?")?

let json = engine::plan::read_json(&mut db, &mut plan, 1)?
```

`plan::read_json` renders; `plan::read_row` hands back the typed values instead, into a
buffer the caller owns and reuses:

```gossamer
let mut vals: Vec<codec::Value> = #[]
if engine::plan::read_row(&mut db, &mut plan, 1, &mut vals)? {
    println("{}", codec::value_to_string(vals[0]))   // ada
}
```

`engine::plan::read_bytes(&mut db, &mut plan, id)` answers the encoded record for a
caller that renders it itself, as a `kv::Read`. All three take a statement a
single read can answer, and nothing else: `plan::read_row` and `plan::read_json` reject
one that would need a walk rather than quietly turning into a scan.

### Straight at the store

Below the engine, `kv` addresses a row by table id and rowid:

```gossamer
let tid = kv::rows::table_id(&mut db.store, "users")
match kv::rows::read(&mut db.store, tid, 1) {
    kv::Read::Value(bytes) => { /* the encoded row */ }
    kv::Read::Absent => { /* not there */ }
    kv::Read::Fault(why) => { /* the store holds it and cannot produce it */ }
}

kv::rows::count(&mut db.store, tid)      // live rows
kv::rows::holds(&mut db.store, tid, 1)   // without reading the value
kv::rows::last_id(&mut db.store, tid)    // the highest id ever assigned
```

A read answers `Absent` for a row the store does not hold, and `Fault` for one it
holds and cannot produce: a record that fails its checksum, or a generation file
that will not open, is reported rather than answered as an absence. The same
distinction runs through the layers above - `engine::cache::row_json` and a
`SELECT` report it too, so a damaged row never reads as a row that was never
written.

`kv` is also a key-value store in its own right, for data that is not a table
row: `kv::put`, `kv::get`, `kv::delete`, `kv::contains`, and `kv::scan::pairs` /
`kv::scan::keys` over a key prefix. Rows and plain keys share one log, one index
and one commit, so a batch between `kv::batch::begin` and `kv::batch::commit`
covers both.

## Why it is fast

Rows are addressed by rowid in a dense `Vec` - three words a row - so a keyed
read builds no key and hashes nothing. A reader holds one sealed generation in
memory when it fits its budget, and a row in that generation is rendered where
it lies: no positional read, and no copy of the value out of the buffer first. A
row outside it costs one positional read, checked against the checksum its slot
carries.

What the statement layer costs, measured against the same store through both
front ends, is in `benchmarks/`: `gos-sql` answers a request with a `SELECT`,
`gos-kv` with the keyed read, and the gap between them is that layer and nothing
else.
