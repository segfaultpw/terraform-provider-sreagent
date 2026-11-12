package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/segfaultpw/terraform-provider-sreagent/internal/client"
)

// The two attributes of a Previewed spec the engine names itself: the Local
// string that carries the plan's fingerprint to the apply, and the write-only
// list of confirmations.
const (
	fingerprintAttr = "preview_fingerprint"
	confirmAttr     = "confirm"
	// confirmOnDestroyAttr is the Kept list a destroy reads its confirmations
	// from, since a destroy has no configuration; destroyFingerprintKey is the
	// private key that carries the destroy plan's fingerprint to the delete.
	confirmOnDestroyAttr  = "confirm_on_destroy"
	destroyFingerprintKey = "destroy_fingerprint"
	// namedRepositories is how many repositories a warning names before it
	// counts the rest.
	namedRepositories = 10
)

type previewChange struct{ From, To any }

type previewChild struct {
	Name     string
	Moved    int
	CIPinned int
	NoRunner bool
}

// preview is the platform's account of what a previewed write would do.
type preview struct {
	Fingerprint  string
	Changes      map[string]previewChange
	Moved        []string
	Kept         int
	Differs      []string
	Children     []previewChild
	NeedsConfirm []string
	Refusals     []string
	LeavesParent bool
}

// previewKeys are the keys of preview.data the provider reads, with the JSON
// type each must have. The platform emits every one of them in every preview,
// so an answer without one is not a preview this provider version understands:
// reading it as "nothing moves" would plan a warning that is false.
var previewKeys = map[string]string{
	"fingerprint":       "string",
	"changes":           "object",
	"moved":             "array",
	"kept":              "number",
	"differs_by_branch": "array",
	"children":          "array",
	"needs_confirm":     "array",
	"refusals":          "array",
	"leaves_parent":     "boolean",
}

// previewChildKeys are the keys of each entry of preview.data.children.
var previewChildKeys = map[string]string{
	"organization_name": "string",
	"moved":             "number",
	"ci_pinned":         "number",
	"no_runner":         "boolean",
}

// PreviewKeys are the keys of preview.data the provider reads, for the contract
// test that holds them to the platform's published shape.
func PreviewKeys() []string { return slices.Sorted(maps.Keys(previewKeys)) }

// PreviewChildKeys are the keys of each following child in preview.data.children.
func PreviewChildKeys() []string { return slices.Sorted(maps.Keys(previewChildKeys)) }

func jsonKind(v any) string {
	switch v.(type) {
	case string:
		return "string"
	case map[string]any:
		return "object"
	case []any:
		return "array"
	case float64:
		return "number"
	case bool:
		return "boolean"
	}
	return "null"
}

// unreadablePreview is the error for an answer that does not carry the keys the
// provider reads.
type unreadablePreview struct{ problems []string }

func (e *unreadablePreview) Error() string {
	return "the platform answered a preview this provider version cannot read (" + strings.Join(e.problems, "; ") +
		"), so nothing was written. Upgrade the provider, or report this if it is already the latest."
}

func keyProblems(m map[string]any, keys map[string]string, where string) []string {
	var out []string
	for _, name := range slices.Sorted(maps.Keys(keys)) {
		v, ok := m[name]
		switch {
		case !ok:
			out = append(out, where+name+" is missing")
		case jsonKind(v) != keys[name]:
			out = append(out, fmt.Sprintf("%s%s is %s, not %s", where, name, jsonKind(v), keys[name]))
		}
	}
	return out
}

