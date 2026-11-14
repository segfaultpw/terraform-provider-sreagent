package fakefacade

import (
	"fmt"
	"slices"
	"strings"
)

// The seven repository settings that can follow the organization default, in
// the order the platform lists them. The first three belong to the whole
// repository.
var (
	followable = []string{"fix_runner", "platform_fallback", "auto_dispatch", "draft_prs", "check_network", "allow_ci_config", "auto_merge"}
	repoWide   = []string{"fix_runner", "platform_fallback", "auto_dispatch"}
)

func builtInDefaults() map[string]any {
	return map[string]any{
		"fix_runner": "platform", "platform_fallback": true, "auto_dispatch": true,
		"draft_prs": nil, "check_network": false, "allow_ci_config": false, "auto_merge": false,
	}
}

// SetDefaults sets the organization's repository defaults the repo_settings
// rows follow: a row stores only the settings it holds itself and answers the
// value in force for the rest.
func (f *Fake) SetDefaults(values map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for k, v := range values {
		f.defaults[k] = v
	}
}

// LegacyAnswers makes the resource answer the way a platform from before the
// repository defaults did: every setting stored on the row, no set_here, and
// no follows_default accepted.
func (f *Fake) LegacyAnswers(key string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.legacy[key] = true
}

func (f *Fake) following(spec string) bool { return spec == "repo_settings" && !f.legacy[spec] }

func heldOf(row map[string]any) []string {
	out := []string{}
	if l, ok := row["set_here"].([]any); ok {
		for _, e := range l {
			out = append(out, fmt.Sprint(e))
		}
	}
	return out
}

func setHeld(row map[string]any, held []string) {
	l := []any{}
	for _, n := range followable {
		if slices.Contains(held, n) {
			l = append(l, n)
		}
	}
	row["set_here"] = l
}

func (f *Fake) repoRows(repo string) []map[string]any {
	var out []map[string]any
	for _, r := range f.rows["repo_settings"] {
		if strings.EqualFold(fmt.Sprint(r["repo"]), repo) {
			out = append(out, r)
		}
	}
	return out
}

func isAllBranches(row map[string]any) bool {
	b, _ := row["branch"].(string)
	return b == ""
}

func (f *Fake) allBranchesRow(repo string) map[string]any {
	for _, r := range f.repoRows(repo) {
		if isAllBranches(r) {
			return r
		}
	}
	return nil
}

// effective is the value a row reads for a setting it does not hold.
func (f *Fake) effective(row map[string]any, name string) any {
	if !isAllBranches(row) {
		if all := f.allBranchesRow(fmt.Sprint(row["repo"])); all != nil && slices.Contains(heldOf(all), name) {
			return all[name]
		}
	}
	return f.defaults[name]
}

// present is the answer for a stored row.
func (f *Fake) present(spec string, row map[string]any) map[string]any {
	if !f.following(spec) {
		return row
	}
	out := map[string]any{}
	for k, v := range row {
		out[k] = v
	}
	held := heldOf(row)
	for _, n := range followable {
		if slices.Contains(held, n) {
			out[n] = row[n]
		} else {
			out[n] = f.effective(row, n)
		}
	}
	setHeld(out, held)
	delete(out, "follows_default")
	return out
}

// finishRepoCreate turns the generic create's row into the model: only the
// settings the body gave a value are held, a branch row copies the runner,
// the fallback and the sending of its repository, and follows_default resets.
func (f *Fake) finishRepoCreate(row, body map[string]any) {
	held := []string{}
	for _, n := range followable {
		if v, ok := body[n]; ok && v != nil {
			held = append(held, n)
		} else {
			delete(row, n)
		}
	}
	if !isAllBranches(row) {
		src := f.allBranchesRow(fmt.Sprint(row["repo"]))
		if src == nil {
			if rows := f.repoRows(fmt.Sprint(row["repo"])); len(rows) > 0 {
				src = rows[0]
			}
		}
		for _, n := range repoWide {
			if src != nil && !slices.Contains(held, n) && slices.Contains(heldOf(src), n) {
				row[n] = src[n]
				held = append(held, n)
			}
		}
	}
	setHeld(row, held)
	delete(row, "follows_default")
	delete(row, "following")
	f.applyFollows(row, body)
	// A repository-wide value written by a create reaches the repository's other rows.
	for _, n := range repoWide {
		if v, ok := body[n]; ok && v != nil {
			f.writeRepoWide(row, n, v)
		}
	}
}

