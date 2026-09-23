// Seeds the pogreb store with the shared dataset.
//
//	seed <path> <rows>
package main

import (
	"fmt"
	"os"
	"strconv"

	"github.com/akrylysov/pogreb"

	"terndbbench/gopogreb/internal/bench"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: seed <path> <rows>")
		os.Exit(2)
	}
	path := os.Args[1]
	rows, err := strconv.ParseUint(os.Args[2], 10, 64)
	if err != nil {
		fmt.Fprintln(os.Stderr, "seed: rows must be a number")
		os.Exit(2)
	}
	if err := os.RemoveAll(path); err != nil {
		panic(err)
	}
	db, err := pogreb.Open(path, nil)
	if err != nil {
		panic(err)
	}
	for id := uint64(1); id <= rows; id++ {
		if err := db.Put(bench.Key(id), bench.Row(id)); err != nil {
			panic(err)
		}
	}
	if err := db.Sync(); err != nil {
		panic(err)
	}
	if err := db.Close(); err != nil {
		panic(err)
	}
	fmt.Printf("seeded %d rows into %s\n", rows, path)
}