// parsePreview reads a preview.data. It fails, naming the keys, when one the
// provider reads is absent or of another type, and when the fingerprint that
// applies the preview is empty.
func parsePreview(m map[string]any) (preview, error) {
	if len(m) == 0 {
		return preview{}, &unreadablePreview{problems: []string{"the answer carries no preview"}}
	}
	problems := keyProblems(m, previewKeys, "")
	if fp, ok := m["fingerprint"].(string); ok && fp == "" {
		problems = append(problems, "fingerprint is empty")
	}
	kids, _ := m["children"].([]any)
	for i, raw := range kids {
		c, ok := raw.(map[string]any)
		if !ok {
			problems = append(problems, fmt.Sprintf("children[%d] is %s, not object", i, jsonKind(raw)))
			continue
		}
		problems = append(problems, keyProblems(c, previewChildKeys, fmt.Sprintf("children[%d].", i))...)
	}
	changes, _ := m["changes"].(map[string]any)
	for _, name := range slices.Sorted(maps.Keys(changes)) {
		c, ok := changes[name].(map[string]any)
		if !ok {
			problems = append(problems, fmt.Sprintf("changes.%s is %s, not object", name, jsonKind(changes[name])))
			continue
		}
		for _, end := range []string{"from", "to"} {
			if _, has := c[end]; !has {
				problems = append(problems, fmt.Sprintf("changes.%s.%s is missing", name, end))
			}
		}
	}
	if len(problems) > 0 {
		return preview{}, &unreadablePreview{problems: problems}
	}
	p := preview{Changes: map[string]previewChange{}}
	p.Fingerprint, _ = m["fingerprint"].(string)
	for name, raw := range changes {
		c := raw.(map[string]any)
		p.Changes[name] = previewChange{From: c["from"], To: c["to"]}
	}
	p.Moved = stringList(m["moved"])
	n, _ := m["kept"].(float64)
	p.Kept = int(n)
	p.Differs = stringList(m["differs_by_branch"])
	for _, raw := range kids {
		c := raw.(map[string]any)
		child := previewChild{}
		child.Name, _ = c["organization_name"].(string)
		moved, _ := c["moved"].(float64)
		child.Moved = int(moved)
		pinned, _ := c["ci_pinned"].(float64)
		child.CIPinned = int(pinned)
		child.NoRunner, _ = c["no_runner"].(bool)
		p.Children = append(p.Children, child)
	}
	p.NeedsConfirm = stringList(m["needs_confirm"])
	p.Refusals = stringList(m["refusals"])
	p.LeavesParent, _ = m["leaves_parent"].(bool)
	return p, nil
}

