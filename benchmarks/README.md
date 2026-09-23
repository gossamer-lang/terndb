# terndb benchmarks

Two benchmarks over one seeded dataset, so the numbers compare like with like.

- **`./run.sh`** - concurrent reads behind one GET endpoint. Everything it
  measures is **over HTTP**, with the store embedded in the web server's own
  process, which is the only deployment terndb has.
- **`./embedded.sh`** - the same keyed read with **no HTTP at all**: one thread
  calling the store it is linked against, in a loop. It answers what the read
  path costs once the web tier is taken away.

## The contract every server implements

- `GET /user/{id}` -> `200` with
  `{"id":1,"name":"user-1","email":"user-1@example.com","score":1.5,"active":true}`,
  or `404` with `{"error":"not found"}`.
- `GET /health` -> `200 ok`, used to wait for readiness.
- The dataset is `N` rows with ids `1..N`, seeded identically everywhere. There
  is one store per engine - `bench.sqlite`, `bench.terndb`, `bench.redb`,
  `bench.rocksdb`, `bench.lmdb`, `bench.pogreb`,
  `bench.bbolt` - and the
  terndb servers all read the same `bench.terndb`, whether they reach a row through
  a `SELECT` or through a keyed get.
- The lookup is by primary key in every stack: `rowid` for terndb, the
  `INTEGER PRIMARY KEY` for SQLite, the key for redb, RocksDB, LMDB, pogreb, and bbolt. No server does a scan, so
  the benchmark measures the read path rather than the absence of an index.

## How a target is named

Every target is named for what it is, in one order:

```
<language>-<tool>-<deployment>-<query>[-<variant>]

gos-terndb-embedded-kv           Gos terndb Embedded: KV
rust-sqlite-embedded-sql         Rust SQLite Embedded: SQL
rust-redb-embedded-kv-typed      Rust redb (typed) Embedded: KV
gos-terndb-embedded-sql-gos-run  Gos terndb (gos run) Embedded: SQL
```

- **language** - `gos`, `rust`, `go`, `python`, `cpp`, `c`.
- **tool** - the store: `terndb`, `sqlite`, `redb`, `rocksdb`, `lmdb`, `pogreb`, `bbolt`.
- **deployment** - `embedded`, the store in the caller's own process, which is
  the only one terndb has.
- **query** - `kv`, a row reached by its identity, or `sql`, a row reached
  through a statement.
- **variant** - the same read path under another condition: `gos-run` for the
  interpreted tier, which `gos run` runs from source, and `typed` for redb
  holding typed columns rather than finished JSON. It is shown as the words the
  id spells.

`ONLY=` takes these ids, the logs and `results.json` are keyed by them, and
every chart label is read straight out of the id, so a target cannot be called
one thing in a run and another on a chart.

## What is compared over HTTP

One deployment, and every stack that can serve it. The store runs in the web
server's own process (`web-embedded-db-*`): `gos-terndb-embedded-sql`,
`gos-terndb-embedded-kv`, `rust-redb-embedded-kv`, `c-lmdb-embedded-kv`,
`cpp-rocksdb-embedded-kv`, `go-pogreb-embedded-kv`, `go-bbolt-embedded-kv`,
`rust-sqlite-embedded-sql`,
`go-sqlite-embedded-sql`, `python-sqlite-embedded-sql`, and the two terndb
servers again under `gos run` (`gos-terndb-embedded-sql-gos-run`,
`gos-terndb-embedded-kv-gos-run`).

The terndb targets read the same store, seeded once, and reach the same bytes:
the row lives under `users/r/<id>`, which is how the storage layer addresses it,
and both decode and render it per request. What the `kv` target leaves off is
the statement layer - no parse, no plan, no prepared-statement lookup - so the
gap between the two is that layer and nothing else.

redb, RocksDB, LMDB, pogreb, and bbolt are asked for the shape a key-value
store is normally given:
the row's finished JSON under its id, returned as it is. They therefore do no
row decode, which is worth holding in mind next to `gos-terndb-embedded-kv` - the
comparison is of key-value read paths, not of identical amounts of work.

