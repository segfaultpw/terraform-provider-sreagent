package fakefacade

import (
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

// securityKeys are the five resources whose writes get the platform's own refusals.
var securityKeys = map[string]bool{
	"security_sla_policy":      true,
	"security_digest_settings": true,
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
	}
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
	}
}
