// Command drift fails when the live OpenAPI document differs from the vendored copy.
//
// The document is served to an API key only, so the key is read from
// SRE_AGENT_API_KEY and sent as a bearer token.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"reflect"
	"strings"
	"time"
)

// keyPrefix is how much of an API key Settings shows (`sre_ak_` and five more),
// so naming it in an error points at the key without disclosing it.
const keyPrefix = 12

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: drift <url> <vendored path>")
		os.Exit(2)
	}
	code, msg := run(&http.Client{Timeout: 30 * time.Second}, os.Args[1], os.Args[2], os.Getenv("SRE_AGENT_API_KEY"))
	if code == 0 {
		fmt.Println(msg)
	} else if code == 1 {
		fmt.Println(msg)
	} else {
		fmt.Fprintln(os.Stderr, msg)
	}
	os.Exit(code)
}

// run compares the live document with the vendored copy. It returns 0 when they
// match, 1 when they differ, and 2 when the comparison could not be made.
func run(client *http.Client, url, vendoredPath, apiKey string) (int, string) {
	// A secret pasted with a stray space or newline is the common way to get a
	// key the API never matches.
	apiKey = strings.TrimSpace(apiKey)
	req, err := http.NewRequest(http.MethodGet, url, nil) //nolint:gosec // the operator names the document to compare
	if err != nil {
		return 2, err.Error()
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	resp, err := client.Do(req) //nolint:gosec // the operator names the document to compare
	if err != nil {
		return 2, err.Error()
	}
	defer func() { _ = resp.Body.Close() }()
	live, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		if apiKey == "" {
			return 2, fmt.Sprintf("the API refused the request (%d): the document needs an API key, set SRE_AGENT_API_KEY (a repository secret in CI)", resp.StatusCode)
		}
		return 2, fmt.Sprintf("the API refused the key starting %s (%d): it is not a live API key of an enabled organization; compare it with Settings, API Keys, or set SRE_AGENT_API_KEY again with the full key", publicPrefix(apiKey), resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return 2, fmt.Sprintf("unexpected status %d fetching the live document", resp.StatusCode)
	}
	vendored, err := os.ReadFile(vendoredPath) //nolint:gosec // the operator names the vendored copy
	if err != nil {
		return 2, err.Error()
	}
	var a, b any
	if json.Unmarshal(live, &a) != nil || json.Unmarshal(vendored, &b) != nil {
		return 2, "one of the documents is not JSON"
	}
	if !reflect.DeepEqual(a, b) {
		return 1, "the live API contract differs from internal/contract/openapi.json; re-vendor it and run go test ./internal/contract/"
	}
	return 0, "contract current"
}

func publicPrefix(key string) string {
	if len(key) <= keyPrefix {
		return key[:len(key)/2] + "..."
	}
	return key[:keyPrefix]
}
