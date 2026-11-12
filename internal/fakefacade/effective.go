package fakefacade

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"path"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// effectiveModel is the repositories the organization's GitHub App reaches
// beside the ones that have settings saved, for the paged view of what each
// repository runs with.
type effectiveModel struct {
	reachable []string
	archived  []string
	pageCap   int
	queries   []map[string]string
}

// Reachable lists repositories the GitHub App reaches, saved settings or not.
func (f *Fake) Reachable(repos ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.em.reachable = append(f.em.reachable, repos...)
}

// Archived marks repositories GitHub reports archived.
func (f *Fake) Archived(repos ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.em.archived = append(f.em.archived, repos...)
}

// CapPages makes every page of the view hold at most n rows, whatever limit is
// asked, the way a platform with a smaller cap would answer.
func (f *Fake) CapPages(n int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.em.pageCap = n
}

// Queries are the query strings of every read of the view, in order.
func (f *Fake) Queries() []map[string]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.em.queries)
}

func (f *Fake) effectiveRow(repo string) map[string]any {
	rows := f.repoRows(repo)
	var base map[string]any
	for _, r := range rows {
		if isAllBranches(r) {
			base = r
		}
	}
	settings := map[string]any{}
	for _, n := range followable {
		value, following := f.defaults[n], true
		switch {
		case base != nil && slices.Contains(heldOf(base), n):
			value, following = base[n], false
		default:
			for _, r := range rows {
				if slices.Contains(heldOf(r), n) {
					following = false
				}
			}
		}
		settings[n] = map[string]any{"value": value, "following": following}
	}
	differs := []any{}
	for _, n := range followable {
		for _, r := range rows {
			if !isAllBranches(r) && slices.Contains(heldOf(r), n) && (base == nil || !slices.Contains(heldOf(base), n)) {
				differs = append(differs, n)
				break
			}
		}
	}
	return map[string]any{
		"repo": repo, "archived": slices.Contains(f.em.archived, repo), "reachable": true,
		"has_row": len(rows) > 0, "fence": "none", "settings": settings, "differs_by_branch": differs,
	}
}

func (f *Fake) serveEffective(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	seen := map[string]string{}
	for k := range q {
		seen[k] = q.Get(k)
	}
	f.em.queries = append(f.em.queries, seen)
	names := map[string]bool{}
	for _, n := range f.em.reachable {
		names[n] = true
	}
	for _, row := range f.rows["repo_settings"] {
		names[fmt.Sprint(row["repo"])] = true
	}
	var all []string
	for n := range names {
		if q.Get("include_archived") != "true" && slices.Contains(f.em.archived, n) {
			continue
		}
		if pat := strings.ToLower(q.Get("name")); pat != "" {
			lower := strings.ToLower(n)
			short := lower[strings.Index(lower, "/")+1:]
			if !strings.Contains(pat, "*") {
				pat += "*"
			}
			if ok, _ := path.Match(pat, lower); !ok {
				if ok, _ = path.Match(pat, short); !ok {
					continue
				}
			}
		}
		row := f.effectiveRow(n)
		if want := q.Get("runner"); want != "" && fmt.Sprint(row["settings"].(map[string]any)["fix_runner"].(map[string]any)["value"]) != want {
			continue
		}
		if want := q.Get("has_fence"); want != "" && (want == "true") != (row["fence"] != "none") {
			continue
		}
		all = append(all, n)
	}
	sort.Strings(all)
	limit := 100
	if n, err := strconv.Atoi(q.Get("limit")); err == nil && n > 0 {
		limit = min(n, 500)
	}
	if f.em.pageCap > 0 {
		limit = min(limit, f.em.pageCap)
	}
	if cur := q.Get("cursor"); cur != "" {
		last, _ := base64.RawURLEncoding.DecodeString(cur)
		i := sort.SearchStrings(all, string(last))
		for i < len(all) && all[i] <= string(last) {
			i++
		}
		all = all[i:]
	}
	var next any
	if len(all) > limit {
		all = all[:limit]
		next = base64.RawURLEncoding.EncodeToString([]byte(all[len(all)-1]))
	}
	rows := make([]any, 0, len(all))
	for _, n := range all {
		rows = append(rows, f.effectiveRow(n))
	}
	reply(w, 200, map[string]any{
		"organization": wrap(nil)["organization"], "data": rows, "truncated": false, "next_cursor": next,
		"defaults":          map[string]any{"values": f.defaultsValues(), "source": f.dm.source, "inherited_from": nil},
		"runner_registered": false, "archived_known": true,
		"counts": map[string]any{"total": len(names), "unreachable": 0, "without_row": len(names) - len(f.rows["repo_settings"]), "with_fence": 0, "archived_hidden": len(f.em.archived)},
	}, "")
}

func (f *Fake) defaultsValues() map[string]any {
	out := map[string]any{}
	for _, k := range defaultsSettings {
		out[k] = f.defaults[k]
	}
	return out
}