RocksDB is the C++ LSM store, reached from C++ with its default options: the
row's JSON under its id as an 8-byte big-endian key, one `Get` per request, the
store opened read-only as the terndb servers open theirs. The HTTP server is
cpp-httplib with one worker per hardware thread. Both are fetched at pinned tags
and built by CMake with the rest of the target, so a run needs a C++20 compiler
and CMake and nothing installed system-wide; the first build compiles RocksDB
itself and takes several minutes.

LMDB is the C memory-mapped B+tree store, reached from C: the row's JSON under
the same 8-byte big-endian key, a read transaction per request as redb takes a
snapshot per request, the environment opened read-only. The HTTP server is
CivetWeb with one worker per hardware thread. LMDB and CivetWeb are fetched at
pinned tags and compiled by CMake with the target.

pogreb is a Go store with a hash index over an append-only log, reached from Go
through `net/http`: the row's JSON under the same 8-byte big-endian key, one
`Get` per request, which hands back a copy of the value.

bbolt is etcd's B+tree store for Go, reached from Go through `net/http`: the
row's JSON under the same key in a `users` bucket, a read transaction per request
as redb takes a snapshot per request, the value copied out before the
transaction ends, and the file opened read-only.

## What is compared with no HTTP (`query-only-*`)

`./embedded.sh` links each store into a single-threaded caller and asks it for a
row by primary key, over and over, for a fixed window. `run.sh` is what seeds
the stores, so run it first; `embedded.sh` reads the dataset it wrote rather
than defining a second one that could drift from it. There is no socket, no
web framework and no load generator in the loop, so the number is the read path
and the JSON rendering and nothing else.

Ten targets, each rendering the same 90-byte row, and each reporting its
queries per second and its peak resident memory:

- `gos-terndb-embedded-sql` - a prepared `SELECT ... WHERE rowid = ?`.
- `gos-terndb-embedded-kv` - the keyed get the storage layer answers with no
  statement layer above it.
- `rust-sqlite-embedded-sql` - rusqlite, one prepared statement.
- `go-sqlite-embedded-sql` - `database/sql` over the pure-Go driver, one
  prepared statement, one connection.
- `rust-redb-embedded-kv` - a read snapshot per query, which is what the HTTP
  server takes per request, with the stored text materialised into an owned
  String so it answers with what the others answer with.
- `rust-redb-embedded-kv-typed` - the same store asked the question the SQL
  stores are asked: four typed columns in a second table, decoded and rendered
  per query.
- `cpp-rocksdb-embedded-kv` - one `Get` per query into an owned `std::string`,
  which is what its HTTP server does per request.
- `c-lmdb-embedded-kv` - a read transaction per query, with the row copied out
  of the map into a buffer of its own, as the others hand back an owned string.
- `go-pogreb-embedded-kv` - one `Get` per query, whose answer is a copy of the
  stored value.
- `go-bbolt-embedded-kv` - a read transaction per query, with the value copied
  out of it into a slice of its own.

The typed target seeds its table into the same file, and runs after
`rust-redb-embedded-kv` so the plain target is measured against the store it was
seeded with.

### What redb is doing, and why its number is so much larger

redb answers far more queries a second than the SQLite stores, and the
reason is what it was asked to store rather than how fast it reads. Its table
holds each row's **finished JSON text** under the row's id, so a query is a
b-tree descent in its own page cache followed by handing back bytes that are
already the answer. It never leaves user space, never checks a record, and
never turns a value into text. `rust-redb-embedded-kv-typed` is the same engine
asked to do the decode and the render, and it is the bar a SQL store is measured
against: terndb answers 3 474 022 queries a second to its 1 804 314.

The SQL stores are asked a different question. `gos-terndb-embedded-sql`,
`rust-sqlite-embedded-sql` and `go-sqlite-embedded-sql` hold four typed columns,
and every query reads them, checks what it
read, and formats the JSON the caller receives - including the float, which is
the single most expensive field to render. terndb additionally pays one
positional read per row, where redb and SQLite answer from a cache of their own.

So the reading is not "redb is faster at the same work": a store
that keeps the rendered answer beats one that builds it, and that is the trade
the two designs make. Compare `gos-terndb-embedded-sql` against
`rust-sqlite-embedded-sql` for the like-for-like number - both decode typed
columns and render one - and against `gos-terndb-embedded-kv` for what the
statement layer costs.