func stringList(v any) []string {
	items, _ := v.([]any)
	out := make([]string, 0, len(items))
	for _, e := range items {
		if s, ok := e.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func show(v any) string {
	if v == nil {
		return "unset"
	}
	return fmt.Sprint(v)
}

func repositories(n int) string {
	if n == 1 {
		return "1 repository"
	}
	return fmt.Sprintf("%d repositories", n)
}

// warning is what a person reads before the change is applied: the settings
// that change, the repositories that move, and what stays.
func (p preview) warning() string {
	var b strings.Builder
	if len(p.Changes) == 0 {
		b.WriteString("This change moves no setting.")
	} else {
		names := slices.Sorted(maps.Keys(p.Changes))
		parts := make([]string, 0, len(names))
		for _, n := range names {
			c := p.Changes[n]
			parts = append(parts, fmt.Sprintf("%s from %s to %s", n, show(c.From), show(c.To)))
		}
		b.WriteString("This change sets " + strings.Join(parts, "; ") + ".")
	}
	switch len(p.Moved) {
	case 0:
		b.WriteString(" No repository moves.")
	default:
		shown := p.Moved
		more := ""
		if len(shown) > namedRepositories {
			more = fmt.Sprintf(" and %d more", len(shown)-namedRepositories)
			shown = shown[:namedRepositories]
		}
		fmt.Fprintf(&b, " %s will read a different value: %s%s.", repositories(len(p.Moved)), strings.Join(shown, ", "), more)
	}
	if p.Kept > 0 {
		fmt.Fprintf(&b, " %s keep a value of their own and do not move.", repositories(p.Kept))
	}
	if len(p.Differs) > 0 {
		fmt.Fprintf(&b, " Branches differ from the repository's own value for: %s.", strings.Join(p.Differs, ", "))
	}
	for _, c := range p.Children {
		fmt.Fprintf(&b, " Child organization %s follows these defaults: %s will read a different value", c.Name, repositories(c.Moved))
		if c.CIPinned > 0 {
			fmt.Fprintf(&b, ", %s pinned to keep CI configuration changes", repositories(c.CIPinned))
		}
		if c.NoRunner {
			b.WriteString(", and it has no fix runner registered")
		}
		b.WriteString(".")
	}
	if p.LeavesParent {
		b.WriteString(" This organization follows its parent's repository defaults. This apply gives it its own for all six settings, and later changes of the parent's defaults no longer reach it; destroy this resource to follow the parent again.")
	}
	return b.String()
}

// problem is a refusal a preview carries, against an attribute or the resource.
type problem struct {
	attr    string
	summary string
	detail  string
}

// problems are the refusals and the missing confirmations of a preview.
func (r *facadeResource) problems(p preview, confirm []string) []problem {
	var out []problem
	if len(p.Refusals) > 0 {
		out = append(out, problem{summary: "The platform refuses this change", detail: strings.Join(p.Refusals, "\n")})
	}
	for _, name := range p.NeedsConfirm {
		if !slices.Contains(confirm, name) {
			out = append(out, r.missingConfirm(name))
		}
	}
	return out
}

// destroyProblems are the refusals and missing confirmations of the preview of a
// destroy. A destroy has no configuration to confirm in, so the way out is
// named: confirm_on_destroy, applied before the destroy, and for a refusal the
// ways to stop managing the defaults without changing them.
func (r *facadeResource) destroyProblems(p preview, confirm []string) []problem {
	var out []problem
	if len(p.Refusals) > 0 {
		out = append(out, problem{summary: "The platform refuses to hand the defaults back to the parent", detail: strings.Join(p.Refusals, "\n") +
			"\n\nThis destroy would fail, so it is stopped before anything is written. Change what the platform refuses and destroy again." + r.destroyExits()})
	}
	for _, name := range p.NeedsConfirm {
		if !slices.Contains(confirm, name) {
			c := r.spec.Confirmations[name]
			out = append(out, problem{summary: "Confirmation needed to destroy", detail: fmt.Sprintf("%s Destroying this resource turns it on for every repository that reads the parent's defaults, so the platform wants the confirmation first. In Terraform, set %s = [%q] on sreagent_%s, run terraform apply, then destroy. Terraform never gives the confirmation for you.", c.Sentence, confirmOnDestroyAttr, name, r.spec.TypeName)})
		}
	}
	return out
}

// destroyExits names the ways to stop managing the defaults without changing
// them, for a destroy plan that fails; neither calls the platform.
func (r *facadeResource) destroyExits() string {
	return fmt.Sprintf("\n\nTo stop managing the defaults without changing them, run terraform state rm sreagent_%[1]s.<name>, or replace the resource block with a removed block that keeps what the platform holds:\n\nremoved {\n  from = sreagent_%[1]s.<name>\n\n  lifecycle {\n    destroy = false\n  }\n}", r.spec.TypeName)
}

func (r *facadeResource) missingConfirm(name string) problem {
	c := r.spec.Confirmations[name]
	return problem{attr: confirmAttr, summary: "Confirmation needed",
		detail: fmt.Sprintf("%s In Terraform, set %s = [%q] on sreagent_%s.", c.Sentence, confirmAttr, name, r.spec.TypeName)}
}

// confirmedFromState is the confirmation check a plan can make without a
// preview: a setting the plan turns on that state does not hold needs the
// confirmation the spec names for it.
func (r *facadeResource) confirmedFromState(ctx context.Context, plan, state valueSource, confirm []string) []problem {
	var out []problem
	for _, name := range slices.Sorted(maps.Keys(r.spec.Confirmations)) {
		a, ok := r.attrNamed(r.spec.Confirmations[name].Attr)
		if !ok || slices.Contains(confirm, name) {
			continue
		}
		var planned types.Bool
		if plan.GetAttribute(ctx, path.Root(a.Attribute()), &planned).HasError() || planned.IsNull() || planned.IsUnknown() || !planned.ValueBool() {
			continue
		}
		var held types.Bool
		if state != nil && state.GetAttribute(ctx, path.Root(a.Attribute()), &held).HasError() {
			continue
		}
		if state != nil && !held.IsNull() && !held.IsUnknown() && held.ValueBool() {
			continue
		}
		out = append(out, r.missingConfirm(name))
	}
	return out
}

func (r *facadeResource) addProblems(diags *diag.Diagnostics, probs []problem) {
	for _, pr := range probs {
		if pr.attr != "" {
			diags.AddAttributeError(path.Root(pr.attr), pr.summary, pr.detail)
		} else {
			diags.AddError(pr.summary, pr.detail)
		}
	}
}

// configuredConfirm is the confirmations the configuration states; known is
// false when they depend on a value not yet known.
func (r *facadeResource) configuredConfirm(ctx context.Context, cfg valueSource) (names []string, known bool, diags diag.Diagnostics) {
	var list types.List
	diags.Append(cfg.GetAttribute(ctx, path.Root(confirmAttr), &list)...)
	if list.IsUnknown() {
		return nil, false, diags
	}
	return stringsOf(list), true, diags
}

// settingsKnown reports that every setting the configuration states is known.
func (r *facadeResource) settingsKnown(ctx context.Context, cfg valueSource) bool {
	for _, a := range r.spec.Attrs {
		if a.Computed || a.WriteOnly || a.Local || a.Kept || a.Secret {
			continue
		}
		v, d := getValue(ctx, cfg, a)
		if d.HasError() || (v != nil && v.IsUnknown()) {
			return false
		}
	}
	return true
}

// errReported marks a failure already added to the diagnostics.
var errReported = errors.New("reported")

func (r *facadeResource) previewedRequest(body map[string]any, ifMatch string, confirm []string, fingerprint string) client.Request {
	b := maps.Clone(body)
	if len(confirm) > 0 {
		b[confirmAttr] = confirm
	}
	if fingerprint != "" {
		b["expected_fingerprint"] = fingerprint
	}
	return client.Request{Method: http.MethodPut, Path: r.spec.Key, Body: b, IfMatch: ifMatch}
}

// previewCall is the first request of a previewed write: it answers the preview
// and writes nothing.
func (r *facadeResource) previewCall(ctx context.Context, body map[string]any, ifMatch string, confirm []string) (preview, error) {
	_, err := r.client.Do(ctx, r.previewedRequest(body, ifMatch, confirm, ""))
	var apiErr *client.APIError
	switch {
	case err == nil:
		return preview{}, fmt.Errorf("the platform wrote sreagent_%s instead of previewing it, so no fingerprint could be taken", r.spec.TypeName)
	case errors.As(err, &apiErr) && apiErr.Status == http.StatusPreconditionRequired:
		return parsePreview(apiErr.Preview)
	}
	return preview{}, err
}

func (r *facadeResource) staleError(diags *diag.Diagnostics, verb string, err error) {
	rerun := "terraform plan"
	if verb == "delete" {
		rerun = "terraform plan -destroy"
	}
	if client.IsRetried(err) {
		diags.AddError(r.title(verb), fmt.Sprintf("The platform answered 409 to a retried request after a gateway error, so the first attempt may have been applied. Run %s to see the defaults as they are now.", rerun))
		return
	}
	detail := fmt.Sprintf("What would move changed since the plan (the platform answered 409), so nothing was written. Run %s again to review the fresh preview.", rerun)
	var apiErr *client.APIError
	if errors.As(err, &apiErr) && len(apiErr.Preview) > 0 {
		if fresh, perr := parsePreview(apiErr.Preview); perr != nil {
			detail += "\n\nThe fresh preview could not be read: " + perr.Error()
		} else {
			detail += "\n\nThe fresh preview: " + fresh.warning()
		}
	}
	diags.AddError(r.title(verb), detail)
}

// previewedWrite sends a previewed write. With the plan's fingerprint it sends
// that and nothing else can happen but a write or a refusal. Without one (a
// plan made with a key that cannot preview, or a destroy whose plan carried
// nothing) it takes the preview itself, judges it as the plan would have, says
// what moves and applies that fingerprint. It answers the fingerprint it applied.
func (r *facadeResource) previewedWrite(ctx context.Context, diags *diag.Diagnostics, verb string, body map[string]any, ifMatch string, confirm []string, planned string) (*client.Response, string, error) {
	if planned != "" {
		out, err := r.client.Do(ctx, r.previewedRequest(body, ifMatch, confirm, planned))
		if client.IsConflict(err) {
			r.staleError(diags, verb, err)
			return nil, "", errReported
		}
		return out, planned, err
	}
	warned := false
	for attempt := 0; ; attempt++ {
		p, err := r.previewCall(ctx, body, ifMatch, confirm)
		if err != nil {
			return nil, "", err
		}
		probs := r.problems(p, confirm)
		if verb == "delete" {
			probs = r.destroyProblems(p, confirm)
		}
		if len(probs) > 0 {
			r.addProblems(diags, probs)
			return nil, "", errReported
		}
		if !warned {
			if verb == "delete" {
				diags.AddWarning("Destroying against a preview taken now", "The destroy plan carried no preview, so the destroy took one. "+p.warning())
			} else {
				diags.AddWarning("Applying against a preview taken now", "The plan could not preview this change, so the apply did. "+p.warning())
			}
			warned = true
		}
		out, err := r.client.Do(ctx, r.previewedRequest(body, ifMatch, confirm, p.Fingerprint))
		if client.IsConflict(err) {
			if attempt == 0 {
				continue
			}
			r.staleError(diags, verb, err)
			return nil, "", errReported
		}
		return out, p.Fingerprint, err
	}
}

// platformTooOld is the sentence for a 404 on the singleton itself.
func (r *facadeResource) platformTooOld(err error) string {
	if r.spec.MinPlatform == "" {
		return err.Error()
	}
	return fmt.Sprintf("%s\n\nsreagent_%s needs platform %s or later.", err.Error(), r.spec.TypeName, r.spec.MinPlatform)
}

// followsParent reports the 404 a child organization gets for a singleton it
// reads from its parent. The one other 404 the singleton can answer is a
// platform that does not serve the resource at all, which names it as one, so
// the sentence of the parent case is not what is matched.
func followsParent(err error) bool {
	var apiErr *client.APIError
	return errors.As(err, &apiErr) && apiErr.Status == http.StatusNotFound && !strings.HasPrefix(apiErr.Message, "No such configuration resource")
}

// planPreview takes the plan-time preview of a Previewed write and carries its
// fingerprint to the apply in the Local attribute.
func (r *facadeResource) planPreview(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	creating := req.State.Raw.IsNull()
	setFingerprint := func(v types.String) {
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root(fingerprintAttr), v)...)
	}
	var prior valueSource
	o := opCreate
	if !creating {
		prior, o = req.State, opUpdate
	}
	body, _, d := r.body(ctx, req.Plan, req.Config, prior, o)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}
	confirm, known, d := r.configuredConfirm(ctx, req.Config)
	resp.Diagnostics.Append(d...)
	settings := r.settingsKnown(ctx, req.Config)
	if settings && len(body) == 0 {
		held := types.StringNull()
		if !creating {
			resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root(fingerprintAttr), &held)...)
		}
		setFingerprint(held)
		return
	}
	if !known || !settings {
		if r.client != nil {
			resp.Diagnostics.AddWarning("This change is not previewed",
				"A value in it is known only at apply, so the plan cannot ask the platform what it moves; the apply previews it and applies what it finds.")
			if known {
				r.addProblems(&resp.Diagnostics, r.confirmedFromState(ctx, req.Plan, prior, confirm))
			}
		}
		setFingerprint(types.StringUnknown())
		return
	}
	if r.client == nil {
		setFingerprint(types.StringUnknown())
		return
	}
	ifMatch := ""
	if creating {
		current, err := r.client.Do(ctx, client.Request{Method: http.MethodGet, Path: r.spec.Key})
		switch {
		case err == nil:
			ifMatch = current.ETag
			if row, derr := decodeRow(current.Data); derr == nil {
				prior = answered{row: row}
			}
		case client.IsNotFound(err) && followsParent(err):
		case client.IsNotFound(err):
			resp.Diagnostics.AddError(r.title("plan"), r.platformTooOld(err))
			return
		case client.IsReadOnlyKey(err):
		default:
			resp.Diagnostics.AddError(r.title("plan"), err.Error())
			return
		}
	} else if req.Private != nil {
		tag, d := r.etag(ctx, req.Private)
		resp.Diagnostics.Append(d...)
		ifMatch = tag
	}
	p, err := r.previewCall(ctx, body, ifMatch, confirm)
	switch {
	case err == nil:
	case client.IsReadOnlyKey(err):
		resp.Diagnostics.AddWarning("This change is not previewed",
			"The preview of what this change moves needs an api:admin key; the apply previews it and applies what it finds.")
		r.addProblems(&resp.Diagnostics, r.confirmedFromState(ctx, req.Plan, prior, confirm))
		setFingerprint(types.StringUnknown())
		return
	case client.IsPreconditionFailed(err):
		summary, detail := r.precondition("plan", err)
		resp.Diagnostics.AddError(summary, detail)
		return
	case client.IsNotFound(err):
		resp.Diagnostics.AddError(r.title("plan"), r.platformTooOld(err))
		return
	default:
		resp.Diagnostics.AddError(r.title("plan"), err.Error())
		return
	}
	if probs := r.problems(p, confirm); len(probs) > 0 {
		r.addProblems(&resp.Diagnostics, probs)
		return
	}
	resp.Diagnostics.AddWarning("What this change moves", p.warning())
	setFingerprint(types.StringValue(p.Fingerprint))
}

