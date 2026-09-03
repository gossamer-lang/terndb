// net/http + SQLite through the pure-Go driver, so the build needs no cgo.
//
// The lookup is by INTEGER PRIMARY KEY, the same keyed read the other servers do.
package main

import (
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"

	_ "modernc.org/sqlite"
)

var db *sql.DB

func user(w http.ResponseWriter, r *http.Request) {
	raw := strings.TrimPrefix(r.URL.Path, "/user/")
	id, err := strconv.ParseInt(raw, 10, 64)
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"error":"bad id"}`)
		return
	}
	var (
		gotID  int64
		name   string
		email  string
		score  float64
		active int64
	)
	row := db.QueryRow("SELECT id, name, email, score, active FROM users WHERE id = ?", id)
	if err := row.Scan(&gotID, &name, &email, &score, &active); err != nil {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"error":"not found"}`)
		return
	}
	fmt.Fprintf(w, `{"id":%d,"name":%q,"email":%q,"score":%v,"active":%v}`,
		gotID, name, email, score, active != 0)
}

func main() {
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = "127.0.0.1:8083"
	}
	path := os.Getenv("DB_PATH")
	if path == "" {
		path = "bench.sqlite"
	}
	var err error
	db, err = sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)")
	if err != nil {
		panic(err)
	}
	// One connection per core: SQLite readers do not block each other in WAL.
	db.SetMaxOpenConns(runtime.NumCPU())
	db.SetMaxIdleConns(runtime.NumCPU())
	if err := db.Ping(); err != nil {
		panic(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/user/", user)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "ok") })
	if err := http.ListenAndServe(addr, mux); err != nil {
		panic(err)
	}
}
