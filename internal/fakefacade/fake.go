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
	"slices"
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
	refused     map[string]string
	onWrite     map[string]func(map[string]any)
	truncated   map[string]bool
	etagMode    etagMode
	defaults    map[string]any
	legacy      map[string]bool
	dm          defaultsModel
	em          effectiveModel
	history     map[string][]map[string]any
	accepted    []string
}

// New starts a fake for specs; the server stops when the test ends.
func New(t *testing.T, specs []engine.Spec) *Fake {
	t.Helper()
	f := &Fake{specs: map[string]engine.Spec{}, rows: map[string]map[string]map[string]any{}, beforeWrite: map[string]func(map[string]any){}, refused: map[string]string{}, onWrite: map[string]func(map[string]any){}, calls: map[string]int{}, bodies: map[string]map[string]any{}, truncated: map[string]bool{}, defaults: builtInDefaults(), legacy: map[string]bool{}, history: map[string][]map[string]any{}, dm: defaultsModel{source: "built_in", parentValues: builtInDefaults()}}
	for _, s := range specs {
		f.specs[s.Key] = s
		f.rows[s.Key] = map[string]map[string]any{}
		// The platform answers defaults for most singletons before any write.
		if s.Shape == engine.Singleton && !s.StartsEmpty {
			f.rows[s.Key][s.Key] = blank(s)
			if isSecurity(s.Key) {
				securitySeed(s, f.rows[s.Key][s.Key])
			}
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

// Seed stores a row for a read-only collection, which has no write door to
// create one through. The row's id is its "id" field.
func (f *Fake) Seed(key string, row map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rows[key][fmt.Sprint(row["id"])] = row
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

// AllowWrites undoes ReadOnly, for a test that must clean up after proving the refusal.
func (f *Fake) AllowWrites() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.readOnly = false
}

// RefusePlan makes every request for key, reads included, answer the 403 the
// platform gives an organization whose plan lacks the feature the resource needs:
// error "forbidden" with the platform's own sentence. A read-only key still gets
// its own refusal for a write, as it does on the platform, where that check runs
// first. An empty sentence lifts the refusal.
func (f *Fake) RefusePlan(key, sentence string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if sentence == "" {
		delete(f.refused, key)
		return
	}
	f.refused[key] = sentence
}

// OnWrite registers fn to run on a row of key after a create or an update stored
// it and before it is answered: the platform's own normalization of what it was
// sent (sorted lists, canonical times, nulls), which the generic fake does not
// know.
func (f *Fake) OnWrite(key string, fn func(row map[string]any)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.onWrite[key] = fn
}

type etagMode int

const (
	strongETags etagMode = iota
	noETags
	weakETags
)

// OmitETags makes every answer carry no ETag, or restores them.
func (f *Fake) OmitETags(omit bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.etagMode = strongETags
	if omit {
		f.etagMode = noETags
	}
}

// WeakETags serves W/"..." ETags, the form a proxy that compresses the answer
// gives, and compares If-Match weakly, as the platform does.
func (f *Fake) WeakETags() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.etagMode = weakETags
}

// tag is the ETag an answer carries for row.
func (f *Fake) tag(spec engine.Spec, row map[string]any) string {
	switch f.etagMode {
	case noETags:
		return ""
	case weakETags:
		return "W/" + etag(spec, row)
	}
	return etag(spec, row)
}

// stale reports an If-Match naming another version of row; a leading W/ is
// ignored, since the tag is a content hash either way.
func stale(ifMatch string, spec engine.Spec, row map[string]any) bool {
	return ifMatch != "" && strings.TrimPrefix(ifMatch, "W/") != etag(spec, row)
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
		case !a.Computed && !a.NotRead && !a.WriteOnly && !a.Kept:
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
	// A null that is a real setting (engine.Attr.NullAs) is what the platform
	// stores before anyone sets it.
	if a.Clearable || a.NullAs != "" {
		return nil
	}
	switch a.Kind {
	case engine.String:
		return ""
	case engine.Int, engine.Float:
		return 0
	case engine.Bool:
		// Every enabled column on the platform defaults to true.
		return a.Name == "enabled" || a.Name == "platform_fallback"
	case engine.StringList:
		return []any{}
	case engine.JSON:
		return map[string]any{}
	}
	return nil
}

// emptyClearableList is a [] sent for a Clearable list. The platform stores
// null for it (log_inline_fields: "null or [] clears it"), and answers null.
func emptyClearableList(a engine.Attr, v any) bool {
	list, ok := v.([]any)
	return ok && a.Clearable && a.Kind == engine.StringList && len(list) == 0
}

// merge folds an object into a stored one; an empty string clears its key.
func merge(stored, sent any) any {
	out := map[string]any{}
	if m, ok := stored.(map[string]any); ok {
		for k, v := range m {
			out[k] = v
		}
	}
	m, ok := sent.(map[string]any)
	if !ok {
		return sent
	}
	for k, v := range m {
		if v == "" {
			delete(out, k)
			continue
		}
		out[k] = v
	}
	return out
}

// Rows counts the stored rows of one resource key.
func (f *Fake) Rows(key string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.rows[key])
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

	if sentence, ok := f.refused[spec.Key]; ok {
		fail(w, 403, "forbidden", sentence)
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
		if r.Method == http.MethodPut {
			f.history[spec.Key] = append(f.history[spec.Key], body)
		}
	}
	if spec.Key == "repository_defaults" {
		f.serveDefaults(w, r, body)
		return
	}
	if spec.Key == "effective_repo_settings" && r.Method == http.MethodGet {
		f.serveEffective(w, r)
		return
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
			list = append(list, f.present(spec.Key, f.rows[spec.Key][id]))
		}
		reply(w, 200, map[string]any{"organization": wrap(nil)["organization"], "data": list, "truncated": f.truncated[spec.Key]}, "")
	case r.Method == http.MethodGet:
		row, ok := f.rows[spec.Key][id]
		if !ok {
			fail(w, 404, "not_found", "No row found.")
			return
		}
		reply(w, 200, wrap(f.present(spec.Key, row)), f.tag(spec, row))
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
		if im := r.Header.Get("If-Match"); stale(im, spec, row) {
			reply(w, 412, map[string]any{"organization": wrap(nil)["organization"], "data": row, "error": "precondition_failed", "message": "The row changed after the version If-Match names; nothing was written."}, f.tag(spec, row))
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
	if msg := bothFences(spec, body); msg != "" {
		fail(w, 422, "unprocessable_entity", msg)
		return
	}
	if msg := f.followsRefusal(spec.Key, body); msg != "" {
		fail(w, 422, "unprocessable_entity", msg)
		return
	}
	if isSecurity(spec.Key) {
		if status, msg := f.securityRefusal(spec, "", nil, body); status != 0 {
			fail(w, status, map[int]string{409: "conflict", 422: "unprocessable_entity"}[status], msg)
			return
		}
	}
	for _, a := range spec.Attrs {
		if a.WriteOnly || len(a.Complement) > 0 {
			continue
		}
		v, present := body[a.Name]
		if a.Required && (!present || v == nil) {
			fail(w, 422, "unprocessable_entity", a.Name+" is required.")
			return
		}
		switch {
		case a.Secret:
			row[a.Name+"_set"] = present && v != nil && v != ""
		case a.NotRead:
		case present && a.Normalize != nil && v != nil:
			row[a.Name] = a.Normalize(fmt.Sprint(v))
		case present && emptyClearableList(a, v):
			row[a.Name] = nil
		case present:
			row[a.Name] = v
		default:
			row[a.Name] = zero(a)
		}
	}
	for _, a := range spec.Attrs {
		if a.Secret {
			setHost(spec, row, a, body[a.Name])
		}
	}
	discardSecrets(spec, row)
	mirrorFence(spec, row, body)
	if spec.Shape == engine.Generated {
		f.nextID++
		row["id"] = fmt.Sprintf("00000000-0000-0000-0000-%012d", f.nextID)
	}
	id := f.rowID(spec, row)
	if f.following(spec.Key) {
		f.rows[spec.Key][id] = row
		f.finishRepoCreate(row, body)
	} else if spec.SetHere != "" {
		delete(row, spec.SetHere)
	}
	if existing, dup := f.rows[spec.Key][id]; dup && spec.Shape != engine.Generated {
		reply(w, 409, map[string]any{"error": "conflict", "message": "has already been taken", "existing": existing}, "")
		return
	}
	if spec.Shape != engine.Generated {
		row["id"] = id
	}
	if isSecurity(spec.Key) {
		f.securityStore(spec, row, body, true)
	}
	if fn := f.onWrite[spec.Key]; fn != nil {
		fn(row)
	}
	f.rows[spec.Key][id] = row
	w.Header().Set("Location", "/api/v1/config/"+spec.Key+"/"+id)
	reply(w, 201, wrap(f.present(spec.Key, row)), f.tag(spec, row))
}

// setHost answers the companion <secret>_host a platform derives from a secret
// URL: scheme, host and a non-default port, null when no value is saved.
func setHost(spec engine.Spec, row map[string]any, secret engine.Attr, sent any) {
	for _, a := range spec.Attrs {
		if !a.Computed || a.Name != secret.Name+"_host" {
			continue
		}
		raw, _ := sent.(string)
		u, err := url.Parse(raw)
		if raw == "" || err != nil || u.Host == "" {
			row[a.Name] = nil
			return
		}
		row[a.Name] = u.Scheme + "://" + u.Host
	}
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
	if im := r.Header.Get("If-Match"); stale(im, spec, row) {
		reply(w, 412, map[string]any{"organization": wrap(nil)["organization"], "data": row, "error": "precondition_failed", "message": "The row changed after the version If-Match names; nothing was written."}, f.tag(spec, row))
		return
	}
	byName := map[string]engine.Attr{}
	previous := map[string]any{}
	for _, a := range spec.Attrs {
		if !a.Computed && !a.CreateOnly {
			byName[a.Name] = a
		}
		previous[a.Name] = row[a.Name]
	}
	if f.legacy[spec.Key] {
		delete(byName, "follows_default")
	}
	for k, v := range body {
		a, ok := byName[k]
		if !ok {
			fail(w, 422, "unprocessable_entity", k+" is not an argument this tool declares.")
			return
		}
		if v == nil && f.following(spec.Key) && slices.Contains(repoWide, k) {
			continue
		}
		if v == nil && !a.Clearable && a.NullAs == "" {
			fail(w, 422, "unprocessable_entity", k+" cannot be null; omit it to keep the stored value.")
			return
		}
	}
	if msg := bothFences(spec, body); msg != "" {
		fail(w, 422, "unprocessable_entity", msg)
		return
	}
	if msg := f.followsRefusal(spec.Key, body); msg != "" {
		fail(w, 422, "unprocessable_entity", msg)
		return
	}
	if isSecurity(spec.Key) {
		if status, msg := f.securityRefusal(spec, id, row, body); status != 0 {
			fail(w, status, map[int]string{409: "conflict", 422: "unprocessable_entity"}[status], msg)
			return
		}
	}
	before := heldOf(row)
	for k, v := range body {
		a := byName[k]
		if a.WriteOnly || (v == nil && f.following(spec.Key) && slices.Contains(repoWide, k)) {
			continue
		}
		if a.Secret {
			row[k+"_set"] = v != nil && v != ""
			setHost(spec, row, a, v)
			continue
		}
		if a.MergedObject {
			row[k] = merge(row[k], v)
			continue
		}
		if v != nil && a.Normalize != nil {
			v = a.Normalize(fmt.Sprint(v))
		}
		if emptyClearableList(a, v) {
			v = nil
		}
		row[k] = v
	}
	discardSecrets(spec, row)
	mirrorFence(spec, row, body)
	if f.following(spec.Key) {
		f.finishRepoUpdate(row, body, before)
	}
	carryIgnored(spec, row, previous)
	if isSecurity(spec.Key) {
		f.securityStore(spec, row, body, false)
	}
	if fn := f.onWrite[spec.Key]; fn != nil {
		fn(row)
	}
	reply(w, 200, wrap(f.present(spec.Key, row)), f.tag(spec, row))
}

// carryIgnored keeps the entries of a list the platform holds through every
// write (engine.Attr.IgnoredValues) in the list a write just stored.
func carryIgnored(spec engine.Spec, row, previous map[string]any) {
	for _, a := range spec.Attrs {
		if len(a.IgnoredValues) == 0 {
			continue
		}
		old, _ := previous[a.Name].([]any)
		cur, _ := row[a.Name].([]any)
		for _, e := range old {
			if slices.Contains(a.IgnoredValues, fmt.Sprint(e)) && !slices.Contains(cur, e) {
				cur = append(cur, e)
			}
		}
		if cur != nil {
			row[a.Name] = cur
		}
	}
}

func (f *Fake) followsRefusal(key string, body map[string]any) string {
	if !f.following(key) {
		return ""
	}
	return followsRefusal(body)
}

// bothFences is the platform's refusal of a repository write that names the
// fence as one directory and as a list.
func bothFences(spec engine.Spec, body map[string]any) string {
	if spec.Key != "repo_settings" {
		return ""
	}
	one, many := body["path_prefix"], body["path_prefixes"]
	if one != nil && many != nil {
		return "Send path_prefixes or path_prefix, not both."
	}
	return ""
}

// mirrorFence keeps path_prefix equal to the list the way the platform does:
// writing one directory replaces the list with it, and path_prefix answers the
// list's only entry, null for any other length.
func mirrorFence(spec engine.Spec, row, body map[string]any) {
	if spec.Key != "repo_settings" {
		return
	}
	if one, ok := body["path_prefix"].(string); ok && one != "" {
		row["path_prefixes"] = []any{one}
		return
	}
	if _, ok := body["path_prefixes"]; !ok {
		return
	}
	if list, ok := row["path_prefixes"].([]any); ok && len(list) == 1 {
		row["path_prefix"] = list[0]
		return
	}
	row["path_prefix"] = nil
}

// discardSecrets drops a stored secret the platform discards for the row's
// value of a discriminating field (engine.Attr.DroppedWhenAttr).
func discardSecrets(spec engine.Spec, row map[string]any) {
	for _, a := range spec.Attrs {
		if a.Secret && a.DroppedWhenAttr != "" && row[a.DroppedWhenAttr] == a.DroppedWhenValue {
			row[a.Name+"_set"] = false
		}
	}
}
