// SQLite queried in process, with nothing above it.
//
// One goroutine, one connection, one prepared statement, and the same
// INTEGER PRIMARY KEY lookup rendered as the same JSON the HTTP server returns
// - so the number is the read path and the rendering, not a web tier.
//
//	embedded <path> <rows> <seconds>
package main

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"database/sql"

	_ "modernc.org/sqlite"
)

// Every target walks the same id sequence, so each decodes the same rows in the
// same order.
const stride = 2654435761

const batch = 1024

func idAt(i, rows int64) int64 { return (i*stride)%rows + 1 }

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

	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)")
	if err != nil {
		panic(err)
	}
	// One thread of queries, so one connection: a pool would measure the
	// scheduler rather than the read path.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	stmt, err := db.Prepare("SELECT id, name, email, score, active FROM users WHERE id = ?")
	if err != nil {
		panic(err)
	}

	read := func(id int64) int {
		var (
			gotID  int64
			name   string
			email  string
			score  float64
			active int64
		)
		if err := stmt.QueryRow(id).Scan(&gotID, &name, &email, &score, &active); err != nil {
			return 0
		}
		return len(fmt.Sprintf(`{"id":%d,"name":%q,"email":%q,"score":%v,"active":%v}`,
			gotID, name, email, score, active != 0))
	}

	if read(1) == 0 {
		fmt.Fprintf(os.Stderr, "embedded: row 1 is missing from %s\n", path)
		os.Exit(1)
	}
	for i := int64(0); i < batch*8; i++ {
		read(idAt(i, rows))
	}

	start := time.Now()
	window := time.Duration(seconds) * time.Second
	var queries int64
	var bytes int64
	for {
		for k := int64(0); k < batch; k++ {
			bytes += int64(read(idAt(queries+k, rows)))
		}
		queries += batch
		if time.Since(start) >= window {
			break
		}
	}
	secs := time.Since(start).Seconds()
	fmt.Printf(`{"target":"go-sqlite","queries":%d,"seconds":%.3f,"qps":%.1f,"ns_per_query":%.1f,"bytes":%d}`+"\n",
		queries, secs, float64(queries)/secs, secs*1e9/float64(queries), bytes)
}