// createPreviewed adopts the organization's row the way a singleton does, with
// the write taken against the preview.
func (r *facadeResource) createPreviewed(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse, body map[string]any) {
	confirm, _, d := r.configuredConfirm(ctx, req.Config)
	resp.Diagnostics.Append(d...)
	var planned types.String
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root(fingerprintAttr), &planned)...)
	if resp.Diagnostics.HasError() {
		return
	}
	current, err := r.client.Do(ctx, client.Request{Method: http.MethodGet, Path: r.spec.Key})
	ifMatch := ""
	switch {
	case err == nil:
		ifMatch = current.ETag
		if len(body) > 0 && ifMatch == "" {
			resp.Diagnostics.AddError(r.title("create"), fmt.Sprintf("the platform answered the current sreagent_%s without a row version, so adopting it could overwrite a change made in the app; nothing was written", r.spec.TypeName))
			return
		}
	case followsParent(err):
	case client.IsNotFound(err):
		resp.Diagnostics.AddError(r.title("create"), r.platformTooOld(err))
		return
	default:
		resp.Diagnostics.AddError(r.title("create"), err.Error())
		return
	}
	out, used := current, ""
	if len(body) == 0 {
		if err != nil {
			resp.Diagnostics.AddError(r.title("create"), fmt.Sprintf("sreagent_%s has no row of its own yet; set at least one attribute to create it", r.spec.TypeName))
			return
		}
	} else {
		out, used, err = r.previewedWrite(ctx, &resp.Diagnostics, "create", body, ifMatch, confirm, planned.ValueString())
		if errors.Is(err, errReported) {
			return
		}
		switch {
		case client.IsReadOnlyKey(err):
			summary, detail := r.readOnly("create")
			resp.Diagnostics.AddError(summary, detail)
			return
		case client.IsPreconditionFailed(err):
			summary, detail := r.precondition("create", err)
			resp.Diagnostics.AddError(summary, detail)
			return
		case err != nil:
			resp.Diagnostics.AddError(r.title("create"), err.Error())
			return
		}
	}
	resp.Diagnostics.Append(r.writeState(ctx, out, req.Plan, &resp.State, resp.Private, resp.Identity)...)
	r.recordFingerprint(ctx, &resp.State, &resp.Diagnostics, planned, used)
}

