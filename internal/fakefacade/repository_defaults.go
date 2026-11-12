package fakefacade

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"sort"
	"strings"
)

// defaultsModel is the organization's repository defaults: the platform answers
// them as one flat object, applies a write only against a preview (428, then a
// fingerprinted request, 409 when what would move changed), and hides the row of
// a child organization that follows its parent behind a 404.
type defaultsModel struct {
	source       string
	parentName   string
	stopped      bool
	parentValues map[string]any
	children     []map[string]any
	refusal      string
	failPreviews int
	changeOnPut  map[string]any
	appearOnPut  []string
	previews     int
	fingerprint  string
	ifMatches    []string
}

var (
	defaultsSettings = []string{"fix_runner", "platform_fallback", "auto_dispatch", "draft_prs", "check_network", "allow_ci_config"}
	defaultsAllowed  = append(slices.Clone(defaultsSettings), "confirm", "follow_parent", "stop_following", "expected_fingerprint")
)

// DefaultsSource sets where the defaults come from: "built_in", "own" or
// "parent" (a child organization that follows parentName, which reads 404 on
// its own row). The parent's values are the ones follow_parent returns to.
func (f *Fake) DefaultsSource(source, parentName string, parentValues map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.dm.source, f.dm.parentName = source, parentName
	f.dm.parentValues = builtInDefaults()
	for k, v := range parentValues {
		f.dm.parentValues[k] = v
	}
	if source == "parent" {
		for k, v := range f.dm.parentValues {
			f.defaults[k] = v
		}
	}
}

// Defaults answers the six values the defaults hold now.
func (f *Fake) Defaults() map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[string]any{}
	for _, k := range defaultsSettings {
		out[k] = f.defaults[k]
	}
	out["source"] = f.dm.source
	out["stopped"] = f.dm.stopped
	return out
}

// RefuseDefaults makes every preview carry this refusal.
func (f *Fake) RefuseDefaults(sentence string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.dm.refusal = sentence
}

// FollowingChildren lists the child organizations a preview names.
func (f *Fake) FollowingChildren(children ...map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.dm.children = children
}

// RefusePreviews answers the next n requests that ask for a preview with the
// refusal of an api:config_read key, which can plan but not write.
func (f *Fake) RefusePreviews(n int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.dm.failPreviews = n
}

// ChangeDefaultsOnNextPut edits the defaults the way a person would in the
// browser between Terraform's refresh and its next PUT of them.
func (f *Fake) ChangeDefaultsOnNextPut(values map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.dm.changeOnPut = values
}

// RepositoryAppearsOnNextFingerprintedPut adds a repository that follows the
// defaults right before the next write is judged, so the preview the write
// carries no longer describes what would move.
func (f *Fake) RepositoryAppearsOnNextFingerprintedPut(repo string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.dm.appearOnPut = append(f.dm.appearOnPut, repo)
}

// Previews counts the 428s the defaults answered.
func (f *Fake) Previews() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.dm.previews
}

// LastFingerprint is the fingerprint of the latest preview the fake gave.
func (f *Fake) LastFingerprint() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.dm.fingerprint
}

// IfMatches are the If-Match values the defaults' PUTs carried, in order.
func (f *Fake) IfMatches() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.dm.ifMatches)
}

// PutBodies are the bodies of every PUT to key, in order.
func (f *Fake) PutBodies(key string) []map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.history[key])
}

func (f *Fake) defaultsRow() map[string]any {
	row := map[string]any{}
	for _, k := range defaultsSettings {
		row[k] = f.defaults[k]
	}
	row["source"] = f.dm.source
	row["inherited_from"] = nil
	row["parent_name"] = nil
	if f.dm.parentName != "" {
		row["parent_name"] = f.dm.parentName
	}
	row["stopped"] = f.dm.stopped
	row["updated_at"] = nil
	if f.dm.source == "own" {
		row["updated_at"] = "2026-11-12T10:00:00Z"
	}
	return row
}

func rowTag(row map[string]any) string {
	b, _ := json.Marshal(row)
	sum := sha256.Sum256(b)
	return `"c1-` + hex.EncodeToString(sum[:]) + `"`
}

