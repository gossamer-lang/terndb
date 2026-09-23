// Seeds the bbolt store with the shared dataset.
//
//	seed <path> <rows>
package main

import (
	"fmt"
	"os"
	"strconv"

	bolt "go.etcd.io/bbolt"

	"terndbbench/gobbolt/internal/bench"
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
	db, err := bolt.Open(path, 0o644, nil)
	if err != nil {
		panic(err)
	}
	err = db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucket(bench.Users)
		if err != nil {
			return err
		}
		for id := uint64(1); id <= rows; id++ {
			if err := b.Put(bench.Key(id), bench.Row(id)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		panic(err)
	}
	if err := db.Close(); err != nil {
		panic(err)
	}
	fmt.Printf("seeded %d rows into %s\n", rows, path)
}
