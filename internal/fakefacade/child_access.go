package fakefacade

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

// childAccessKey is the resource key of what a parent organization gives one
// child of one connected GitHub account. The platform has no door that creates or
// deletes such a row (a pair exists once the child is linked and the account
// connected), addresses it by the child in the path and the account in the query,
// and applies a change that takes something away only against a preview: a
// narrowing sent with no fingerprint answers 428 and the preview, a fingerprint
// that no longer describes what would move answers 409 and the fresh preview, and
// a change that takes nothing away is written at once. A fingerprint, when sent,
// is always checked, so one no preview carries answers the fresh preview and
// writes nothing.
const childAccessKey = "github_child_access"

type childAccessRow struct {
	child, name, account string
	mode                 string
	repos                []string
	shared               bool
	openFixPRs           int
}

type childAccessModel struct {
	lastFP     string
	accounts   map[string][]string
	rows       []*childAccessRow
	previews   int
	writes     int
	impactOnce func(*childAccessRow)
}

// GitHubAccount registers a connected account of the parent and the repositories
// it lists.
func (f *Fake) GitHubAccount(label string, repositories ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.ca.accounts == nil {
		f.ca.accounts = map[string][]string{}
	}
	f.ca.accounts[label] = repositories
}

// LinkChild gives child (an id) and its name the account as a row, as the app
// does when a child is linked: mode, the repositories of a selected one and
// whether the child shares the parent's context repository.
func (f *Fake) LinkChild(child, name, account, mode string, shared bool, repositories ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ca.rows = append(f.ca.rows, &childAccessRow{child: child, name: name, account: account, mode: mode, repos: repositories, shared: shared})
}

// ChildAccessRow answers what the platform holds for the pair: mode, repositories
// and sharing.
func (f *Fake) ChildAccessRow(child, account string) (mode string, repositories []string, shared bool, ok bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	row := f.ca.find(child, account)
	if row == nil {
		return "", nil, false, false
	}
	return row.mode, append([]string{}, row.repos...), row.shared, true
}

// ChildHoldsOpenFixPRs sets how many open fix pull requests of the child the
// platform counts on every repository that leaves it, so a preview carries
// something to be stale about.
func (f *Fake) ChildHoldsOpenFixPRs(child, account string, n int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if row := f.ca.find(child, account); row != nil {
		row.openFixPRs = n
	}
}

// ChildChangesOnNextFingerprintedPut edits the pair the way a person would in the
// app between Terraform's preview and its write: fn runs right before the next
// PUT that carries the fingerprint of the latest preview is judged.
func (f *Fake) ChildChangesOnNextFingerprintedPut(fn func(mode *string, repositories *[]string, shared *bool, openFixPRs *int)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ca.impactOnce = func(row *childAccessRow) { fn(&row.mode, &row.repos, &row.shared, &row.openFixPRs) }
}

// ChildAccessPreviews counts the previews (428, and a 409 with the fresh preview)
// the platform answered.
func (f *Fake) ChildAccessPreviews() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.ca.previews
}

// ChildAccessWrites counts the PUTs that changed a row.
func (f *Fake) ChildAccessWrites() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.ca.writes
}

func (m *childAccessModel) find(child, account string) *childAccessRow {
	for _, row := range m.rows {
		if (row.child == child || strings.EqualFold(row.name, child)) && (account == "" || strings.EqualFold(row.account, account)) {
			return row
		}
	}
	return nil
}

func (m *childAccessModel) accountsOf(child string) []*childAccessRow {
	var out []*childAccessRow
	for _, row := range m.rows {
		if row.child == child || strings.EqualFold(row.name, child) {
			out = append(out, row)
		}
	}
	return out
}

func (m *childAccessModel) answer(row *childAccessRow) map[string]any {
	repos := []any{}
	details := []any{}
	if row.mode == "selected" {
		for _, r := range row.repos {
			repos = append(repos, r)
			details = append(details, map[string]any{"full_name": r, "repository_id": nil, "moved": false})
		}
	}
	return map[string]any{
		"child_organization": row.child, "child_name": row.name, "account": row.account,
		"installation_id": 1000 + len(row.account), "mode": row.mode, "repositories": repos,
		"context_repo_shared": row.shared, "repository_details": details,
		"repository_count": m.reach(row.account, row.mode, row.repos), "details_read": true,
		"applies": row.mode != "none", "reason": nil, "source": nil, "stopped": false,
		"own_github": false, "updated_at": nil,
	}
}

func (m *childAccessModel) reach(account, mode string, repos []string) int {
	return len(m.reachSet(account, mode, repos))
}

