// bbolt queried in process, with nothing above it.
//
// One goroutine takes a read transaction per query and copies the row's stored
// JSON out of it, which is what the HTTP server does per request - so the two
// numbers describe the same read path with and without a web tier above it.
//
//	embedded <path> <rows> <seconds>
package main

import (
	"fmt"
	"os"
	"strconv"
	"time"

	bolt "go.etcd.io/bbolt"

	"terndbbench/gobbolt/internal/bench"
)

func main() {
	if len(os.Args) < 4 {
		fmt.Fprintln(os.Stderr, "usage: embedded <path> <rows> <seconds>")
		os.Exit(2)
	}
	path := os.Args[1]
	rows, err := strconv.ParseInt(os.Args[2], 10, 64)
	if err != nil || rows < 1 {
		fmt.Fprintln(os.Stderr, "embedded: rows must be positive")
		os.Exit(2)
	}
	seconds, err := strconv.ParseInt(os.Args[3], 10, 64)
	if err != nil {
		fmt.Fprintln(os.Stderr, "embedded: seconds must be a number")
		os.Exit(2)
	}
	db, err := bolt.Open(path, 0o644, &bolt.Options{ReadOnly: true})
	if err != nil {
		panic(err)
	}
	defer db.Close()

	// A read transaction per query, as redb takes a snapshot per query. The
	// value is only valid inside it, so the caller receives a copy, as every
	// other target hands back an owned answer.
	read := func(id uint64) int {
		var out []byte
		_ = db.View(func(tx *bolt.Tx) error {
			if v := tx.Bucket(bench.Users).Get(bench.Key(id)); v != nil {
				out = append([]byte(nil), v...)
			}
			return nil
		})
		return len(out)
	}

	if read(1) == 0 {
		fmt.Fprintf(os.Stderr, "embedded: row 1 is missing from %s\n", path)
		os.Exit(1)
	}
	for i := int64(0); i < bench.Batch*8; i++ {
		read(bench.IDAt(i, rows))
	}

	start := time.Now()
	window := time.Duration(seconds) * time.Second
	var queries int64
	var bytes int64
	for {
		for k := int64(0); k < bench.Batch; k++ {
			bytes += int64(read(bench.IDAt(queries+k, rows)))
		}
		queries += bench.Batch
		if time.Since(start) >= window {
			break
		}
	}
	secs := time.Since(start).Seconds()
	fmt.Printf(`{"target":"go-bbolt","queries":%d,"seconds":%.3f,"qps":%.1f,"ns_per_query":%.1f,"bytes":%d}`+"\n",
		queries, secs, float64(queries)/secs, secs*1e9/float64(queries), bytes)
}
