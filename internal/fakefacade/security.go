package fakefacade

import (
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/segfaultpw/terraform-provider-sreagent/internal/engine"
)

// The five security resources, as the platform serves them: the defaults a
// singleton holds before anyone writes it, the refusals it makes across fields,
// and the normalization of what it stores. Each line is read from the platform's
// own changesets and from the contract test of the platform (the sentences are
// its sentences), so a unit test here is a statement about the real API.

const micro = "2006-01-02T15:04:05.000000Z07:00"

const maxRules = 20

const maxAllowRules = 100

var builtInClasses = []any{"hardcoded_secret", "unsafe_deserialization", "ci_unpinned_action", "ci_plain_secret"}

// securityKeys are the five resources whose writes get the platform's own refusals.
var securityKeys = map[string]bool{
	"security_sla_policy":         true,
	"security_digest_settings":    true,
	"security_code_classes":       true,
	"security_notification_rules": true,
	"secret_allow_rules":          true,
}

func isSecurity(key string) bool { return securityKeys[key] }

// securitySeed gives a singleton the row the platform answers before any write.
func securitySeed(spec engine.Spec, row map[string]any) {
	switch spec.Key {
	case "security_sla_policy":
		row["critical_days"], row["high_days"], row["medium_days"], row["low_days"], row["info_days"] = 7, 30, 90, 180, nil
		row["defaults"], row["updated_at"] = true, nil
	case "security_digest_settings":
		row["enabled"], row["cadence"], row["weekday"], row["hour"] = false, "weekly", 1, 9
		row["slack_channel"], row["email"], row["timezone"] = nil, false, "UTC"
		row["last_sent_at"], row["last_sent_period"] = nil, nil
	case "security_code_classes":
		row["classes"], row["built_in_classes"], row["available_classes"], row["limit"] = []any{}, builtInClasses, []any{}, 50
	}
}

// SetAcceptedClasses sets the other:<kind> classes the organization accepted on
// the Code improvement page; the ones already marked stay marked.
func (f *Fake) SetAcceptedClasses(classes ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.accepted = classes
	if row := f.rows["security_code_classes"]["security_code_classes"]; row != nil {
		f.refreshClasses(row)
	}
}

// refreshClasses answers available_classes: the accepted classes that are not marked.
func (f *Fake) refreshClasses(row map[string]any) {
	marked, _ := row["classes"].([]any)
	available := []any{}
	for _, c := range f.accepted {
		if !slices.Contains(marked, any(c)) {
			available = append(available, c)
		}
	}
	row["available_classes"] = available
}

func number(v any) (int64, bool) {
	switch n := v.(type) {
	case int:
		return int64(n), true
	case int64:
		return n, true
	case float64:
		return int64(n), true
	case interface{ Int64() (int64, error) }:
		i, err := n.Int64()
		return i, err == nil
	}
	return 0, false
}

func text(v any) string {
	s, _ := v.(string)
	return s
}

// merged is the row a write would produce: the stored row with the body's fields.
func merged(prior, body map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range prior {
		out[k] = v
	}
	for k, v := range body {
		out[k] = v
	}
	return out
}

func blankOrNil(v any) bool {
	s, ok := v.(string)
	return v == nil || (ok && strings.TrimSpace(s) == "")
}

// securityRefusal is the 422 or 409 the platform answers a write with, judged on
// the row the write would produce; status 0 is no refusal. prior is nil on a
// create.
func (f *Fake) securityRefusal(spec engine.Spec, id string, prior, body map[string]any) (int, string) {
	switch spec.Key {
	case "security_sla_policy":
		if len(body) == 0 {
			return 422, "Name at least one of critical_days, high_days, medium_days, low_days, info_days to change."
		}
		for k, v := range body {
			if v == nil {
				continue
			}
			if n, ok := number(v); !ok || n < 1 || n > 365 {
				return 422, k + " must be between 1 and 365 days, or null for no SLA."
			}
		}
	case "security_digest_settings":
		if len(body) == 0 {
			return 422, "Name at least one of enabled, cadence, weekday, hour, slack_channel, email to change."
		}
		for k, v := range body {
			switch k {
			case "enabled", "email", "cadence", "weekday", "hour":
				if v == nil {
					return 422, k + " cannot be null."
				}
			}
		}
		row := merged(prior, body)
		if on, _ := row["enabled"].(bool); on && blankOrNil(row["slack_channel"]) {
			if email, _ := row["email"].(bool); !email {
				return 422, "Choose a Slack channel or email for the digest."
			}
		}
	case "security_code_classes":
		classes, _ := body["classes"].([]any)
		if len(classes) > 50 {
			return 422, "At most 50 classes can be marked."
		}
		marked, _ := prior["classes"].([]any)
		available, _ := prior["available_classes"].([]any)
		for _, c := range classes {
			switch {
			case slices.Contains(builtInClasses, c):
				return 422, fmt.Sprintf("%v always counts as security and is not listed; list the other:<kind> classes you want counted.", c)
			case slices.Contains(marked, c), slices.Contains(available, c):
			default:
				return 422, fmt.Sprintf("%v is not a class this organization accepted. Name a class from available_classes.", c)
			}
		}
	case "security_notification_rules":
		return f.ruleRefusal(spec, id, prior, body)
	case "secret_allow_rules":
		return f.allowRefusal(spec, body)
	}
	return 0, ""
}