func (m *childAccessModel) reachSet(account, mode string, repos []string) map[string]string {
	out := map[string]string{}
	switch mode {
	case "all":
		for _, r := range m.accounts[account] {
			out[strings.ToLower(r)] = r
		}
	case "selected":
		for _, r := range repos {
			out[strings.ToLower(r)] = r
		}
	}
	return out
}

func asStrings(v any) []string {
	items, _ := v.([]any)
	out := make([]string, 0, len(items))
	for _, e := range items {
		out = append(out, fmt.Sprint(e))
	}
	return out
}

func sortedNames(set map[string]string) []string {
	out := make([]string, 0, len(set))
	for _, name := range set {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

type childAccessPlan struct {
	row                  *childAccessRow
	mode                 string
	repos                []string
	shared               bool
	lost, gained         []string
	narrows, widens, any bool
}

func (m *childAccessModel) plan(row *childAccessRow, body map[string]any) (*childAccessPlan, string) {
	p := &childAccessPlan{row: row, mode: row.mode, repos: row.repos, shared: row.shared}
	if v, ok := body["mode"].(string); ok {
		p.mode = v
	}
	if v, ok := body["context_repo_shared"].(bool); ok {
		p.shared = v
	}
	_, named := body["repositories"]
	switch {
	case named:
		p.repos = asStrings(body["repositories"])
		if p.mode != "selected" {
			return nil, "Repositories can only be given with the mode selected."
		}
	case p.mode == "selected" && row.mode != "selected":
		return nil, "Name the repositories to give, or choose all or nothing."
	case p.mode != "selected":
		p.repos = nil
	}
	if len(p.repos) > 500 {
		return nil, "A child organization can be given at most 500 repositories of one account."
	}
	for _, r := range p.repos {
		found := false
		for _, have := range m.accounts[row.account] {
			if strings.EqualFold(have, r) {
				found = true
			}
		}
		if !found {
			return nil, fmt.Sprintf("%s is not a repository of %s.", r, row.account)
		}
	}
	before := m.reachSet(row.account, row.mode, row.repos)
	after := m.reachSet(row.account, p.mode, p.repos)
	lost, gained := map[string]string{}, map[string]string{}
	for k, v := range before {
		if _, ok := after[k]; !ok {
			lost[k] = v
		}
	}
	for k, v := range after {
		if _, ok := before[k]; !ok {
			gained[k] = v
		}
	}
	p.lost, p.gained = sortedNames(lost), sortedNames(gained)
	p.narrows = len(p.lost) > 0 || (row.shared && !p.shared)
	p.widens = len(p.gained) > 0 || (!row.shared && p.shared)
	p.any = p.narrows || p.widens || row.mode != p.mode
	return p, ""
}

func (p *childAccessPlan) fingerprint() string {
	b, _ := json.Marshal([]any{p.row.child, p.row.account, p.row.mode, p.row.repos, p.row.shared, p.mode, p.repos, p.shared, p.lost, p.gained, p.row.openFixPRs})
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:16])
}

func (m *childAccessModel) side(account, mode string, repos []string, shared bool) map[string]any {
	items := []any{}
	if mode == "selected" {
		for _, r := range repos {
			items = append(items, map[string]any{"full_name": r, "repository_id": nil, "moved": false})
		}
	}
	return map[string]any{"mode": mode, "repositories": items, "repository_count": m.reach(account, mode, repos), "context_repo_shared": shared}
}

func reachList(names []string) map[string]any {
	items := []any{}
	for i, n := range names {
		if i == 10 {
			break
		}
		items = append(items, map[string]any{"full_name": n, "repository_id": nil})
	}
	more := 0
	if len(names) > 10 {
		more = len(names) - 10
	}
	return map[string]any{"count": len(names), "repositories": items, "more": more}
}

func (m *childAccessModel) preview(p *childAccessPlan) map[string]any {
	row := p.row
	byRepo := []any{}
	waiting := 0
	if p.narrows {
		for _, n := range p.lost {
			byRepo = append(byRepo, map[string]any{"repository": n, "open_fix_prs": row.openFixPRs, "auto_merge_off": 0, "waiting": 0, "cards": 0})
		}
	}
	openPRs := row.openFixPRs * len(p.lost)
	return map[string]any{
		"fingerprint": p.fingerprint(), "narrows": p.narrows, "widens": p.widens, "changes": p.any,
		"account": row.account, "account_id": 4242, "installation_id": 1000 + len(row.account),
		"child":          map[string]any{"id": row.child, "name": row.name},
		"before":         m.side(row.account, row.mode, row.repos, row.shared),
		"after":          m.side(row.account, p.mode, p.repos, p.shared),
		"stops_reaching": reachList(p.lost), "starts_reaching": reachList(p.gained),
		"auto_merge_off": 0, "open_fix_prs": openPRs, "waiting": waiting,
		"waiting_detail": map[string]any{"held": 0, "queued_for_runner": 0, "in_flight": 0},
		"cards":          0, "by_repository": byRepo, "counts_complete": true,
		"later_repositories_not_given": p.mode != "all", "refusals": []any{},
	}
}

