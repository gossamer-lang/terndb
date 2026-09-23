// net/http + pogreb, an embedded hash-indexed key-value store for Go.
//
// The row is stored as its finished JSON under its integer id, as rust-redb,
// cpp-rocksdb, and c-lmdb store it.
package main

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/akrylysov/pogreb"

	"terndbbench/gopogreb/internal/bench"
)

var db *pogreb.DB

func user(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	id, err := strconv.ParseUint(strings.TrimPrefix(r.URL.Path, "/user/"), 10, 64)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"error":"not found"}`)
		return
	}
	v, err := db.Get(bench.Key(id))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"error":"read failed"}`)
		return
	}
	if v == nil {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"error":"not found"}`)
		return
	}
	w.Write(v)
}

func main() {
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = "127.0.0.1:8092"
	}
	path := os.Getenv("DB_PATH")
	if path == "" {
		path = "bench.pogreb"
	}
	var err error
	db, err = pogreb.Open(path, nil)
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
