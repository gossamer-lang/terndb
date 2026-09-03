// Concurrent GET load generator. Standard library only, so it needs no module
// downloads and adds no measurement of somebody else's HTTP client.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"sort"
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

func main() {
	url := flag.String("url", "http://127.0.0.1:8080", "base URL")
	workers := flag.Int("workers", 8, "concurrent workers")
	seconds := flag.Int("seconds", 5, "measured duration")
	warmup := flag.Int("warmup", 2, "warmup seconds, not measured")
	rows := flag.Int("rows", 50000, "highest row id to request")
	name := flag.String("name", "", "label for the result")
	flag.Parse()

	// One client, shared: connection reuse is what a real caller does, and
	// re-dialling per request would measure the kernel instead of the server.
	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        *workers * 2,
			MaxIdleConnsPerHost: *workers * 2,
			MaxConnsPerHost:     *workers * 2,
			DisableCompression:  true,
		},
	}

	var requests, errors, notFound int64
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
			for {
				select {
				case <-stop:
					latencies[w] = local
					return
				default:
				}
				id := rng.Intn(*rows) + 1
				start := time.Now()
				resp, err := client.Get(fmt.Sprintf("%s/user/%d", *url, id))
				elapsed := float64(time.Since(start).Microseconds()) / 1000.0
				if err != nil {
					atomic.AddInt64(&errors, 1)
					continue
				}
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
				if resp.StatusCode == 404 {
					atomic.AddInt64(&notFound, 1)
				} else if resp.StatusCode != 200 {
					atomic.AddInt64(&errors, 1)
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
		label = *url
	}
	res := Result{
		Target: label, Workers: *workers, DurationSec: elapsed,
		Requests: int64(len(all)), Errors: errors, NotFound: notFound,
		RPS: float64(len(all)) / elapsed, MeanMs: mean,
		P50Ms: percentile(all, 0.50), P90Ms: percentile(all, 0.90),
		P99Ms: percentile(all, 0.99), MaxMs: percentile(all, 1.0),
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(res)
}
