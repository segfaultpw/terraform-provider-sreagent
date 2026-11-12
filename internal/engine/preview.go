package engine

import (
	"context"
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

func parsePreview(m map[string]any) preview {
	p := preview{Changes: map[string]previewChange{}}
	p.Fingerprint, _ = m["fingerprint"].(string)
	if changes, ok := m["changes"].(map[string]any); ok {
		for name, raw := range changes {
			if c, ok := raw.(map[string]any); ok {
				p.Changes[name] = previewChange{From: c["from"], To: c["to"]}
			}
		}
	}
	p.Moved = stringList(m["moved"])
	if n, ok := m["kept"].(float64); ok {
		p.Kept = int(n)
	}
	p.Differs = stringList(m["differs_by_branch"])
	if kids, ok := m["children"].([]any); ok {
		for _, raw := range kids {
			c, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			child := previewChild{}
			child.Name, _ = c["organization_name"].(string)
			if n, ok := c["moved"].(float64); ok {
				child.Moved = int(n)
			}
			if n, ok := c["ci_pinned"].(float64); ok {
				child.CIPinned = int(n)
			}
			child.NoRunner, _ = c["no_runner"].(bool)
			p.Children = append(p.Children, child)
		}
	}
	p.NeedsConfirm = stringList(m["needs_confirm"])
	p.Refusals = stringList(m["refusals"])
	p.LeavesParent, _ = m["leaves_parent"].(bool)
	return p
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
		if a.Computed || a.WriteOnly || a.Local || a.Secret {
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
		return parsePreview(apiErr.Preview), nil
	}
	return preview{}, err
}

func (r *facadeResource) staleError(diags *diag.Diagnostics, verb string, err error) {
	detail := "What would move changed since the plan (the platform answered 409), so nothing was written. Run terraform plan again to review the fresh preview."
	var apiErr *client.APIError
	if errors.As(err, &apiErr) && len(apiErr.Preview) > 0 {
		detail += "\n\nThe fresh preview: " + parsePreview(apiErr.Preview).warning()
	}
	diags.AddError(r.title(verb), detail)
}

// previewedWrite sends a previewed write. With the plan's fingerprint it sends
// that and nothing else can happen but a write or a refusal. Without one (a
// plan made with a key that cannot preview) it takes the preview itself, judges
// it as the plan would have, says what moves and applies that fingerprint. It
// answers the fingerprint it applied.
func (r *facadeResource) previewedWrite(ctx context.Context, diags *diag.Diagnostics, verb string, body map[string]any, ifMatch string, confirm []string, planned string) (*client.Response, string, error) {
	if planned != "" {
		out, err := r.client.Do(ctx, r.previewedRequest(body, ifMatch, confirm, planned))
		if client.IsConflict(err) {
			r.staleError(diags, verb, err)
			return nil, "", errReported
		}
		return out, planned, err
	}
	for attempt := 0; ; attempt++ {
		p, err := r.previewCall(ctx, body, ifMatch, confirm)
		if err != nil {
			return nil, "", err
		}
		if probs := r.problems(p, confirm); len(probs) > 0 {
			r.addProblems(diags, probs)
			return nil, "", errReported
		}
		diags.AddWarning("Applying against a preview taken now", "The plan could not preview this change, so the apply did. "+p.warning())
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
	if len(body) == 0 {
		held := types.StringNull()
		if !creating {
			resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root(fingerprintAttr), &held)...)
		}
		setFingerprint(held)
		return
	}
	confirm, known, d := r.configuredConfirm(ctx, req.Config)
	resp.Diagnostics.Append(d...)
	if r.client == nil || !known || !r.settingsKnown(ctx, req.Config) {
		setFingerprint(types.StringUnknown())
		return
	}
	ifMatch := ""
	if creating {
		current, err := r.client.Do(ctx, client.Request{Method: http.MethodGet, Path: r.spec.Key})
		switch {
		case err == nil:
			ifMatch = current.ETag
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

// destroyPreviewed sends DestroyBody as a previewed write.
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
	_, _, err := r.previewedWrite(ctx, &resp.Diagnostics, "delete", maps.Clone(r.spec.DestroyBody), etag, nil, "")
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