Every target walks the same id sequence - `(i * 2654435761) % rows + 1` - so each
decodes the same rows in the same order and none is handed a friendlier access
pattern. Each runs eight batches of warmup before the clock starts, and the
byte count each target reports is the JSON it received: they match to the byte
across all five, which is how the run proves the work was the same.

## Running

```sh
./run.sh                 # everything over HTTP, default 50000 rows
ROWS=200000 ./run.sh     # a larger dataset
ONLY=gos-terndb-embedded-kv,rust-redb-embedded-kv ./run.sh

./embedded.sh            # the same dataset, no HTTP
```

`DATA=` and `RESULTS=` say where the stores and the run are written, so a short
run can check the harness without touching the dataset and the numbers already
recorded:

```sh
DATA=/tmp/bench RESULTS=/tmp/bench/results.json ROWS=2000 SECONDS_PER=1 ./run.sh
```

They write `results.json` and `embedded.json` and print a summary. Then:

```sh
uv run chart.py results.json
```

which writes five charts plus `charts/index.html`, where the table of every
measurement sits beneath them:

- `web-embedded-db-{response-rate,cpu,memory}` - a web server with the store in
  its own process.
- `query-only-{queries,memory}` - no web tier at all: one thread asking the
  store it is linked against.

A chart keeps one colour per read path across every image, and a variant is that
path's colour under a hatch. The palette holds eight hues, so the Go stores past
the first share Go's hue under a texture of their own: dots for pogreb, lines for
bbolt.

## Which number to read

An HTTP target is described by three numbers and a query-only one by two.
Nothing else is reported, because nothing else decides between two stores.

**Response rate** is what the deployment serves. Read it together with the next
column: the load generator is Go's standard `http.Client` driven by `WORKERS`
goroutines on the same box as the server, and it saturates at a little over
200 000 requests a second. Every target at or near that number is reporting the
client's ceiling rather than its own - a Gossamer server whose handler returns a
constant body measures the same response rate as one that reads a row.

**CPU per request** separates the targets the response rate cannot. It is what
the server's process tree actually spent per request it served, from
`utime + stime` either side of the measured window. `WORKERS=32 ./run.sh` drives
the client harder if you want to see where the headroom goes; the ranking by CPU
per request does not move.

**Memory** is the peak resident set of the whole process tree, from the kernel's
own high-water mark, which is one process: the web server holds the store. The
query-only benchmark reports the same thing for the
one process it runs, which is what the store costs to run with nothing else in
it.

**Queries per second** is the query-only rate: one thread, no socket, no HTTP.

A target's resident set is also sampled either side of the load, and the load
runs twice so a cache filling up is not read as a leak: the first window
finishes filling whatever caches the stack keeps, and the second is the one
reported. A target still growing by more than `LEAK_LIMIT_BYTES` (8) bytes a
request in that second window is recorded as a failure and marked `LEAKING`,
because the rest of its numbers describe a process that cannot serve for long.
That is a gate on the measurement, not a number the report ranks anything by.

## Reading the results

- The no-HTTP benchmark is one thread by design: it is a per-query cost, not a
  saturation number, and a thread count would only measure this box's cores.
  Read it beside `cpu_us_per_request` from the HTTP run, which measures the same
  thing from the other direction.
- redb is asked for a read snapshot per query, matching its HTTP server. An
  embedded caller that held one snapshot across many reads would answer from a
  fixed view and pay less; that is a different contract, not a faster one.
- Everything runs on one machine, so the client competes with the servers for
  cores. The load generator is pinned to fewer workers than the box has, which
  is why the throughput column ties at the top and the CPU column does not.
- Page caches are warm by the time the measured phase starts; this measures the
  code path, not the disk.
- SQLite is used with WAL and a per-connection handle, which is its fast
  read configuration.
- Every SQL server prepares its statement once: `prepare_cached` for rusqlite,
  a prepared statement for terndb. Parsing per request was worth about 34 us
  of the 41 us a request used to spend below HTTP.
