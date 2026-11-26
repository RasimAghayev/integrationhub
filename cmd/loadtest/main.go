// Command loadtest is a small HTTP load generator for the integration execute
// endpoint. It reports throughput and latency percentiles without external
// dependencies.
package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"sync"
	"time"
)

func main() {
	var (
		url         = flag.String("url", "http://localhost:8080/api/v1/integrations", "target base URL")
		token       = flag.String("token", "", "bearer token or api key value")
		integration = flag.String("integration", "", "integration ID to execute")
		body        = flag.String("body", "{}", "request body")
		workers     = flag.Int("workers", 10, "concurrent workers")
		duration    = flag.Duration("duration", 10*time.Second, "test duration")
		timeout     = flag.Duration("timeout", 30*time.Second, "per-request timeout")
	)
	flag.Parse()

	if *integration == "" {
		log.Fatal("-integration is required")
	}

	client := &http.Client{Timeout: *timeout}
	ctx, cancel := context.WithTimeout(context.Background(), *duration)
	defer cancel()

	var (
		mu       sync.Mutex
		okCount  int
		failCnt  int
		statuses = map[int]int{}
		lat      []time.Duration
		start    = time.Now()
		wg       sync.WaitGroup
	)

	for i := 0; i < *workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ctx.Err() == nil {
				req, err := http.NewRequestWithContext(ctx, http.MethodPost,
					*url+"/"+*integration+"/execute", bytes.NewReader([]byte(*body)))
				if err != nil {
					return
				}
				req.Header.Set("Content-Type", "application/json")
				if *token != "" {
					req.Header.Set("Authorization", "Bearer "+*token)
				}
				t0 := time.Now()
				resp, err := client.Do(req)
				elapsed := time.Since(t0)
				if err != nil {
					mu.Lock()
					failCnt++
					mu.Unlock()
					continue
				}
				_, _ = io.Copy(io.Discard, resp.Body)
				_ = resp.Body.Close()

				mu.Lock()
				statuses[resp.StatusCode]++
				if resp.StatusCode < 400 {
					okCount++
				} else {
					failCnt++
				}
				lat = append(lat, elapsed)
				mu.Unlock()
			}
		}()
	}

	wg.Wait()
	elapsed := time.Since(start)

	sort.Slice(lat, func(i, j int) bool { return lat[i] < lat[j] })
	total := okCount + failCnt
	fmt.Printf("duration=%s workers=%d total=%d ok=%d failed=%d rps=%.1f\n",
		elapsed.Round(time.Millisecond), *workers, total, okCount, failCnt,
		float64(total)/elapsed.Seconds())
	if len(lat) > 0 {
		fmt.Printf("latency p50=%s p95=%s p99=%s max=%s\n",
			percentile(lat, 50), percentile(lat, 95), percentile(lat, 99), lat[len(lat)-1])
	}
	keys := make([]int, 0, len(statuses))
	for k := range statuses {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	for _, k := range keys {
		fmt.Printf("status %d: %d\n", k, statuses[k])
	}
	if failCnt > 0 {
		os.Exit(1)
	}
}

func percentile(sorted []time.Duration, p int) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	idx := (len(sorted) - 1) * p / 100
	return sorted[idx].Round(time.Millisecond)
}
