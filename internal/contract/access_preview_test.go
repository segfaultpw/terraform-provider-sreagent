package contract

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/segfaultpw/terraform-provider-sreagent/internal/engine"
)

// accessFixtures are the previews the platform emits for a change of what a
// parent gives one child of one GitHub account, copied from its code: a narrowing
// of one account, a change of sharing alone (which names no account) and the
// answer to a stale fingerprint. The contract publishes preview as an object with
// the keys under preview.data; the provider reads those keys.
func accessFixtures() (map[string]map[string]any, error) {
	raw, err := os.ReadFile("testdata/child_access_preview.json")
	if err != nil {
		return nil, err
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	out := map[string]map[string]any{}
	for _, name := range []string{"preview", "sharing_only", "stale"} {
		var m map[string]any
		if err := json.Unmarshal(doc[name], &m); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		out[name] = m
	}
	return out, nil
}

// conformance reports every way value departs from a published schema: a type, a
// missing required key, a value outside an enum, a key the schema does not
// publish. It reads the keywords the platform uses (type, properties, items,
// required, enum) and ignores the rest.
func conformance(value any, schema map[string]any, where string) []string {
	if value == nil {
		if !slices.Contains(typeNames(schema), "null") && len(typeNames(schema)) > 0 {
			return []string{where + " is null, the schema does not allow it"}
		}
		return nil
	}
	var out []string
	if names := slices.DeleteFunc(slices.Clone(typeNames(schema)), func(n string) bool { return n == "null" }); len(names) > 0 {
		ok := false
		for _, n := range names {
			ok = ok || matches(n, value)
		}
		if !ok {
			return []string{fmt.Sprintf("%s is %T, published as %v", where, value, names)}
		}
	}
	if enum, ok := schema["enum"].([]any); ok && !slices.Contains(enum, value) {
		out = append(out, fmt.Sprintf("%s is %v, which is not in its enum %v", where, value, enum))
	}
	switch v := value.(type) {
	case map[string]any:
		props, _ := schema["properties"].(map[string]any)
		if props == nil {
			return out
		}
		required, _ := schema["required"].([]any)
		for _, r := range required {
			if _, ok := v[fmt.Sprint(r)]; !ok {
				out = append(out, where+"."+fmt.Sprint(r)+" is missing")
			}
		}
		for _, key := range slices.Sorted(maps.Keys(v)) {
			sub, ok := props[key].(map[string]any)
			if !ok {
				out = append(out, where+"."+key+" is not published")
				continue
			}
			out = append(out, conformance(v[key], sub, where+"."+key)...)
		}
	case []any:
		if items, ok := schema["items"].(map[string]any); ok {
			for i, e := range v {
				out = append(out, conformance(e, items, fmt.Sprintf("%s[%d]", where, i))...)
			}
		}
	}
	return out
}

func typeNames(schema map[string]any) []string {
	switch t := schema["type"].(type) {
	case string:
		return []string{t}
	case []any:
		out := make([]string, 0, len(t))
		for _, e := range t {
			out = append(out, fmt.Sprint(e))
		}
		return out
	}
	return nil
}

func matches(kind string, v any) bool {
	switch kind {
	case "string":
		_, ok := v.(string)
		return ok
	case "integer", "number":
		_, ok := v.(float64)
		return ok
	case "boolean":
		_, ok := v.(bool)
		return ok
	case "object":
		_, ok := v.(map[string]any)
		return ok
	case "array":
		_, ok := v.([]any)
		return ok
	}
	return true
}

// accessSchemaProblems holds a published child access preview schema to the wire
// shape: the keys the provider reads are under preview.data and every one of them
// is published and required, so a renamed or dropped key is noticed. Unlike the
// repository defaults' preview, which the platform publishes as a bare object,
// this one is published in full, and a schema that is not is drift.
func accessSchemaProblems(preview map[string]any) []string {
	props, published := preview["properties"].(map[string]any)
	if !published {
		return []string{"publishes no preview properties, but the provider reads the preview under preview.data"}
	}
	data, ok := props["data"].(map[string]any)
	if !ok {
		return []string{"publishes preview properties without data, but the provider reads the preview under preview.data"}
	}
	dataProps, published := data["properties"].(map[string]any)
	if !published {
		return []string{"publishes preview.data without properties, but the provider reads its keys"}
	}
	required, _ := data["required"].([]any)
	var out []string
	for _, key := range engine.AccessPreviewKeys() {
		if _, ok := dataProps[key]; !ok {
			out = append(out, fmt.Sprintf("publishes preview.data without %s, which the provider reads", key))
		}
		if !slices.Contains(required, any(key)) {
			out = append(out, fmt.Sprintf("does not require preview.data.%s, which the provider reads", key))
		}
	}
	for _, key := range engine.AccessPreviewOptionalKeys() {
		if _, ok := dataProps[key]; !ok {
			out = append(out, fmt.Sprintf("publishes preview.data without %s", key))
		}
	}
	return out
}

// checkAccessPreviewShape holds the keys of preview.data the provider reads for a
// child access preview to what the platform publishes: the fixtures of its
// answers, which must be readable and conform to the published schema, and the
// 428 and 409 schemas.
func checkAccessPreviewShape(published map[string]map[string]any, add func(string, ...any)) {
	fixtures, err := accessFixtures()
	if err != nil {
		add("the child access preview fixtures cannot be read: %v", err)
		return
	}
	for _, name := range slices.Sorted(maps.Keys(fixtures)) {
		if err := engine.CheckAccessPreview(fixtures[name]); err != nil {
			add("the %s fixture is not a preview the provider can read: %v", name, err)
		}
	}
	if stops, _ := fixtures["preview"]["stops_reaching"].(map[string]any); stops["count"] == float64(0) {
		add("the narrowing fixture stops reaching nothing, so the reach keys are not pinned")
	}
	if fixtures["preview"]["account"] == nil || fixtures["sharing_only"]["account"] != nil {
		add("the fixtures must hold a preview that names an account and one of sharing alone that names none")
	}
	if fixtures["stale"]["stale"] != true {
		add("the stale fixture is not marked stale")
	}
	if refusals, _ := fixtures["stale"]["refusals"].([]any); len(refusals) == 0 {
		add("no fixture holds a refusal, so the refusal keys are not pinned")
	}
	for _, status := range []string{"428", "409"} {
		schema := published[status]
		for _, problem := range accessSchemaProblems(schema) {
			add("the %s answer %s", status, problem)
		}
		props, _ := schema["properties"].(map[string]any)
		data, _ := props["data"].(map[string]any)
		if data == nil {
			continue
		}
		for _, name := range slices.Sorted(maps.Keys(fixtures)) {
			for _, problem := range conformance(fixtures[name], data, "fixture "+name) {
				add("the %s answer publishes a schema the %s fixture departs from: %s", status, name, problem)
			}
		}
	}
}

// childAccessSpec is the spec of what a parent gives one child of one GitHub
// account, as the provider declares it: a row the platform has no door to create
// or delete, addressed by the child in the path and the account in the query,
// adopted by the block and left in place on destroy, and written against a
// preview when the change takes something away.
func childAccessSpec() engine.Spec {
	return engine.Spec{
		Key: "github_child_access", TypeName: "github_child_access", ListName: "github_child_access_rows",
		Shape: engine.PathAndQuery, IDAttr: "child_organization", QueryAttr: "account", Lifecycle: true,
		AdoptOnly: true, Previewed: true, ProbeFingerprint: "preview", AccessPreview: true,
		LeaveOnDestroy: "The child keeps what it was given; change it here or in the app.",
		Attrs: []engine.Attr{
			{Name: "child_organization", Kind: engine.String, Address: true, Required: true, CreateOnly: true},
			{Name: "account", Kind: engine.String, Address: true, Required: true, CreateOnly: true},
			{Name: "mode", Kind: engine.String, UpdateOnly: true, OneOf: []string{"all", "selected", "none"}},
			{Name: "repositories", Kind: engine.StringList, UpdateOnly: true, Recomputed: true, MaxItems: 500},
			{Name: "context_repo_shared", Kind: engine.Bool, UpdateOnly: true},
			{Name: "child_name", Kind: engine.String, Computed: true},
			{Name: "installation_id", Kind: engine.Int, Computed: true},
			{Name: "repository_count", Kind: engine.Int, Computed: true},
			{Name: "details_read", Kind: engine.Bool, Computed: true},
			{Name: "applies", Kind: engine.Bool, Computed: true},
			{Name: "reason", Kind: engine.String, Computed: true},
			{Name: "source", Kind: engine.String, Computed: true},
			{Name: "stopped", Kind: engine.Bool, Computed: true},
			{Name: "own_github", Kind: engine.Bool, Computed: true},
			{Name: "updated_at", Kind: engine.String, Computed: true},
			{Name: "repository_details", Kind: engine.JSON, Computed: true},
			{Name: "preview_fingerprint", Kind: engine.String, Computed: true, Local: true},
		},
	}
}

func TestAChildAccessRowAgreesWithTheContract(t *testing.T) {
	d := doc(t)
	for _, problem := range specProblems(d, childAccessSpec()) {
		t.Error(problem)
	}
}

func TestThePublishedChildAccessPreviewsConformToTheFixtures(t *testing.T) {
	d := doc(t)
	put := d.Paths["/github_child_access/{id}"]["put"]
	fixtures, err := accessFixtures()
	if err != nil {
		t.Fatal(err)
	}
	for _, status := range []string{"428", "409"} {
		schema := put.Responses[status].Content["application/json"].Schema.Properties["preview"]
		properties, ok := schema["properties"].(map[string]any)
		if !ok {
			t.Errorf("the PUT of github_child_access does not publish the preview object of its %s answer", status)
			continue
		}
		data, ok := properties["data"].(map[string]any)
		if !ok {
			t.Errorf("the PUT of github_child_access does not publish preview.data of its %s answer", status)
			continue
		}
		for name, fixture := range fixtures {
			if problems := conformance(fixture, data, name); len(problems) > 0 {
				t.Errorf("%s: %s", status, strings.Join(problems, "; "))
			}
		}
	}
}

// Each change below is one the platform could ship to a row addressed by a path
// and a query, written against a preview, adopted and left in place.
func TestTheContractTestCatchesDriftOfAPathAndQueryRow(t *testing.T) {
	for _, c := range []struct {
		name   string
		mutate func(d *Doc)
		spec   func(s *engine.Spec)
	}{
		{"the contract stops addressing the row by its account", func(d *Doc) {
			r := d.Resources["github_child_access"]
			r.IDArgs = []string{"child_organization"}
			d.Resources["github_child_access"] = r
		}, nil},
		{"the contract addresses the row by another account argument", func(d *Doc) {
			r := d.Resources["github_child_access"]
			r.IDArgs = []string{"child_organization", "installation"}
			d.Resources["github_child_access"] = r
		}, nil},
		{"the path is addressed by an id the spec does not hold", nil, func(s *engine.Spec) { s.IDAttr = "child" }},
		{"the row stops being addressed in the query", nil, func(s *engine.Spec) { s.QueryAttr = "installation" }},
		{"the spec forgets its fingerprint probe", nil, func(s *engine.Spec) { s.ProbeFingerprint = "" }},
		{"the contract stops publishing the fingerprint argument", func(d *Doc) {
			r := d.Resources["github_child_access"]
			r.ActionArgs = nil
			d.Resources["github_child_access"] = r
		}, nil},
		{"the PUT stops documenting the 428", func(d *Doc) {
			op := d.Paths["/github_child_access/{id}"]["put"]
			delete(op.Responses, "428")
			d.Paths["/github_child_access/{id}"]["put"] = op
		}, nil},
		{"the PUT stops documenting the 409", func(d *Doc) {
			op := d.Paths["/github_child_access/{id}"]["put"]
			delete(op.Responses, "409")
			d.Paths["/github_child_access/{id}"]["put"] = op
		}, nil},
		{"the 428 publishes the preview as a bare object", func(d *Doc) {
			op := d.Paths["/github_child_access/{id}"]["put"]
			op.Responses["428"].Content["application/json"].Schema.Properties["preview"] = map[string]any{"type": "object"}
			d.Paths["/github_child_access/{id}"]["put"] = op
		}, nil},
		{"the 409 publishes a preview without a key the provider reads", func(d *Doc) {
			op := d.Paths["/github_child_access/{id}"]["put"]
			data := op.Responses["409"].Content["application/json"].Schema.Properties["preview"]["properties"].(map[string]any)["data"].(map[string]any)
			delete(data["properties"].(map[string]any), "stops_reaching")
			d.Paths["/github_child_access/{id}"]["put"] = op
		}, nil},
		{"the 428 stops requiring a key the provider reads", func(d *Doc) {
			op := d.Paths["/github_child_access/{id}"]["put"]
			data := op.Responses["428"].Content["application/json"].Schema.Properties["preview"]["properties"].(map[string]any)["data"].(map[string]any)
			data["required"] = slices.DeleteFunc(slices.Clone(data["required"].([]any)), func(k any) bool { return k == "fingerprint" })
			d.Paths["/github_child_access/{id}"]["put"] = op
		}, nil},
		{"the 428 publishes the preview's keys one level too high", func(d *Doc) {
			op := d.Paths["/github_child_access/{id}"]["put"]
			flat := map[string]any{}
			for _, k := range engine.AccessPreviewKeys() {
				flat[k] = map[string]any{}
			}
			op.Responses["428"].Content["application/json"].Schema.Properties["preview"] = map[string]any{"type": "object", "properties": flat}
			d.Paths["/github_child_access/{id}"]["put"] = op
		}, nil},
		{"the preview's account stops being nullable", func(d *Doc) {
			op := d.Paths["/github_child_access/{id}"]["put"]
			data := op.Responses["428"].Content["application/json"].Schema.Properties["preview"]["properties"].(map[string]any)["data"].(map[string]any)
			data["properties"].(map[string]any)["account"].(map[string]any)["type"] = "string"
			d.Paths["/github_child_access/{id}"]["put"] = op
		}, nil},
		{"the contract gains a create door", func(d *Doc) {
			r := d.Resources["github_child_access"]
			r.CreateFields = []string{"mode"}
			d.Resources["github_child_access"] = r
		}, nil},
		{"the account becomes a field of the row", func(d *Doc) {
			r := d.Resources["github_child_access"]
			r.Fields = append(slices.Clone(r.Fields), "installation")
			d.Resources["github_child_access"] = r
		}, nil},
		{"the row's sharing stops being a field", func(d *Doc) {
			r := d.Resources["github_child_access"]
			r.Fields = slices.DeleteFunc(slices.Clone(r.Fields), func(f string) bool { return f == "context_repo_shared" })
			d.Resources["github_child_access"] = r
		}, nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			d := doc(t)
			spec := childAccessSpec()
			if problems := specProblems(d, spec); len(problems) != 0 {
				t.Fatalf("the untouched document already disagrees: %v", problems)
			}
			if c.mutate != nil {
				c.mutate(d)
			}
			if c.spec != nil {
				c.spec(&spec)
			}
			if len(specProblems(d, spec)) == 0 {
				t.Fatal("the contract test missed this drift")
			}
		})
	}
}

