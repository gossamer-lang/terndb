// pogreb queried in process, with nothing above it.
//
// One goroutine asks for a row per query and receives its stored JSON as a
// slice of its own, which is what the HTTP server does per request - so the two
// numbers describe the same read path with and without a web tier above it.
//
//	embedded <path> <rows> <seconds>
package main

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/akrylysov/pogreb"

	"terndbbench/gopogreb/internal/bench"
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
	db, err := pogreb.Open(path, nil)
	if err != nil {
		panic(err)
	}
	defer db.Close()

	// Get answers a slice the caller owns, as every other target hands back
	// an owned answer.
	read := func(id uint64) int {
		v, err := db.Get(bench.Key(id))
		if err != nil {
			return 0
		}
		return len(v)
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
	fmt.Printf(`{"target":"go-pogreb","queries":%d,"seconds":%.3f,"qps":%.1f,"ns_per_query":%.1f,"bytes":%d}`+"\n",
		queries, secs, float64(queries)/secs, secs*1e9/float64(queries), bytes)
}
