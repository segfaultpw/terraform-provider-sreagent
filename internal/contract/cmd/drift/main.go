// Command drift fails when the live OpenAPI document differs from the vendored copy.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"reflect"
	"time"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: drift <url> <vendored path>")
		os.Exit(2)
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(os.Args[1]) //nolint:gosec // the operator names the document to compare
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	defer func() { _ = resp.Body.Close() }()
	live, _ := io.ReadAll(resp.Body)
	vendored, err := os.ReadFile(os.Args[2]) //nolint:gosec // the operator names the vendored copy
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	var a, b any
	if json.Unmarshal(live, &a) != nil || json.Unmarshal(vendored, &b) != nil {
		fmt.Fprintln(os.Stderr, "one of the documents is not JSON")
		os.Exit(2)
	}
	if !reflect.DeepEqual(a, b) {
		fmt.Println("the live API contract differs from internal/contract/openapi.json; re-vendor it and run go test ./internal/contract/")
		os.Exit(1)
	}
	fmt.Println("contract current")
}
