package engine

import (
	"fmt"
	"maps"
	"slices"
	"strings"
)

// The preview of a change to what a parent organization gives one child of one
// GitHub account is not the repository defaults' preview: it has its own keys
// (what the child stops and starts reaching, what the change holds) and is read
// by its own reader, chosen by Spec.AccessPreview.

// accessPreviewKeys are the keys of preview.data the provider reads for a child
// access preview, with the JSON type each must have; "|null" marks a key the
// platform answers as null for a change of sharing alone. The platform emits
// every one of them in every preview.
var accessPreviewKeys = map[string]string{
	"fingerprint":                  "string",
	"narrows":                      "boolean",
	"widens":                       "boolean",
	"changes":                      "boolean",
	"account":                      "string|null",
	"account_id":                   "number|null",
	"installation_id":              "number|null",
	"child":                        "object",
	"before":                       "object",
	"after":                        "object",
	"stops_reaching":               "object",
	"starts_reaching":              "object",
	"auto_merge_off":               "number",
	"open_fix_prs":                 "number",
	"waiting":                      "number",
	"waiting_detail":               "object",
	"cards":                        "number",
	"by_repository":                "array",
	"counts_complete":              "boolean",
	"later_repositories_not_given": "boolean",
	"refusals":                     "array",
}

// accessPreviewOptionalKeys are published beside the required ones: applied and
// stale on the answers that carry a preview, note on the 428.
var accessPreviewOptionalKeys = map[string]string{
	"applied": "boolean",
	"stale":   "boolean",
	"note":    "string",
}

// accessViewKeys are the keys of preview.data.before and .after.
var accessViewKeys = map[string]string{
	"mode":                "string|null",
	"repositories":        "array",
	"repository_count":    "number",
	"context_repo_shared": "boolean",
}

// accessReachKeys are the keys of preview.data.stops_reaching and .starts_reaching.
var accessReachKeys = map[string]string{
	"count":        "number",
	"repositories": "array",
	"more":         "number",
}

var accessDetailKeys = map[string]string{
	"held":              "number",
	"queued_for_runner": "number",
	"in_flight":         "number",
}

var accessChildKeys = map[string]string{
	"id":   "string",
	"name": "string",
}

// AccessPreviewKeys are the keys of preview.data the provider requires of a
// child access preview, for the contract test that holds them to the platform's
// published shape.
func AccessPreviewKeys() []string { return slices.Sorted(maps.Keys(accessPreviewKeys)) }

// AccessPreviewOptionalKeys are the keys the platform publishes beside the
// required ones.
func AccessPreviewOptionalKeys() []string { return slices.Sorted(maps.Keys(accessPreviewOptionalKeys)) }

// CheckAccessPreview reports why a preview.data is not a child access preview
// this provider version can read.
func CheckAccessPreview(m map[string]any) error {
	_, err := parseAccessPreview(m)
	return err
}

func kindMatches(v any, want string) bool {
	kind := jsonKind(v)
	for _, one := range strings.Split(want, "|") {
		if kind == one {
			return true
		}
	}
	return false
}

func accessKeyProblems(m map[string]any, keys map[string]string, where string) []string {
	var out []string
	for _, name := range slices.Sorted(maps.Keys(keys)) {
		v, ok := m[name]
		switch {
		case !ok:
			out = append(out, where+name+" is missing")
		case !kindMatches(v, keys[name]):
			out = append(out, fmt.Sprintf("%s%s is %s, not %s", where, name, jsonKind(v), keys[name]))
		}
	}
	return out
}

// accessRepository is one repository a preview names.
type accessRepository struct {
	Name  string
	Moved bool
}

// accessSide is the grant before or after a change.
type accessSide struct {
	Mode         string
	Repositories []accessRepository
	Count        int
	Shares       bool
}

// accessReach is the repositories a child starts or stops reaching: the count,
// the first of them and how many more there are.
type accessReach struct {
	Count int
	Names []string
	More  int
}