type defaultsAnalysis struct {
	preview     map[string]any
	fingerprint string
	to          map[string]any
	kind        string
	needs       []string
	refusals    []string
}

func (f *Fake) analyzeDefaults(body map[string]any) (*defaultsAnalysis, string) {
	kind := "set"
	switch {
	case body["follow_parent"] == true:
		kind = "follow_parent"
	case body["stop_following"] == true:
		kind = "stop_following"
	}
	if kind == "follow_parent" && f.dm.parentName == "" {
		return nil, "Following a parent's repository defaults is only for an organization that has a parent."
	}
	from := map[string]any{}
	to := map[string]any{}
	for _, k := range defaultsSettings {
		from[k], to[k] = f.defaults[k], f.defaults[k]
	}
	sets := map[string]any{}
	for _, k := range defaultsSettings {
		if v, ok := body[k]; ok {
			sets[k] = v
		}
	}
	switch kind {
	case "set":
		if len(sets) == 0 {
			return nil, "Name at least one repository default to change."
		}
		for k, v := range sets {
			to[k] = v
		}
	case "follow_parent":
		to = map[string]any{}
		for _, k := range defaultsSettings {
			to[k] = f.dm.parentValues[k]
		}
	case "stop_following":
		to = builtInDefaults()
	}
	changes := map[string]any{}
	var changed []string
	for _, k := range defaultsSettings {
		if fmt.Sprint(from[k]) != fmt.Sprint(to[k]) {
			changes[k] = map[string]any{"from": from[k], "to": to[k]}
			changed = append(changed, k)
		}
	}
	repos := map[string]bool{}
	moves := map[string]bool{}
	for _, row := range f.rows["repo_settings"] {
		repo := strings.ToLower(fmt.Sprint(row["repo"]))
		repos[repo] = true
		for _, k := range changed {
			if !slices.Contains(heldOf(row), k) {
				moves[repo] = true
			}
		}
	}
	moved := []string{}
	for repo := range moves {
		moved = append(moved, repo)
	}
	sort.Strings(moved)
	kept := 0
	if len(changed) > 0 {
		kept = len(repos) - len(moved)
	}
	var needs []string
	if c, ok := changes["allow_ci_config"].(map[string]any); ok && c["to"] == true {
		needs = append(needs, "ci_config")
	}
	if c, ok := changes["check_network"].(map[string]any); ok && c["to"] == true {
		needs = append(needs, "check_network")
	}
	refusals := []string{}
	if f.dm.refusal != "" {
		refusals = append(refusals, f.dm.refusal)
	}
	leaves := f.dm.source == "parent" && kind == "set"
	children := f.dm.children
	if children == nil {
		children = []map[string]any{}
	}
	stable, _ := json.Marshal([]any{kind, sets, from, to, moved, children, leaves, f.dm.source})
	sum := sha256.Sum256(stable)
	fingerprint := hex.EncodeToString(sum[:])
	if needs == nil {
		needs = []string{}
	}
	return &defaultsAnalysis{
		fingerprint: fingerprint, to: to, kind: kind, needs: needs, refusals: refusals,
		preview: map[string]any{
			"fingerprint": fingerprint, "changes": changes, "moved": moved, "kept": kept,
			"differs_by_branch": []any{}, "children": children, "needs_confirm": needs,
			"refusals": refusals, "leaves_parent": leaves, "applied": false,
		},
	}, ""
}

var confirmSentences = map[string]string{
	"ci_config":     "Turning on fix changes to CI configuration for every repository that follows the default needs confirmation: pass confirm ci_config.",
	"check_network": "Turning on network access for checks in every repository that follows the default needs confirmation: pass confirm check_network.",
}

func (f *Fake) serveDefaults(w http.ResponseWriter, r *http.Request, body map[string]any) {
	if f.dm.source == "missing" {
		fail(w, 404, "not_found", "No such configuration resource: repository_defaults.")
		return
	}
	switch r.Method {
	case http.MethodGet:
		if f.dm.source == "parent" {
			fail(w, 404, "not_found", "This organization has no repository_defaults of its own; it uses its parent organization's. Write it here to configure this organization's own.")
			return
		}
		row := f.defaultsRow()
		reply(w, 200, wrap(row), rowTag(row))
	case http.MethodPut:
		f.putDefaults(w, r, body)
	default:
		fail(w, 405, "method_not_allowed", "repository_defaults is a singleton: read it or change it.")
	}
}

