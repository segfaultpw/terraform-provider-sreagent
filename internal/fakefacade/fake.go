// Package fakefacade is an in-memory implementation of the configuration
// API contract, for unit tests. The acceptance suite runs the same
// lifecycles against the real API, which is what keeps this honest.
package fakefacade

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/segfaultpw/terraform-provider-sreagent/internal/engine"
)

// Fake serves the configuration API for the specs it was built with.
type Fake struct {
	URL string

	mu          sync.Mutex
	specs       map[string]engine.Spec
	rows        map[string]map[string]map[string]any
	nextID      int
	beforeWrite map[string]func(map[string]any)
	calls       map[string]int
	bodies      map[string]map[string]any
	readOnly    bool
	truncated   map[string]bool
}

// New starts a fake for specs; the server stops when the test ends.
func New(t *testing.T, specs []engine.Spec) *Fake {
	t.Helper()
	f := &Fake{specs: map[string]engine.Spec{}, rows: map[string]map[string]map[string]any{}, beforeWrite: map[string]func(map[string]any){}, calls: map[string]int{}, bodies: map[string]map[string]any{}, truncated: map[string]bool{}}
	for _, s := range specs {
		f.specs[s.Key] = s
		f.rows[s.Key] = map[string]map[string]any{}
		// The platform answers defaults for most singletons before any write.
		if s.Shape == engine.Singleton && !s.StartsEmpty {
			f.rows[s.Key][s.Key] = blank(s)
		}
	}
	srv := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(srv.Close)
	f.URL = srv.URL
	return f
}

// Row answers the stored row, or nil.
func (f *Fake) Row(key, id string) map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.rows[key][id]
}

// Mutate edits a stored row the way a person would in the browser.
func (f *Fake) Mutate(key, id string, fn func(map[string]any)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn(f.rows[key][id])
}

// Remove deletes a row out of band.
func (f *Fake) Remove(key, id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.rows[key], id)
}

// ReadOnly makes every write answer the facade's api:config_read refusal.
func (f *Fake) ReadOnly() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.readOnly = true
}

// ForceTruncated makes the list of key say it may hold more rows than it answered.
func (f *Fake) ForceTruncated(key string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.truncated[key] = true
}

// MutateBeforeNextWrite simulates a browser edit landing between Terraform's refresh and its write.
func (f *Fake) MutateBeforeNextWrite(key string, fn func(map[string]any)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.beforeWrite[key] = fn
}

// LastBody answers the body of the latest request of one method against one resource key.
func (f *Fake) LastBody(method, key string) map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.bodies[method+" "+key]
}

// Calls counts the requests of one method against one resource key.
func (f *Fake) Calls(method, key string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[method+" "+key]
}