// The preview reader the provider uses for a child access preview refuses every
// answer that lacks a key it reads, so a fixture that loses one is noticed here
// and not at an apply.
func TestEveryChildAccessPreviewKeyIsPinnedByTheFixtures(t *testing.T) {
	fixtures, err := accessFixtures()
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range engine.AccessPreviewKeys() {
		for name, fixture := range fixtures {
			if _, ok := fixture[key]; !ok {
				t.Errorf("the %s fixture lacks %s", name, key)
			}
			broken := maps.Clone(fixture)
			delete(broken, key)
			if engine.CheckAccessPreview(broken) == nil {
				t.Errorf("a %s fixture without %s is still read as a preview", name, key)
			}
		}
	}
}

// githubAccountSpec is the spec of one connected GitHub account's settings: a
// natural-key row the platform has no door to create or delete, adopted and left
// in place. It is held to the contract here so the capabilities a second adopted
// row needs are proved beside the first.
func githubAccountSpec() engine.Spec {
	return engine.Spec{
		Key: "github_accounts", TypeName: "github_account", ListName: "github_accounts",
		Shape: engine.NaturalKey, IDAttr: "account", Lifecycle: true,
		AdoptOnly:      true,
		LeaveOnDestroy: "The account stays connected; disconnect it on the Integrations page.",
		Attrs: []engine.Attr{
			{Name: "account", Kind: engine.String, Address: true, Required: true, CreateOnly: true},
			{Name: "auto_pr_enabled", Kind: engine.Bool, UpdateOnly: true},
			{Name: "auto_revise_on_review", Kind: engine.Bool, UpdateOnly: true},
			{Name: "draft_prs", Kind: engine.Bool, UpdateOnly: true},
			{Name: "review_enabled", Kind: engine.Bool, UpdateOnly: true},
			{Name: "link_comments_enabled", Kind: engine.Bool, UpdateOnly: true},
			{Name: "auto_merge_requires_verification", Kind: engine.Bool, UpdateOnly: true},
			{Name: "review_severity_floor", Kind: engine.String, UpdateOnly: true, OneOf: []string{"critical", "high", "medium", "low"}},
			{Name: "account_id", Kind: engine.Int, Computed: true},
			{Name: "account_type", Kind: engine.String, Computed: true},
			{Name: "installation_id", Kind: engine.Int, Computed: true},
			{Name: "status", Kind: engine.String, Computed: true},
			{Name: "managed", Kind: engine.Bool, Computed: true},
			{Name: "holds_context_repo", Kind: engine.Bool, Computed: true},
			{Name: "repository_count", Kind: engine.Int, Computed: true},
			{Name: "repos_truncated", Kind: engine.Bool, Computed: true},
			{Name: "repos_refreshed_at", Kind: engine.String, Computed: true},
			{Name: "repos", Kind: engine.JSON, Computed: true},
		},
	}
}

func TestAnAdoptedNaturalKeyRowAgreesWithTheContract(t *testing.T) {
	d := doc(t)
	for _, problem := range specProblems(d, githubAccountSpec()) {
		t.Error(problem)
	}
	spec := githubAccountSpec()
	spec.Attrs[0].Address = false
	if len(specProblems(d, spec)) == 0 {
		t.Error("an adopted row addressed by a plain attribute was accepted")
	}
}