func (f *Fake) putDefaults(w http.ResponseWriter, r *http.Request, body map[string]any) {
	f.dm.ifMatches = append(f.dm.ifMatches, r.Header.Get("If-Match"))
	_, fingerprinted := body["expected_fingerprint"]
	if f.readOnly || (!fingerprinted && f.dm.failPreviews > 0) {
		if !f.readOnly {
			f.dm.failPreviews--
		}
		fail(w, 403, "read_only_key", "This API key holds api:config_read, which reads configuration and never changes it.")
		return
	}
	if f.dm.changeOnPut != nil {
		for k, v := range f.dm.changeOnPut {
			f.defaults[k] = v
		}
		f.dm.changeOnPut = nil
	}
	for k := range body {
		if !slices.Contains(defaultsAllowed, k) {
			fail(w, 422, "unprocessable_entity", k+" is not an argument this tool declares.")
			return
		}
	}
	if im := r.Header.Get("If-Match"); im != "" {
		if f.dm.source == "parent" {
			fail(w, 412, "precondition_failed", "If-Match named a version of a row this organization does not own.")
			return
		}
		row := f.defaultsRow()
		if strings.TrimPrefix(im, "W/") != rowTag(row) {
			reply(w, 412, map[string]any{"organization": wrap(nil)["organization"], "data": row, "error": "precondition_failed", "message": "The row changed after the version If-Match names; nothing was written."}, rowTag(row))
			return
		}
	}
	if fingerprinted {
		for _, repo := range f.dm.appearOnPut {
			f.addFollowingRepo(repo)
		}
		f.dm.appearOnPut = nil
	}
	analysis, refusal := f.analyzeDefaults(body)
	if analysis == nil {
		fail(w, 422, "unprocessable_entity", refusal)
		return
	}
	if !fingerprinted {
		f.dm.previews++
		f.dm.fingerprint = analysis.fingerprint
		reply(w, 428, map[string]any{"error": "precondition_required", "message": "This change is applied against a preview you have read, and nothing was written.", "preview": map[string]any{"data": analysis.preview}}, "")
		return
	}
	if want, _ := body["expected_fingerprint"].(string); want != analysis.fingerprint {
		f.dm.fingerprint = analysis.fingerprint
		reply(w, 409, map[string]any{"error": "conflict", "message": "What would move changed since that preview, so nothing was written.", "preview": map[string]any{"data": analysis.preview}}, "")
		return
	}
	if len(analysis.refusals) > 0 {
		fail(w, 422, "unprocessable_entity", analysis.refusals[0])
		return
	}
	given, _ := body["confirm"].([]any)
	for _, need := range analysis.needs {
		if !slices.Contains(given, any(need)) {
			fail(w, 422, "unprocessable_entity", confirmSentences[need])
			return
		}
	}
	switch analysis.kind {
	case "set":
		for _, k := range defaultsSettings {
			if v, ok := body[k]; ok {
				f.defaults[k] = v
			}
		}
		f.dm.source = "own"
	case "follow_parent":
		for k, v := range analysis.to {
			f.defaults[k] = v
		}
		f.dm.source, f.dm.stopped = "parent", false
	case "stop_following":
		for k, v := range analysis.to {
			f.defaults[k] = v
		}
		f.dm.source, f.dm.stopped = "built_in", true
	}
	row := f.defaultsRow()
	reply(w, 200, wrap(row), rowTag(row))
}

// addFollowingRepo stores a repository row that holds nothing itself.
func (f *Fake) addFollowingRepo(repo string) {
	f.nextID++
	id := fmt.Sprintf("00000000-0000-0000-0000-%012d", f.nextID)
	f.rows["repo_settings"][id] = map[string]any{"id": id, "repo": repo, "branch": "", "set_here": []any{}}
}