func reply(w http.ResponseWriter, status int, body map[string]any, etag string) {
	if etag != "" {
		w.Header().Set("ETag", etag)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func wrap(data any) map[string]any {
	return map[string]any{"organization": map[string]any{"id": "org-1", "name": "Fake", "slug": "fake"}, "data": data}
}

func fail(w http.ResponseWriter, status int, code, msg string) {
	reply(w, status, map[string]any{"error": code, "message": msg}, "")
}

func etag(spec engine.Spec, row map[string]any) string {
	keys := []string{}
	for _, a := range spec.Attrs {
		switch {
		case a.Secret:
			keys = append(keys, a.Name+"_set")
		case !a.Computed && !a.NotRead:
			keys = append(keys, a.Name)
		}
	}
	sort.Strings(keys)
	pairs := make([][]any, 0, len(keys))
	for _, k := range keys {
		pairs = append(pairs, []any{k, row[k]})
	}
	b, _ := json.Marshal(pairs)
	sum := sha256.Sum256(b)
	return `"c1-` + hex.EncodeToString(sum[:]) + `"`
}

func zero(a engine.Attr) any {
	if a.Clearable {
		return nil
	}
	switch a.Kind {
	case engine.String:
		return ""
	case engine.Int, engine.Float:
		return 0
	case engine.Bool:
		// Every enabled column on the platform defaults to true.
		return a.Name == "enabled"
	case engine.StringList:
		return []any{}
	case engine.JSON:
		return map[string]any{}
	}
	return nil
}

// blank is a row holding every field's zero value.
func blank(spec engine.Spec) map[string]any {
	row := map[string]any{}
	for _, a := range spec.Attrs {
		if a.Secret {
			row[a.Name+"_set"] = false
		} else {
			row[a.Name] = zero(a)
		}
	}
	return row
}

func (f *Fake) rowID(spec engine.Spec, row map[string]any) string {
	switch spec.Shape {
	case engine.Singleton:
		return spec.Key
	case engine.NaturalKey:
		return fmt.Sprint(row[spec.IDAttr])
	case engine.ServiceBinding:
		id := url.PathEscape(fmt.Sprint(row["service"]))
		if env := fmt.Sprint(row["environment"]); env != "" {
			id += "?environment=" + url.QueryEscape(env)
		}
		return id
	}
	return fmt.Sprint(row["id"])
}

func (f *Fake) serve(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer sre_ak_") {
		fail(w, 401, "unauthorized", "missing key")
		return
	}
	rest := strings.TrimPrefix(r.URL.EscapedPath(), "/api/v1/config/")
	parts := strings.SplitN(rest, "/", 2)
	spec, ok := f.specs[parts[0]]
	if !ok {
		fail(w, 404, "not_found", "No such configuration resource: "+parts[0]+".")
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls[r.Method+" "+spec.Key]++
	if f.readOnly && r.Method != http.MethodGet {
		fail(w, 403, "read_only_key", "This API key holds api:config_read, which reads configuration and never changes it.")
		return
	}

	id := ""
	if len(parts) == 2 {
		seg, _ := url.PathUnescape(parts[1])
		id = url.PathEscape(seg)
		if env := r.URL.Query().Get("environment"); env != "" {
			id += "?environment=" + url.QueryEscape(env)
		}
	}
	if spec.Shape == engine.Singleton {
		id = spec.Key
	}

	var body map[string]any
	if r.Body != nil && (r.Method == http.MethodPost || r.Method == http.MethodPut) {
		d := json.NewDecoder(r.Body)
		d.UseNumber()
		_ = d.Decode(&body)
		f.bodies[r.Method+" "+spec.Key] = body
	}

	switch {
	case r.Method == http.MethodGet && (id == "" && spec.Shape != engine.Singleton):
		ids := make([]string, 0, len(f.rows[spec.Key]))
		for id := range f.rows[spec.Key] {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		list := []any{}
		for _, id := range ids {
			list = append(list, f.rows[spec.Key][id])
		}
		reply(w, 200, map[string]any{"organization": wrap(nil)["organization"], "data": list, "truncated": f.truncated[spec.Key]}, "")
	case r.Method == http.MethodGet:
		row, ok := f.rows[spec.Key][id]
		if !ok {
			fail(w, 404, "not_found", "No row found.")
			return
		}
		reply(w, 200, wrap(row), etag(spec, row))
	case r.Method == http.MethodPost:
		f.create(w, spec, body)
	case r.Method == http.MethodPut:
		f.update(w, r, spec, id, body)
	case r.Method == http.MethodDelete:
		row, ok := f.rows[spec.Key][id]
		if !ok {
			fail(w, 404, "not_found", "No row found.")
			return
		}
		if im := r.Header.Get("If-Match"); im != "" && im != etag(spec, row) {
			reply(w, 412, map[string]any{"organization": wrap(nil)["organization"], "data": row, "error": "precondition_failed", "message": "The row changed after the version If-Match names; nothing was written."}, etag(spec, row))
			return
		}
		delete(f.rows[spec.Key], id)
		reply(w, 200, wrap("Deleted."), "")
	}
}

func (f *Fake) create(w http.ResponseWriter, spec engine.Spec, body map[string]any) {
	row := map[string]any{}
	allowed := map[string]engine.Attr{}
	for _, a := range spec.Attrs {
		if !a.Computed && !a.UpdateOnly {
			allowed[a.Name] = a
		}
	}
	for k := range body {
		if _, ok := allowed[k]; !ok {
			fail(w, 422, "unprocessable_entity", k+" is not an argument this tool declares.")
			return
		}
	}
	for _, a := range spec.Attrs {
		v, present := body[a.Name]
		if a.Required && (!present || v == nil) {
			fail(w, 422, "unprocessable_entity", a.Name+" is required.")
			return
		}
		switch {
		case a.Secret:
			row[a.Name+"_set"] = present
		case a.NotRead:
		case present && a.Normalize != nil && v != nil:
			row[a.Name] = a.Normalize(fmt.Sprint(v))
		case present:
			row[a.Name] = v
		default:
			row[a.Name] = zero(a)
		}
	}
	if spec.Shape == engine.Generated {
		f.nextID++
		row["id"] = fmt.Sprintf("00000000-0000-0000-0000-%012d", f.nextID)
	}
	id := f.rowID(spec, row)
	if existing, dup := f.rows[spec.Key][id]; dup && spec.Shape != engine.Generated {
		reply(w, 409, map[string]any{"error": "conflict", "message": "has already been taken", "existing": existing}, "")
		return
	}
	if spec.Shape != engine.Generated {
		row["id"] = id
	}
	f.rows[spec.Key][id] = row
	w.Header().Set("Location", "/api/v1/config/"+spec.Key+"/"+id)
	reply(w, 201, wrap(row), etag(spec, row))
}

func (f *Fake) update(w http.ResponseWriter, r *http.Request, spec engine.Spec, id string, body map[string]any) {
	row, ok := f.rows[spec.Key][id]
	if !ok && spec.Shape == engine.Singleton {
		row = blank(spec)
		f.rows[spec.Key][id] = row
		ok = true
	}
	if !ok {
		fail(w, 404, "not_found", "No row found.")
		return
	}
	if fn := f.beforeWrite[spec.Key]; fn != nil {
		fn(row)
		delete(f.beforeWrite, spec.Key)
	}
	if im := r.Header.Get("If-Match"); im != "" && im != etag(spec, row) {
		reply(w, 412, map[string]any{"organization": wrap(nil)["organization"], "data": row, "error": "precondition_failed", "message": "The row changed after the version If-Match names; nothing was written."}, etag(spec, row))
		return
	}
	byName := map[string]engine.Attr{}
	for _, a := range spec.Attrs {
		if !a.Computed && !a.CreateOnly {
			byName[a.Name] = a
		}
	}
	for k, v := range body {
		a, ok := byName[k]
		if !ok {
			fail(w, 422, "unprocessable_entity", k+" is not an argument this tool declares.")
			return
		}
		if v == nil && !a.Clearable {
			fail(w, 422, "unprocessable_entity", k+" cannot be null; omit it to keep the stored value.")
			return
		}
	}
	for k, v := range body {
		if byName[k].Secret {
			row[k+"_set"] = true
			continue
		}
		row[k] = v
	}
	reply(w, 200, wrap(row), etag(spec, row))
}
