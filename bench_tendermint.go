// bench_tendermint.go
//
// Usage examples:
//
//   # hit a local single-node devnet
//   go run bench_tendermint.go -addr 127.0.0.1:26657 -n 10000 -c 32
//
//   # hit a CloudLab node that listens on the public control IP
//   go run bench_tendermint.go -addr 155.98.38.94:26657 -n 50000 -c 64 -prefix node0
//
// Flags
//   -addr       RPC host:port of **one** Tendermint node (it will forward to peers)
//   -n          total number of Tx to send (default 10 000)
//   -c          concurrency -- number of goroutines acting as clients (default 16)
//   -prefix     key-prefix so multiple benchmark runs don’t overwrite each other
//   -timeout    per-RPC timeout (default 5 s)

package main

import (
	"bytes"
	"encoding/hex"
	"flag"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

/*---------------------------------------------------------*
|                    CLI Flags & Globals                  |
*---------------------------------------------------------*/

var (
	addr     = flag.String("addr", "127.0.0.1:26657", "host:port of Tendermint RPC")
	totalReq = flag.Int("n", 10000, "total requests to send")
	concur   = flag.Int("c", 16, "concurrency (goroutines)")
	prefix   = flag.String("prefix", "bench", "key prefix to avoid clashes")
	timeout  = flag.Duration("timeout", 5*time.Second, "per-request timeout")
)

const contentType = "application/x-www-form-urlencoded"

/*---------------------------------------------------------*
|                       Helpers                           |
*---------------------------------------------------------*/

// makeTx returns a []byte that kvstore understands:  key_i=value_i
func makeTx(i int) []byte {
	key := fmt.Sprintf("%s_%d", *prefix, i)
	val := strconv.Itoa(i)
	return []byte(key + "=" + val)
}

// broadcastTxCommit performs POST /broadcast_tx_commit?tx=0x<hex>
func broadcastTxCommit(client *http.Client, hostport string, tx []byte) error {
	hexTx := "0x" + hex.EncodeToString(tx)
	reqURL := fmt.Sprintf("http://%s/broadcast_tx_commit?tx=%s", hostport, hexTx)

	req, err := http.NewRequest("POST", reqURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", contentType)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Tendermint always returns 200 even on tx failure;
	// treat a network-level error or empty body as failure here.
	body, _ := ioutil.ReadAll(resp.Body)
	if len(body) == 0 || resp.StatusCode != 200 {
		return fmt.Errorf("bad HTTP %d", resp.StatusCode)
	}
	// very light check: contains `"code":0`
	if !bytes.Contains(body, []byte(`"code":0`)) {
		return fmt.Errorf("tx rejected: %s",
			strings.ReplaceAll(string(body), "\n", " "))
	}
	return nil
}

/*---------------------------------------------------------*
|                     Main routine                        |
*---------------------------------------------------------*/

func main() {
	flag.Parse()
	rand.Seed(time.Now().UnixNano())

	// For latency stats
	latCh := make(chan time.Duration, *totalReq)
	var sent int32

	// HTTP client with per-request timeout
	httpClient := &http.Client{Timeout: *timeout}

	var wg sync.WaitGroup
	for i := 0; i < *concur; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for {
				idx := int(atomic.AddInt32(&sent, 1)) - 1
				if idx >= *totalReq {
					return
				}
				tx := makeTx(idx)
				start := time.Now()
				if err := broadcastTxCommit(httpClient, *addr, tx); err != nil {
					fmt.Printf("[worker %d] tx %d error: %v\n", id, idx, err)
					continue // still count latency for successful ones only
				}
				latCh <- time.Since(start)
			}
		}(i)
	}

	// Close channel when all workers exit
	go func() {
		wg.Wait()
		close(latCh)
	}()

	// Collect latencies
	var (
		latencies []time.Duration
		sum       time.Duration
		count     int
	)
	for l := range latCh {
		latencies = append(latencies, l)
		sum += l
		count++
	}

	if count == 0 {
		fmt.Println("No successful transactions!")
		return
	}

	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	p50 := latencies[count/2]
	p95 := latencies[int(float64(count)*0.95)]
	p99 := latencies[int(float64(count)*0.99)]
	avg := sum / time.Duration(count)

	// Rough throughput = total / (avgLatency * concurrency)
	throughput := float64(count) / (avg.Seconds())

	fmt.Println("========== Tendermint KVStore Benchmark ==========")
	fmt.Printf("RPC target        : %s\n", *addr)
	fmt.Printf("Total TX          : %d\n", count)
	fmt.Printf("Concurrency       : %d\n", *concur)
	fmt.Printf("Avg latency       : %v\n", avg)
	fmt.Printf("p50 / p95 / p99   : %v / %v / %v\n", p50, p95, p99)
	fmt.Printf("Approx throughput : %.2f tx/sec\n", throughput)
}