// recordFingerprint stores the fingerprint the plan promised, or the one the
// apply took when the plan could not.
func (r *facadeResource) recordFingerprint(ctx context.Context, state interface {
	SetAttribute(context.Context, path.Path, any) diag.Diagnostics
}, diags *diag.Diagnostics, planned types.String, used string) {
	value := types.StringNull()
	switch {
	case !planned.IsNull() && !planned.IsUnknown():
		value = planned
	case used != "":
		value = types.StringValue(used)
	}
	diags.Append(state.SetAttribute(ctx, path.Root(fingerprintAttr), value)...)
}

// destroyFollows reports that state says the destroy takes effect.
func (r *facadeResource) destroyFollows(ctx context.Context, state valueSource) bool {
	if r.spec.DestroyBody == nil {
		return false
	}
	a, ok := r.attrNamed(r.spec.DestroyWhen)
	if !ok {
		return false
	}
	v, d := getValue(ctx, state, a)
	return !d.HasError() && v != nil && !v.IsNull() && !v.IsUnknown()
}

// kept is the list a Kept attribute holds in a state.
func (r *facadeResource) kept(ctx context.Context, state valueSource, name string) []string {
	var list types.List
	if state.GetAttribute(ctx, path.Root(name), &list).HasError() || list.IsNull() || list.IsUnknown() {
		return nil
	}
	return stringsOf(list)
}

