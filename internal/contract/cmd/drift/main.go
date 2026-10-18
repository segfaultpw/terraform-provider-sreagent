// Command drift compares the live OpenAPI document with the vendored copy.
//
// The comparison is of the contract's shape, not its prose: documentation keys
// (description, summary, title, example, examples, externalDocs, $comment) are
// removed at every schema depth before the two documents are compared, so a
// reworded description is "current" and a path, method, parameter, property,
// type, enum value, required list, format or default that changed is drift.
// With -write the live document replaces the vendored copy whenever the two
// differ at all (shape or prose), so the vendored copy keeps tracking the
// platform and stays readable.
//
// The document is served to an API key only, so the key is read from
// SRE_AGENT_API_KEY and sent as a bearer token.
//
// Exit codes: 0 the shapes match, 1 the shapes differ, 2 the comparison could
// not be made.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"
)

// keyPrefix is how much of an API key Settings shows (`sre_ak_` and five more),
// so naming it in an error points at the key without disclosing it.
const keyPrefix = 12

func main() {
	fs := flag.NewFlagSet("drift", flag.ContinueOnError)
	write := fs.Bool("write", false, "replace the vendored copy with the live document when they differ at all")
	if err := fs.Parse(os.Args[1:]); err != nil || fs.NArg() != 2 {
		fmt.Fprintln(os.Stderr, "usage: drift [-write] <url> <vendored path>")
		os.Exit(2)
	}
	code, msg := run(&http.Client{Timeout: 30 * time.Second}, fs.Arg(0), fs.Arg(1), os.Getenv("SRE_AGENT_API_KEY"), *write)
	switch code {
	case 0, 1:
		fmt.Println(msg)
	default:
		fmt.Fprintln(os.Stderr, msg)
	}
	os.Exit(code)
}

// run compares the live document with the vendored copy. It returns 0 when
// their shapes match, 1 when they differ, and 2 when the comparison could not be
// made. With write set, a live document that differs in any way (shape or
// prose) replaces the vendored copy.
func run(client *http.Client, url, vendoredPath, apiKey string, write bool) (int, string) {
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
	if reflect.DeepEqual(a, b) {
		return 0, "contract current"
	}
	wrote := ""
	if write {
		if err := writeFile(vendoredPath, live); err != nil {
			return 2, err.Error()
		}
		wrote = "; wrote the live document to " + vendoredPath
	}
	if !reflect.DeepEqual(shape(a), shape(b)) {
		return 1, "the live API contract differs from internal/contract/openapi.json" + wrote + "; re-vendor it and run go test ./..."
	}
	return 0, "contract current (descriptions differ)" + wrote
}

// writeFile replaces path with body through a temporary file in the same
// directory, so an interrupted write never leaves half a document behind.
func writeFile(path string, body []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".openapi-*.json")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(body); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil { //nolint:gosec // the vendored document is a committed, world-readable file
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// docKeys are the schema keywords that only document: removing them never
// changes what a request or a response may contain.
var docKeys = map[string]bool{
	"description":  true,
	"summary":      true,
	"title":        true,
	"example":      true,
	"examples":     true,
	"externalDocs": true,
	"$comment":     true,
}

// nameMaps are the keywords whose value is a map from a name the document
// chooses (a property, a schema, a path, a status code, a media type) to a
// schema or object. The names are shape and are never removed, even when one
// is spelled like a documentation key: the API has properties called
// "description" and "title".
var nameMaps = map[string]bool{
	"properties":        true,
	"patternProperties": true,
	"schemas":           true,
	"paths":             true,
	"responses":         true,
	"content":           true,
	"headers":           true,
	"securitySchemes":   true,
	"definitions":       true,
	"$defs":             true,
	"callbacks":         true,
	"links":             true,
	"encoding":          true,
}

// valueKeys hold data, not schema: what sits under them is compared as it is,
// so a default object with a "description" member is a changed default.
// x-sreagent-resources is the provider's own table of field lists and
// per-field values.
var valueKeys = map[string]bool{
	"default":              true,
	"enum":                 true,
	"const":                true,
	"x-sreagent-resources": true,
}

// shape returns doc without its documentation keys.
func shape(doc any) any {
	return strip(doc, false)
}

func strip(v any, names bool) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, child := range t {
			switch {
			case names:
				out[k] = strip(child, false)
			case docKeys[k]:
			case valueKeys[k]:
				out[k] = child
			default:
				out[k] = strip(child, nameMaps[k])
			}
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, child := range t {
			out[i] = strip(child, false)
		}
		return out
	default:
		return v
	}
}

func publicPrefix(key string) string {
	if len(key) <= keyPrefix {
		return key[:len(key)/2] + "..."
	}
	return key[:keyPrefix]
}
