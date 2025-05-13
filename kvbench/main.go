package main

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"net/http"
	"time"
)

var (
	rpcAddr  = flag.String("addr", "127.0.0.1:26657", "Tendermint RPC host:port")
	tps      = flag.Int("tps", 500, "offered tx/s")
	duration = flag.Int("sec", 120, "benchmark length in seconds")
)

func postTx(kv string) error {
	tx := "0x" + hex.EncodeToString([]byte(kv))
	url := fmt.Sprintf("http://%s/broadcast_tx_commit?tx=%s", *rpcAddr, tx)
	resp, err := http.Post(url, "text/plain", nil)
	if err != nil {
		return err
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return nil
}

func query(key string) ([]byte, error) {
	url := fmt.Sprintf(`http://%s/abci_query?data="%s"`, *rpcAddr, key)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var buf bytes.Buffer
	io.Copy(&buf, resp.Body)

	raw := bytes.Split(buf.Bytes(), []byte(`"value":"`))
	if len(raw) < 2 {
		return nil, fmt.Errorf("no value")
	}
	valb64 := bytes.SplitN(raw[1], []byte(`"`), 2)[0]
	return base64.StdEncoding.DecodeString(string(valb64))
}

func main() {
	flag.Parse()

	//account bootstrap
	_ = postTx("acc1=10000000")
	_ = postTx("acc2=0")

	//fire load
	fmt.Printf("sending %d TPS for %d s …\n", *tps, *duration)
	tick := time.NewTicker(time.Second / time.Duration(*tps))
	stop := time.After(time.Duration(*duration) * time.Second)

	var sent uint64
loop:
	for {
		select {
		case <-tick.C:
			sent++

			// acc2_N = uint64(1)

			balance := sent
			buf := make([]byte, 8)
			binary.LittleEndian.PutUint64(buf, uint64(balance))
			kv := "acc2=" + string(buf)
			go postTx(kv)

		case <-stop:
			break loop
		}
	}
	tick.Stop()

	//cool-down for two more 10-s rounds
	time.Sleep(20 * time.Second)

	//check balance
	val, _ := query("acc2")
	var total uint64
	for i := 0; i+8 <= len(val); i += 8 {
		total += binary.LittleEndian.Uint64(val[i : i+8])
	}
	fmt.Printf("sent=%d committed=%d expected≥%d\n",
		sent, total, *tps*(*duration))
}