func (f *Fake) ruleRefusal(spec engine.Spec, id string, prior, body map[string]any) (int, string) {
	if name, ok := body["name"].(string); ok {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" || utf8.RuneCountInString(trimmed) > 80 {
			return 422, "name must be 1 to 80 characters."
		}
		for other, row := range f.rows[spec.Key] {
			if other != id && text(row["name"]) == trimmed {
				return 409, "A notification rule with that name already exists."
			}
		}
	}
	if prior == nil && len(f.rows[spec.Key]) >= maxRules {
		return 422, "An organization can have at most 20 notification rules."
	}
	row := merged(prior, body)
	if prior == nil {
		row = ruleDefaults(body)
	}
	if page, _ := row["page"].(bool); page && row["min_severity"] != "critical" {
		return 422, "Paging is only allowed on a rule whose minimum severity is critical."
	}
	slack, _ := row["slack"].(bool)
	email, _ := row["email"].(bool)
	page, _ := row["page"].(bool)
	if !slack && !email && !page {
		return 422, "Choose at least one of Slack, email or paging."
	}
	if events, ok := row["events"].([]any); ok && len(events) == 0 {
		return 422, "events must hold at least one event."
	}
	return 0, ""
}

// ruleDefaults is a created rule: what the body names, the platform's default for the rest.
func ruleDefaults(body map[string]any) map[string]any {
	row := map[string]any{"enabled": true, "areas": []any{}, "min_severity": "high", "events": []any{"new_finding"}, "slack": true, "slack_channel": nil, "email": true, "page": false}
	for k, v := range body {
		row[k] = v
	}
	return row
}

func (f *Fake) allowRefusal(spec engine.Spec, body map[string]any) (int, string) {
	if len(f.rows[spec.Key]) >= maxAllowRules {
		return 422, "At most 100 allow rules can be kept. Delete one you no longer need first."
	}
	if n := utf8.RuneCountInString(strings.TrimSpace(text(body["reason"]))); n < 10 || n > 500 {
		return 422, "reason must be 10 to 500 characters."
	}
	if raw, ok := body["expires_at"].(string); ok && strings.TrimSpace(raw) != "" {
		end, err := time.Parse(time.RFC3339, strings.TrimSpace(raw))
		if err != nil {
			return 422, "expires_at must be an ISO 8601 time."
		}
		if !end.After(time.Now()) {
			return 422, "expires_at must be in the future."
		}
	}
	if strings.Contains(text(body["reason"]), "AKIA") {
		return 422, "The reason looks like it holds a credential. Say why the match is safe without quoting it."
	}
	return 0, ""
}

// securityStore is the platform's own normalization of a row it stored: sorted
// classes, trimmed text, blanks as none, microsecond times, the defaults a rule
// takes, the flag that says the SLA is still the platform's.
func (f *Fake) securityStore(spec engine.Spec, row, body map[string]any, creating bool) {
	now := time.Now().UTC().Format(micro)
	switch spec.Key {
	case "security_sla_policy":
		for _, k := range []string{"critical_days", "high_days", "medium_days", "low_days", "info_days"} {
			if n, ok := number(row[k]); ok {
				row[k] = int(n)
			}
		}
		row["defaults"] = row["critical_days"] == any(7) && row["high_days"] == any(30) && row["medium_days"] == any(90) && row["low_days"] == any(180) && row["info_days"] == nil
		row["updated_at"] = now
	case "security_digest_settings":
		if blankOrNil(row["slack_channel"]) {
			row["slack_channel"] = nil
		}
	case "security_code_classes":
		names := []string{}
		for _, c := range anySlice(row["classes"]) {
			if s := fmt.Sprint(c); !slices.Contains(names, s) {
				names = append(names, s)
			}
		}
		slices.Sort(names)
		sorted := make([]any, 0, len(names))
		for _, n := range names {
			sorted = append(sorted, n)
		}
		row["classes"] = sorted
		f.refreshClasses(row)
	case "security_notification_rules":
		if creating {
			for k, v := range ruleDefaults(body) {
				if _, named := body[k]; !named {
					row[k] = v
				}
			}
		}
		row["name"] = strings.TrimSpace(text(row["name"]))
		if blankOrNil(row["slack_channel"]) {
			row["slack_channel"] = nil
		}
		row["updated_at"] = now
	case "secret_allow_rules":
		for _, k := range []string{"path_glob", "reason", "repository", "detector_type", "expires_at"} {
			if s, ok := row[k].(string); ok {
				row[k] = strings.TrimSpace(s)
			}
			if blankOrNil(row[k]) && k != "path_glob" && k != "reason" {
				row[k] = nil
			}
		}
		if s, ok := row["repository"].(string); ok {
			row["repository"] = strings.ToLower(s)
		}
		if s, ok := row["expires_at"].(string); ok {
			if t, err := time.Parse(time.RFC3339, s); err == nil {
				row["expires_at"] = t.UTC().Format(micro)
			}
		}
		if creating {
			row["created_at"] = now
		}
	}
}

func anySlice(v any) []any {
	list, _ := v.([]any)
	return list
}