// finishRepoUpdate keeps the held list in step with what an update wrote.
func (f *Fake) finishRepoUpdate(row, body map[string]any, before []string) {
	held := slices.Clone(before)
	for _, n := range followable {
		v, sent := body[n]
		switch {
		case !sent:
		case v == nil:
			held = slices.DeleteFunc(held, func(h string) bool { return h == n })
			delete(row, n)
		default:
			if !slices.Contains(held, n) {
				held = append(held, n)
			}
		}
	}
	setHeld(row, held)
	delete(row, "follows_default")
	f.applyFollows(row, body)
	for _, n := range repoWide {
		if v, ok := body[n]; ok && v != nil {
			f.writeRepoWide(row, n, v)
		}
	}
}

// applyFollows clears the settings a follows_default names: the repository-wide
// ones on every row of the repository, the others on this row.
func (f *Fake) applyFollows(row, body map[string]any) {
	names, _ := body["follows_default"].([]any)
	for _, e := range names {
		n := fmt.Sprint(e)
		targets := []map[string]any{row}
		if slices.Contains(repoWide, n) {
			targets = f.repoRows(fmt.Sprint(row["repo"]))
		}
		for _, t := range targets {
			held := slices.DeleteFunc(heldOf(t), func(h string) bool { return h == n })
			delete(t, n)
			setHeld(t, held)
		}
	}
}

func (f *Fake) writeRepoWide(row map[string]any, name string, v any) {
	for _, r := range f.repoRows(fmt.Sprint(row["repo"])) {
		r[name] = v
		if h := heldOf(r); !slices.Contains(h, name) {
			setHeld(r, append(h, name))
		}
	}
}

// followsRefusal is the platform's refusal of a setting that is both sent and reset.
func followsRefusal(body map[string]any) string {
	names, _ := body["follows_default"].([]any)
	for _, e := range names {
		n := fmt.Sprint(e)
		if v, sent := body[n]; sent && v != nil {
			return n + " is both set and reset to follow the default; send one."
		}
	}
	return ""
}

// autoMergeConfirmSentence is the platform's refusal of a write that turns
// auto-merge on without confirm naming it.
func autoMergeConfirmSentence(repo string) string {
	return "Turning on auto-merge for " + repo + " needs confirmation: pass confirm auto_merge."
}

// autoMergeConfirmRefusal judges a row write the way the platform does: it turns
// auto-merge on when it sets true on a row that does not hold true, or resets the
// setting onto a default that says on. before is the row as stored, nil for a
// create. An empty answer means the write may go ahead.
func (f *Fake) autoMergeConfirmRefusal(repo string, before, body map[string]any) string {
	given, _ := body["confirm"].([]any)
	if slices.Contains(given, any("auto_merge")) {
		return ""
	}
	if v, sent := body["auto_merge"]; sent && v == true && (before == nil || !slices.Contains(heldOf(before), "auto_merge") || before["auto_merge"] != true) {
		return autoMergeConfirmSentence(repo)
	}
	resets, _ := body["follows_default"].([]any)
	if slices.Contains(resets, any("auto_merge")) && f.defaults["auto_merge"] == true {
		return autoMergeConfirmSentence(repo)
	}
	return ""
}

// deleteRefusal is the platform's refusal of removing an all-branches entry that
// says off while the organization default says on: the branches no entry names
// would start merging by themselves.
func (f *Fake) deleteRefusal(spec string, row map[string]any) string {
	if spec != "repo_settings" || !f.following(spec) || !isAllBranches(row) {
		return ""
	}
	if slices.Contains(heldOf(row), "auto_merge") && row["auto_merge"] == false && f.defaults["auto_merge"] == true {
		return "Removing this entry would make fix pull requests on the other branches of " + fmt.Sprint(row["repo"]) + " merge by themselves, because the organization default says on and this entry says off. Turn the organization default off first, or keep this entry."
	}
	return ""
}
