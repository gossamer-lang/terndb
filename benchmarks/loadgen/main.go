// Concurrent GET load generator. Standard library only, so it needs no module
// downloads and adds no measurement of somebody else's HTTP client.
//
// Each worker keeps one keep-alive connection and has one request in flight on
// it, the shape of a pool of synchronous callers. The request is written and the
// response framed directly on the connection: a general-purpose client spends
// more per request than the servers under test do, and would cap every one of
// them at its own ceiling.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"math/rand"
	"net"
	"net/url"
	"os"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

type Result struct {
	Target      string  `json:"target"`
	Workers     int     `json:"workers"`
	DurationSec float64 `json:"duration_sec"`
	Requests    int64   `json:"requests"`
	Errors      int64   `json:"errors"`
	NotFound    int64   `json:"not_found"`
	RPS         float64 `json:"rps"`
	MeanMs      float64 `json:"mean_ms"`
	P50Ms       float64 `json:"p50_ms"`
	P90Ms       float64 `json:"p90_ms"`
	P99Ms       float64 `json:"p99_ms"`
	MaxMs       float64 `json:"max_ms"`
}

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	i := int(p * float64(len(sorted)-1))
	return sorted[i]
}

// conn is one keep-alive connection and the buffers its requests reuse.
type conn struct {
	addr string
	host string
	c    net.Conn
	r    *bufio.Reader
	req  []byte
}

func (k *conn) dial() error {
	if k.c != nil {
		k.c.Close()
	}
	c, err := net.DialTimeout("tcp", k.addr, 10*time.Second)
	if err != nil {
		k.c = nil
		return err
	}
	if tc, ok := c.(*net.TCPConn); ok {
		tc.SetNoDelay(true)
	}
	k.c = c
	k.r = bufio.NewReaderSize(c, 16<<10)
	return nil
}

// get sends `GET /user/{id}` and reads the whole response, answering its status.
func (k *conn) get(id int) (int, error) {
	if k.c == nil {
		if err := k.dial(); err != nil {
			return 0, err
		}
	}
	k.req = append(k.req[:0], "GET /user/"...)
	k.req = strconv.AppendInt(k.req, int64(id), 10)
	k.req = append(k.req, " HTTP/1.1\r\nHost: "...)
	k.req = append(k.req, k.host...)
	k.req = append(k.req, "\r\n\r\n"...)
	k.c.SetDeadline(time.Now().Add(10 * time.Second))
	if _, err := k.c.Write(k.req); err != nil {
		k.c.Close()
		k.c = nil
		return 0, err
	}
	status, err := k.readResponse()
	if err != nil {
		k.c.Close()
		k.c = nil
	}
	return status, err
}

var errMalformed = errors.New("malformed response")

// readResponse reads one response, framed by Content-Length or chunked
// encoding, and closes the connection when the server says it will.
func (k *conn) readResponse() (int, error) {
	line, err := k.r.ReadSlice('\n')
	if err != nil {
		return 0, err
	}
	if len(line) < 12 || !bytes.HasPrefix(line, []byte("HTTP/1.")) {
		return 0, errMalformed
	}
	status, err := strconv.Atoi(string(line[9:12]))
	if err != nil {
		return 0, errMalformed
	}
	length, chunked, closing := -1, false, false
	for {
		h, err := k.r.ReadSlice('\n')
		if err != nil {
			return 0, err
		}
		h = bytes.TrimRight(h, "\r\n")
		if len(h) == 0 {
			break
		}
		colon := bytes.IndexByte(h, ':')
		if colon < 0 {
			return 0, errMalformed
		}
		name, value := h[:colon], bytes.TrimSpace(h[colon+1:])
		switch {
		case bytes.EqualFold(name, []byte("content-length")):
			if length, err = strconv.Atoi(string(value)); err != nil {
				return 0, errMalformed
			}
		case bytes.EqualFold(name, []byte("transfer-encoding")):
			chunked = bytes.Contains(bytes.ToLower(value), []byte("chunked"))
		case bytes.EqualFold(name, []byte("connection")):
			closing = bytes.Contains(bytes.ToLower(value), []byte("close"))
		}
	}
	switch {
	case chunked:
		if err := k.skipChunked(); err != nil {
			return 0, err
		}
	case length >= 0:
		if _, err := k.r.Discard(length); err != nil {
			return 0, err
		}
	default:
		// No framing: the body runs to the end of the connection.
		if _, err := io.Copy(io.Discard, k.r); err != nil {
			return 0, err
		}
		closing = true
	}
	if closing {
		k.c.Close()
		k.c = nil
	}
	return status, nil
}

