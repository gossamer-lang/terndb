// net/http + bbolt, etcd's embedded B+tree key-value store for Go.
//
// The row is stored as its finished JSON under its integer id, as rust-redb,
// cpp-rocksdb, c-lmdb, and go-pogreb store it, and the store is opened
// read-only, as the terndb servers open theirs.
package main

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	bolt "go.etcd.io/bbolt"

	"terndbbench/gobbolt/internal/bench"
)

var db *bolt.DB

func user(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	id, err := strconv.ParseUint(strings.TrimPrefix(r.URL.Path, "/user/"), 10, 64)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"error":"not found"}`)
		return
	}
	// A read transaction per request, as redb takes a snapshot per request.
	// The value is only valid inside it, so it is copied out.
	var row []byte
	err = db.View(func(tx *bolt.Tx) error {
		if v := tx.Bucket(bench.Users).Get(bench.Key(id)); v != nil {
			row = append([]byte(nil), v...)
		}
		return nil
	})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"error":"read failed"}`)
		return
	}
	if row == nil {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"error":"not found"}`)
		return
	}
	w.Write(row)
}

func main() {
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = "127.0.0.1:8093"
	}
	path := os.Getenv("DB_PATH")
	if path == "" {
		path = "bench.bbolt"
	}
	var err error
	db, err = bolt.Open(path, 0o644, &bolt.Options{ReadOnly: true})
	if err != nil {
		panic(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/user/", user)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "ok") })
	if err := http.ListenAndServe(addr, mux); err != nil {
		panic(err)
	}
}
