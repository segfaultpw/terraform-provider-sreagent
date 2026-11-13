package fakefacade

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/segfaultpw/terraform-provider-sreagent/internal/engine"
)

// The five security resources, as the platform serves them: the defaults a
// singleton holds before anyone writes it, the refusals it makes across fields,
// and the normalization of what it stores. Each line is read from the platform's
// own changesets and from the contract test of the platform (the sentences are
// its sentences), so a unit test here is a statement about the real API.

const micro = "2006-01-02T15:04:05.000000Z07:00"

var builtInClasses = []any{"hardcoded_secret", "unsafe_deserialization", "ci_unpinned_action", "ci_plain_secret"}

// securityKeys are the five resources whose writes get the platform's own refusals.
var securityKeys = map[string]bool{
	"security_sla_policy":      true,
	"security_digest_settings": true,
	"security_code_classes":    true,
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
func (f *Fake) securityRefusal(spec engine.Spec, _ string, prior, body map[string]any) (int, string) {
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
	}
	return 0, ""
}

// securityStore is the platform's own normalization of a row it stored: sorted
// classes, trimmed text, blanks as none, microsecond times, the defaults a rule
// takes, the flag that says the SLA is still the platform's.
func (f *Fake) securityStore(spec engine.Spec, row, _ map[string]any, _ bool) {
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
	}
}

func anySlice(v any) []any {
	list, _ := v.([]any)
	return list
}