// parseAccessPreview reads a child access preview.data. It fails, naming the
// keys, when one the provider reads is absent or of another type, and when the
// fingerprint that applies the preview is empty, so a renamed key is never read
// as a change that moves nothing.
func parseAccessPreview(m map[string]any) (preview, error) {
	if len(m) == 0 {
		return preview{}, &unreadablePreview{problems: []string{"the answer carries no preview"}}
	}
	problems := accessKeyProblems(m, accessPreviewKeys, "")
	for name, want := range accessPreviewOptionalKeys {
		if v, ok := m[name]; ok && !kindMatches(v, want) {
			problems = append(problems, fmt.Sprintf("%s is %s, not %s", name, jsonKind(v), want))
		}
	}
	if fp, ok := m["fingerprint"].(string); ok && fp == "" {
		problems = append(problems, "fingerprint is empty")
	}
	objects := map[string]map[string]string{
		"child": accessChildKeys, "before": accessViewKeys, "after": accessViewKeys,
		"stops_reaching": accessReachKeys, "starts_reaching": accessReachKeys, "waiting_detail": accessDetailKeys,
	}
	for _, name := range slices.Sorted(maps.Keys(objects)) {
		if sub, ok := m[name].(map[string]any); ok {
			problems = append(problems, accessKeyProblems(sub, objects[name], name+".")...)
		}
	}
	refusals, _ := m["refusals"].([]any)
	for i, raw := range refusals {
		e, ok := raw.(map[string]any)
		if !ok {
			problems = append(problems, fmt.Sprintf("refusals[%d] is %s, not object", i, jsonKind(raw)))
			continue
		}
		problems = append(problems, accessKeyProblems(e, map[string]string{"repository": "string", "code": "string", "reason": "string"}, fmt.Sprintf("refusals[%d].", i))...)
	}
	if len(problems) > 0 {
		return preview{}, &unreadablePreview{problems: problems}
	}
	p := preview{Access: true}
	p.Fingerprint, _ = m["fingerprint"].(string)
	for _, raw := range refusals {
		e := raw.(map[string]any)
		repo, _ := e["repository"].(string)
		reason, _ := e["reason"].(string)
		p.Refusals = append(p.Refusals, repo+": "+reason)
	}
	p.text = accessText(m)
	return p, nil
}

func accessSideOf(v any) accessSide {
	m, _ := v.(map[string]any)
	side := accessSide{}
	side.Mode, _ = m["mode"].(string)
	n, _ := m["repository_count"].(float64)
	side.Count = int(n)
	side.Shares, _ = m["context_repo_shared"].(bool)
	items, _ := m["repositories"].([]any)
	for _, raw := range items {
		e, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		name, _ := e["full_name"].(string)
		moved, _ := e["moved"].(bool)
		if name != "" {
			side.Repositories = append(side.Repositories, accessRepository{Name: name, Moved: moved})
		}
	}
	return side
}

func accessReachOf(v any) accessReach {
	m, _ := v.(map[string]any)
	r := accessReach{}
	n, _ := m["count"].(float64)
	r.Count = int(n)
	more, _ := m["more"].(float64)
	r.More = int(more)
	items, _ := m["repositories"].([]any)
	for _, raw := range items {
		if e, ok := raw.(map[string]any); ok {
			if name, _ := e["full_name"].(string); name != "" {
				r.Names = append(r.Names, name)
			}
		}
	}
	return r
}

// described is a side as a sentence says it: what the child may use.
func (s accessSide) described() string {
	switch s.Mode {
	case "all":
		return "every repository of the account"
	case "none":
		return "nothing of the account"
	case "selected":
		names := make([]string, 0, len(s.Repositories))
		for _, r := range s.Repositories {
			names = append(names, r.Name)
		}
		if len(names) == 0 {
			return "a selected list that is empty"
		}
		return "only " + namedList(names)
	}
	return "nothing yet (no grant is stored)"
}

