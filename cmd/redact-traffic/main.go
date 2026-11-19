// redact-traffic is an offline maintenance tool. Stop ARTEX and make an encrypted
// backup first. Unsupported file/blob captures cause a fail-closed error.
package main

import (
	"flag"
	"fmt"
	"github.com/Autumn-27/artex/traffic"
	"log"
)

func main() {
	dir := flag.String("dir", "", "traffic store directory (server must be stopped)")
	flag.Parse()
	if *dir == "" {
		log.Fatal("-dir is required")
	}
	t, err := traffic.Open(*dir, "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	defer t.Close()
	n, err := t.RedactStoredCredentials()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Redacted %d stored captures; compacted SQLite and truncated WAL\n", n)
}
