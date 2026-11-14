package fakefacade

import (
	"fmt"
	"net/http"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"
)

func isOrganization(key string) bool { return key == "organization_settings" }

// organizationSeed is what the platform answers for an organization nobody has
// changed: English, every thread event posted, SRE Agent's twelve quick reactions.
func organizationSeed(row map[string]any) {
	row["language"] = "en"
	row["slack_thread_events_off"] = []any{}
	row["reaction_quick_row"] = []any{}
}

var threadEvents = []string{"column_moves", "card_done", "card_wont_do", "pull_request_opened", "pull_request_merged", "pull_request_closed", "pull_request_reopened", "change_pushed", "assistant_replies", "comments"}

// emojiTable is the fully qualified spelling of the emoji the tests use, by their
// bare form (no U+FE0E or U+FE0F). The platform looks every value up this way and
// stores the fully qualified form; anything it does not know is refused.
var emojiTable = func() map[string]string {
	out := map[string]string{}
	for _, full := range []string{
		"❤️",                   // red heart: unqualified when sent as U+2764
		"\U0001F44D",           // thumbs up
		"\U0001F44D\U0001F3FD", // thumbs up, medium skin tone
		"\U0001F389",           // party popper
		"\U0001F468‍\U0001F469‍\U0001F467‍\U0001F466", // family: man, woman, girl, boy (ZWJ)
		"\U0001F3F3️‍\U0001F308",                      // rainbow flag (ZWJ, selector in the middle)
		"\U0001F469\U0001F3FE‍\U0001F4BB",             // woman technologist, medium-dark skin tone
		"☺️",                                          // smiling face
		"\U0001F680",                                  // rocket
		"\U0001F440",                                  // eyes
		"\U0001F64F",                                  // folded hands
		"\U0001F525",                                  // fire
		"\U0001F60A",                                  // smiling face with smiling eyes
		"\U0001F622",                                  // crying face
		"✅",                                           // check mark button
		"\U0001F4AF",                                  // hundred points
	} {
		out[strings.NewReplacer("︎", "", "️", "").Replace(full)] = full
	}
	return out
}()

var slackNames = map[string]string{":tada:": "\U0001F389", ":rocket:": "\U0001F680"}

func normalizeEmoji(v string) (string, bool) {
	if full, ok := slackNames[v]; ok {
		return full, true
	}
	full, ok := emojiTable[strings.NewReplacer("︎", "", "️", "").Replace(v)]
	return full, ok
}

// organizationRefusal is the platform's refusal of the three settings of this
// release: a language it does not offer, a thread event it does not name, and a
// quick row that is too long, repeats an emoji or holds something that is none.
func organizationRefusal(body map[string]any) (int, string) {
	if v, ok := body["language"]; ok {
		if s, _ := v.(string); s != "en" && s != "es" {
			return http.StatusUnprocessableEntity, "language must be English or Spanish"
		}
	}
	if v, ok := body["slack_thread_events_off"]; ok {
		list, _ := v.([]any)
		for _, e := range list {
			if !slices.Contains(threadEvents, fmt.Sprint(e)) {
				return http.StatusUnprocessableEntity, "slack_thread_events_off must name only events that can be switched off: " + strings.Join(threadEvents, ", ") + ". A failed, declined or cancelled fix or change is always posted."
			}
		}
	}
	if v, ok := body["reaction_quick_row"]; ok {
		list, _ := v.([]any)
		if len(list) > 12 {
			return http.StatusUnprocessableEntity, "reaction_quick_row must hold up to 12 emoji"
		}
		seen := map[string]bool{}
		for _, e := range list {
			s, _ := e.(string)
			full, known := normalizeEmoji(s)
			switch {
			case utf8.RuneCountInString(s) > 64 || !known:
				return http.StatusUnprocessableEntity, "reaction_quick_row must be a list of up to 12 different emoji, each one emoji or its Slack name in colons, such as :tada:. An empty list uses SRE Agent's twelve."
			case seen[full]:
				return http.StatusUnprocessableEntity, "reaction_quick_row must not hold the same emoji twice"
			}
			seen[full] = true
		}
	}
	return 0, ""
}

// organizationStore applies the platform's normalization after a write: the
// events sorted without repeats, each emoji in its fully qualified form.
func organizationStore(row, body map[string]any) {
	if v, ok := body["slack_thread_events_off"]; ok {
		list, _ := v.([]any)
		names := []string{}
		for _, e := range list {
			if n := fmt.Sprint(e); !slices.Contains(names, n) {
				names = append(names, n)
			}
		}
		sort.Strings(names)
		out := make([]any, 0, len(names))
		for _, n := range names {
			out = append(out, n)
		}
		row["slack_thread_events_off"] = out
	}
	if v, ok := body["reaction_quick_row"]; ok {
		list, _ := v.([]any)
		out := make([]any, 0, len(list))
		for _, e := range list {
			full, _ := normalizeEmoji(fmt.Sprint(e))
			out = append(out, full)
		}
		row["reaction_quick_row"] = out
	}
}

// serveInformativeKinds answers the list the way the platform does: the proposals
// and the approved kinds by default, one status or all with status, and the counts
// and limits beside the rows.
func (f *Fake) serveInformativeKinds(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	ids := make([]string, 0, len(f.rows["alert_informative_kinds"]))
	for id := range f.rows["alert_informative_kinds"] {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	list := []any{}
	proposed, approved := 0, 0
	for _, id := range ids {
		row := f.rows["alert_informative_kinds"][id]
		st := fmt.Sprint(row["status"])
		switch st {
		case "proposed":
			proposed++
		case "approved":
			approved++
		}
		switch {
		case status == "all", status == st, status == "" && (st == "proposed" || st == "approved"):
			list = append(list, row)
		}
	}
	reply(w, 200, map[string]any{
		"organization": wrap(nil)["organization"], "data": list, "truncated": false,
		"counts": map[string]any{"proposed": proposed, "approved": approved},
		"limits": map[string]any{"approved": 50, "proposed": 20},
	}, "")
}