// planDestroy plans a destroy like a change: it asks the platform what the
// destroy moves, fails the plan when the platform would refuse it or wants a
// confirmation the state does not hold (confirm_on_destroy), warns with what
// moves, and carries the fingerprint to the delete in private state.
func (r *facadeResource) planDestroy(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if r.client == nil || req.State.Raw.IsNull() || r.spec.DestroyBody == nil {
		return
	}
	if !r.destroyFollows(ctx, req.State) {
		resp.Diagnostics.AddWarning("What destroying this resource leaves",
			"Destroying this resource stops Terraform managing the defaults. The platform keeps them, and every repository that follows them keeps reading them.")
		return
	}
	ifMatch := ""
	if req.Private != nil {
		tag, d := r.etag(ctx, req.Private)
		resp.Diagnostics.Append(d...)
		ifMatch = tag
	}
	confirm := r.kept(ctx, req.State, confirmOnDestroyAttr)
	p, err := r.previewCall(ctx, maps.Clone(r.spec.DestroyBody), ifMatch, confirm)
	switch {
	case err == nil:
	case client.IsReadOnlyKey(err):
		resp.Diagnostics.AddWarning("This destroy is not previewed",
			"The preview of what this destroy moves needs an api:admin key; the destroy previews it, and applies it only when the platform accepts it.")
		return
	case client.IsNotFound(err):
		return
	case client.IsPreconditionFailed(err):
		summary, detail := r.precondition("plan", err)
		resp.Diagnostics.AddError(summary, detail)
		return
	default:
		resp.Diagnostics.AddError(r.title("plan"), err.Error()+r.destroyExits())
		return
	}
	if probs := r.destroyProblems(p, confirm); len(probs) > 0 {
		r.addProblems(&resp.Diagnostics, probs)
		return
	}
	var stopped types.Bool
	ends := false
	if a, ok := r.attrNamed("stopped"); ok && !req.State.GetAttribute(ctx, path.Root(a.Attribute()), &stopped).HasError() {
		ends = stopped.ValueBool()
	}
	moves := len(p.Changes) > 0 || len(p.Moved) > 0 || len(p.Children) > 0
	switch {
	case moves:
		text := "Destroying this resource hands the organization back to its parent's repository defaults, and the values it holds of its own are dropped. " + p.warning()
		if ends {
			text += " The stop this organization made is ended too, so later changes of the parent's defaults reach it again."
		}
		resp.Diagnostics.AddWarning("What destroying this resource moves", text)
	case ends:
		resp.Diagnostics.AddWarning("What destroying this resource moves",
			"Destroying this resource hands the organization back to its parent's repository defaults, and it moves no value now. The stop this organization made is ended too, so later changes of the parent's defaults reach it again.")
	}
	if resp.Private != nil {
		b, _ := json.Marshal(p.Fingerprint)
		resp.Diagnostics.Append(resp.Private.SetKey(ctx, destroyFingerprintKey, b)...)
	}
}