func (f *Fake) previewAnswer(w http.ResponseWriter, status int, code, message string, data map[string]any) {
	f.ca.previews++
	f.ca.lastFP, _ = data["fingerprint"].(string)
	reply(w, status, map[string]any{"error": code, "message": message, "preview": map[string]any{"data": data, "organization": wrap(nil)["organization"]}}, "")
}

// serveChildAccess answers the resource: GET of a row or the list, and the PUT of
// a change. It has no create and no delete door.
func (f *Fake) serveChildAccess(w http.ResponseWriter, r *http.Request, id string, body map[string]any) {
	m := &f.ca
	switch r.Method {
	case http.MethodPost, http.MethodDelete:
		fail(w, 405, "method_not_allowed", "github_child_access has no door that creates or removes a row.")
		return
	}
	if id == "" {
		list := []any{}
		for _, row := range m.rows {
			list = append(list, m.answer(row))
		}
		reply(w, 200, map[string]any{"organization": wrap(nil)["organization"], "data": list, "truncated": false}, "")
		return
	}
	segment, _, _ := strings.Cut(id, "?")
	child, _ := url.PathUnescape(segment)
	account := r.URL.Query().Get("account")
	candidates := m.accountsOf(child)
	if len(candidates) == 0 {
		fail(w, 404, "not_found", "That organization is not a child of this one.")
		return
	}
	var row *childAccessRow
	switch {
	case account == "" && len(candidates) == 1:
		row = candidates[0]
	case account == "":
		fail(w, 422, "unprocessable_entity", "This organization has several GitHub accounts connected. Name one.")
		return
	default:
		row = m.find(child, account)
	}
	if row == nil {
		fail(w, 404, "not_found", "That GitHub account is not connected to this organization.")
		return
	}
	tag := rowTag(m.answer(row))
	if r.Method == http.MethodGet {
		reply(w, 200, wrap(m.answer(row)), tag)
		return
	}
	for k := range body {
		if k != "mode" && k != "repositories" && k != "context_repo_shared" && k != "expected_fingerprint" {
			fail(w, 422, "unprocessable_entity", k+" is not an argument this tool declares.")
			return
		}
	}
	if im := r.Header.Get("If-Match"); im != "" && strings.TrimPrefix(im, "W/") != tag {
		reply(w, 412, map[string]any{"organization": wrap(nil)["organization"], "data": m.answer(row), "error": "precondition_failed", "message": "The row changed after the version If-Match names; nothing was written."}, tag)
		return
	}
	fingerprint, _ := body["expected_fingerprint"].(string)
	if fingerprint != "" && fingerprint == m.lastFP && m.impactOnce != nil {
		m.impactOnce(row)
		m.impactOnce = nil
	}
	if len(body) == 0 || (len(body) == 1 && fingerprint != "") {
		fail(w, 422, "unprocessable_entity", "There is nothing to change: send a mode, a list of repositories or whether to share the context repository.")
		return
	}
	p, refusal := m.plan(row, body)
	if refusal != "" {
		fail(w, 422, "unprocessable_entity", refusal)
		return
	}
	if !p.any {
		reply(w, 200, wrap(m.answer(row)), rowTag(m.answer(row)))
		return
	}
	preview := m.preview(p)
	switch {
	case fingerprint != "" && fingerprint != p.fingerprint():
		preview["applied"], preview["stale"] = false, true
		f.previewAnswer(w, 409, "conflict", "What would move changed since the preview; nothing was written.", preview)
		return
	case fingerprint == "" && p.narrows:
		preview["applied"], preview["note"] = false, "Nothing was written: send this fingerprint as expected_fingerprint, with the same arguments, to write the change."
		f.previewAnswer(w, 428, "precondition_required", "This change takes something away: read the preview first.", preview)
		return
	}
	row.mode, row.repos, row.shared = p.mode, p.repos, p.shared
	m.writes++
	reply(w, 200, wrap(m.answer(row)), rowTag(m.answer(row)))
}