func (r accessReach) described(verb string) string {
	if r.Count == 0 {
		return ""
	}
	names := namedList(r.Names)
	if r.More > 0 {
		names += fmt.Sprintf(" and %d more", r.More)
	}
	return fmt.Sprintf(" It %s reaching %s: %s.", verb, repositories(r.Count), names)
}

func counted(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// accessText is the warning a plan says before a child access change is applied.
func accessText(m map[string]any) string {
	var b strings.Builder
	child, _ := m["child"].(map[string]any)
	name, _ := child["name"].(string)
	account, _ := m["account"].(string)
	before, after := accessSideOf(m["before"]), accessSideOf(m["after"])
	changes, _ := m["changes"].(bool)
	b.WriteString("This change is for child organization " + name)
	if account != "" {
		b.WriteString(" and the GitHub account " + account)
	}
	b.WriteString(".")
	if !changes {
		b.WriteString(" The platform has nothing to write.")
		return b.String()
	}
	if before.Mode != after.Mode || !sameRepositories(before, after) {
		fmt.Fprintf(&b, " It may use %s now and will use %s.", before.described(), after.described())
	}
	if before.Shares != after.Shares {
		if after.Shares {
			b.WriteString(" It will start to use this organization's context repository.")
		} else {
			b.WriteString(" It will stop using this organization's context repository.")
		}
	}
	b.WriteString(accessReachOf(m["stops_reaching"]).described("stops"))
	b.WriteString(accessReachOf(m["starts_reaching"]).described("starts"))
	if narrows, _ := m["narrows"].(bool); narrows {
		openPRs, _ := m["open_fix_prs"].(float64)
		off, _ := m["auto_merge_off"].(float64)
		waiting, _ := m["waiting"].(float64)
		cards, _ := m["cards"].(float64)
		detail, _ := m["waiting_detail"].(map[string]any)
		heldRequests, _ := detail["held"].(float64)
		queued, _ := detail["queued_for_runner"].(float64)
		flight, _ := detail["in_flight"].(float64)
		var held []string
		if openPRs > 0 {
			clause := fmt.Sprintf("%s of the child's stay open on GitHub, each with one comment saying SRE Agent no longer manages it for this organization", counted(int(openPRs), "open fix pull request"))
			if off > 0 {
				clause += fmt.Sprintf(" (auto-merge is turned off on %d of them)", int(off))
			}
			held = append(held, clause)
		}
		if waiting > 0 {
			held = append(held, fmt.Sprintf("%s still waiting or running stop at their next step (%d held, %d queued for the runner, %d running)", counted(int(waiting), "fix request"), int(heldRequests), int(queued), int(flight)))
		}
		if cards > 0 {
			held = append(held, fmt.Sprintf("%s keep their links and history", counted(int(cards), "card")))
		}
		if len(held) == 0 {
			b.WriteString(" Nothing of the child is held on those repositories.")
		} else {
			b.WriteString(" On those repositories, " + strings.Join(held, "; ") + ". Nothing is closed or deleted.")
		}
		if complete, _ := m["counts_complete"].(bool); !complete {
			b.WriteString(" These counts were cut at a bound, so more may be held than they say.")
		}
	}
	if later, _ := m["later_repositories_not_given"].(bool); later {
		b.WriteString(" Repositories the account gets later are not given to the child until they are named here.")
	}
	return b.String()
}

func sameRepositories(a, b accessSide) bool {
	if len(a.Repositories) != len(b.Repositories) {
		return false
	}
	names := func(s accessSide) []string {
		out := make([]string, 0, len(s.Repositories))
		for _, r := range s.Repositories {
			out = append(out, strings.ToLower(r.Name))
		}
		slices.Sort(out)
		return out
	}
	return slices.Equal(names(a), names(b))
}