// destroyPreviewed sends DestroyBody as a previewed write, against the
// fingerprint the destroy plan took, or a preview of its own when the plan
// could not carry one.
func (r *facadeResource) destroyPreviewed(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	etag, d := r.etag(ctx, req.Private)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}
	if etag == "" {
		summary, detail := r.noVersion("delete")
		resp.Diagnostics.AddError(summary, detail)
		return
	}
	planned := ""
	if b, d := req.Private.GetKey(ctx, destroyFingerprintKey); !d.HasError() && len(b) > 0 {
		_ = json.Unmarshal(b, &planned)
	}
	confirm := r.kept(ctx, req.State, confirmOnDestroyAttr)
	_, _, err := r.previewedWrite(ctx, &resp.Diagnostics, "delete", maps.Clone(r.spec.DestroyBody), etag, confirm, planned)
	switch {
	case err == nil:
		if r.spec.DestroyWarning != "" {
			resp.Diagnostics.AddWarning("Defaults handed back", r.spec.DestroyWarning)
		}
	case errors.Is(err, errReported), client.IsNotFound(err):
	case client.IsReadOnlyKey(err):
		summary, detail := r.readOnly("delete")
		resp.Diagnostics.AddError(summary, detail)
	case client.IsPreconditionFailed(err):
		summary, detail := r.precondition("delete", err)
		resp.Diagnostics.AddError(summary, detail)
	default:
		resp.Diagnostics.AddError(r.title("delete"), err.Error())
	}
}

// importCheck is the import of a singleton a child organization reads from its
// parent: there is nothing of its own to import.
func (r *facadeResource) importCheck(ctx context.Context, resp *resource.ImportStateResponse) {
	if r.client == nil {
		return
	}
	_, err := r.client.Do(ctx, client.Request{Method: http.MethodGet, Path: r.spec.Key})
	switch {
	case followsParent(err):
		resp.Diagnostics.AddError("Nothing to import", fmt.Sprintf("This organization follows its parent's %s, so it has none of its own to import. Declare sreagent_%s with the settings to give it its own (the plan says what moves), or leave the resource out to keep following.", r.spec.Key, r.spec.TypeName))
	case client.IsNotFound(err):
		resp.Diagnostics.AddError(r.title("import"), r.platformTooOld(err))
	}
}

// CheckPreview reports why a preview.data is not one this provider version can
// read, for the contract test that holds the platform's fixture to it.
func CheckPreview(m map[string]any) error {
	_, err := parsePreview(m)
	return err
}