func (k *conn) skipChunked() error {
	for {
		line, err := k.r.ReadSlice('\n')
		if err != nil {
			return err
		}
		size := bytes.TrimSpace(line)
		if semi := bytes.IndexByte(size, ';'); semi >= 0 {
			size = size[:semi]
		}
		n, err := strconv.ParseInt(string(size), 16, 64)
		if err != nil {
			return errMalformed
		}
		if n == 0 {
			// Trailers, then the blank line that ends them.
			for {
				t, err := k.r.ReadSlice('\n')
				if err != nil {
					return err
				}
				if len(bytes.TrimRight(t, "\r\n")) == 0 {
					return nil
				}
			}
		}
		if _, err := k.r.Discard(int(n) + 2); err != nil {
			return err
		}
	}
}

func main() {
	target := flag.String("url", "http://127.0.0.1:8080", "base URL")
	workers := flag.Int("workers", 8, "concurrent workers")
	seconds := flag.Int("seconds", 5, "measured duration")
	warmup := flag.Int("warmup", 2, "warmup seconds, not measured")
	rows := flag.Int("rows", 50000, "highest row id to request")
	name := flag.String("name", "", "label for the result")
	flag.Parse()

	base, err := url.Parse(*target)
	if err != nil || base.Host == "" {
		os.Stderr.WriteString("loadgen: -url must be http://host:port\n")
		os.Exit(2)
	}

	var requests, failures, notFound int64
	latencies := make([][]float64, *workers)
	var wg sync.WaitGroup
	stop := make(chan struct{})
	// Every worker reads the same flag, so all of them start recording at the
	// same instant and the result is the whole pool's throughput.
	var measuring atomic.Bool

	for w := 0; w < *workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			rng := rand.New(rand.NewSource(int64(w)*7919 + 1))
			local := make([]float64, 0, 1<<16)
			// A connection is reused for as long as the server keeps it,
			// which is what a real caller does; re-dialling per request would
			// measure the kernel instead of the server.
			k := &conn{addr: base.Host, host: base.Host}
			defer func() {
				if k.c != nil {
					k.c.Close()
				}
			}()
			for {
				select {
				case <-stop:
					latencies[w] = local
					return
				default:
				}
				id := rng.Intn(*rows) + 1
				start := time.Now()
				status, err := k.get(id)
				elapsed := float64(time.Since(start).Microseconds()) / 1000.0
				if err != nil {
					atomic.AddInt64(&failures, 1)
					continue
				}
				if status == 404 {
					atomic.AddInt64(&notFound, 1)
				} else if status != 200 {
					atomic.AddInt64(&failures, 1)
					continue
				}
				if measuring.Load() {
					atomic.AddInt64(&requests, 1)
					local = append(local, elapsed)
				}
			}
		}(w)
	}

	time.Sleep(time.Duration(*warmup) * time.Second)
	measuring.Store(true)
	started := time.Now()
	time.Sleep(time.Duration(*seconds) * time.Second)
	elapsed := time.Since(started).Seconds()
	measuring.Store(false)
	close(stop)
	wg.Wait()

	all := make([]float64, 0, requests)
	for _, l := range latencies {
		all = append(all, l...)
	}
	sort.Float64s(all)
	sum := 0.0
	for _, v := range all {
		sum += v
	}
	mean := 0.0
	if len(all) > 0 {
		mean = sum / float64(len(all))
	}
	label := *name
	if label == "" {
		label = *target
	}
	res := Result{
		Target: label, Workers: *workers, DurationSec: elapsed,
		Requests: int64(len(all)), Errors: failures, NotFound: notFound,
		RPS: float64(len(all)) / elapsed, MeanMs: mean,
		P50Ms: percentile(all, 0.50), P90Ms: percentile(all, 0.90),
		P99Ms: percentile(all, 0.99), MaxMs: percentile(all, 1.0),
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(res)
}
