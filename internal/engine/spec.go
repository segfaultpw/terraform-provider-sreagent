// Package engine turns a declarative Spec of one configuration API resource
// into a Terraform resource and data source.
package engine

import "strings"

// Kind is an attribute's value type.
type Kind int

// The kinds an attribute can hold. JSON is a free-form object or array kept
// as normalized text, so key order never reads as drift.
const (
	String Kind = iota
	Int
	Float
	Bool
	StringList
	JSON
)

// Attr is one field of a resource. Flags mirror x-sreagent-resources in the
// OpenAPI document; internal/contract proves each one against it.
type Attr struct {
	// Name is the API's field name.
	Name string
	// TFName is the Terraform attribute when Name cannot be one: provider is
	// reserved in every resource and data source block.
	TFName      string
	Description string
	Kind        Kind
	// Required: in the create schema's required list.
	Required bool
	// CreateOnly: sent on create, absent from update, so a change replaces the row.
	CreateOnly bool
	// UpdateOnly: absent from create; sent by a follow-up PUT after create.
	UpdateOnly bool
	// Clearable: a nullable collection field; unset sends null on update.
	Clearable bool
	// NullMeans: for a Clearable field, the JSON literal of the value the
	// platform acts on when the field is null, which is also what a create
	// that leaves it out stores. An answer equal to it reads back as the
	// null an unset configuration holds. A Clearable object's is always {}.
	NullMeans string
	// Computed: answered, never written.
	Computed bool
	// Recomputed: the platform derives this attribute from another one the
	// same write can change (path_prefix from path_prefixes), so a prior value
	// is not carried into a plan that changes the resource: it stays unknown
	// until the answer, instead of keeping a value the write is about to move.
	Recomputed bool
	// Secret: exposed as <name>_wo, <name>_wo_version and <name>_set.
	Secret bool
	// NotRead: an action argument the server never answers; state keeps config.
	// It is Optional without Computed and replaces the row when it changes.
	NotRead bool
	// Normalize: the server stores this form; values equal after it are equal.
	Normalize func(string) string
	OneOf     []string
	// HiddenKeys: top-level keys of a JSON attribute the platform stores but
	// never answers, because they can carry a credential. Terraform could only
	// hold them in state, so configuration naming one is refused. The platform
	// keeps them when a write leaves them out, and the attribute is sent on
	// update only when it changed.
	HiddenKeys []string
	// MergedObject: a JSON object the platform merges into what it stores, so
	// a key left out is kept; an update names each dropped key with "".
	MergedObject bool
	// MaxItems: the longest list the platform stores, as its API document
	// publishes it (maxItems); a longer one is refused at plan.
	MaxItems int
	// FieldNames: a list of JSON field names the platform stores as written. It
	// does not trim them, so a padded name is valid; a name that is blank, over
	// 128 code points or holds a control character is refused.
	FieldNames bool
	// RefusedWith names another attribute of the same resource that must be
	// empty when this list is not. The platform refuses the pair with
	// RefusedWhy; both are checked at plan, where the configuration says so.
	RefusedWith string
	RefusedWhy  string
	// DroppedWhenAttr and DroppedWhenValue: for a Secret, the platform discards
	// the stored secret when the string attribute DroppedWhenAttr (an API name)
	// is DroppedWhenValue, whatever the configuration sends, so the plan says
	// the secret is no longer set instead of keeping the prior state's true.
	DroppedWhenAttr  string
	DroppedWhenValue string
	// WriteOnly: a list action argument the API never answers. It is read from
	// the configuration, never held in plan or state, and sent only when the
	// resource decides to (Spec.FollowsDefault), so it can never be a diff by
	// itself and an import has nothing to plan for it. Its OneOf bounds the
	// items.
	WriteOnly bool
	// Complement: Computed. The members of this list that the answered list
	// named Name does not hold, in this list's order; null when the API answers
	// no such list.
	Complement []string
	// NullUnlessIn: for a Clearable attribute whose answer is the value in force
	// rather than the value the row holds. When the answered list named here does
	// not hold this attribute's name and the prior value is null, the attribute
	// stays null: the row follows a default, and a configuration that leaves the
	// attribute out is satisfied by it.
	NullUnlessIn string
	// IgnoredValues: entries of a list that the platform keeps through every
	// write of this attribute. A read drops them and a write never sends them,
	// and a configuration that names one is refused at validate with IgnoredWhy.
	IgnoredValues []string
	IgnoredWhy    string
	// Kept: an Optional list of the configuration that the provider keeps in
	// state and never sends or reads from the API: a value a later operation
	// that has no configuration (a destroy) needs. It is never a write by itself.
	Kept bool
	// Local: Computed and kept by the provider alone. It is never sent, never read
	// from the API, and carried through a refresh from the prior state.
	Local bool
}

// Attribute is the Terraform attribute name.
func (a Attr) Attribute() string {
	if a.TFName != "" {
		return a.TFName
	}
	return a.Name
}

// Presence is how a configuration states one attribute.
type Presence int

// An attribute the configuration leaves out is Absent, one whose value is not
// known until apply is Unknown, and one it sets is Present.
const (
	Absent Presence = iota
	Unknown
	Present
)

// Config is the configuration a Check judges. Names are the API's field names,
// so a secret's write-only argument is read under the secret's own name.
type Config interface {
	String(name string) (string, Presence)
	// List reports whether a list attribute is stated; an empty list is stated.
	List(name string) Presence
}

// Issue is one refusal a Check raises at plan, against the attribute named by
// Attr (an API field name).
type Issue struct {
	Attr    string
	Summary string
	Detail  string
}

// Shape is how a row is addressed.
type Shape int

// Generated rows are addressed by a platform uuid, NaturalKey rows by one
// of their own fields, a Singleton by its resource key, and a ServiceBinding
// by its service plus an optional environment query.
const (
	Generated Shape = iota
	NaturalKey
	Singleton
	ServiceBinding
)

// Spec is one facade resource.
type Spec struct {
	Key         string
	TypeName    string
	ListName    string
	Description string
	Shape       Shape
	IDAttr      string
	Lifecycle   bool
	// Upsert: the write tool describes the row's whole state, so an update
	// sends every field rather than only the changed ones.
	Upsert    bool
	Attrs     []Attr
	ListAttrs []Attr
	// Checks refuse at plan what the platform refuses across attributes (a
	// field required for one provider and refused for the others). A Check
	// judges only what the configuration states: an Unknown value is never
	// refused, since it could still turn out right.
	Checks []func(Config) []Issue
	// ListIDAttr names a list row when the list answers rows without an id.
	ListIDAttr string
	// StartsEmpty: a singleton an organization has no row of until its first
	// write. Read only by internal/fakefacade.
	StartsEmpty bool
	// AlwaysPresent: every organization has this singleton, so a 404 on read means the
	// platform predates the resource. Its data source then fails naming the release needed
	// instead of reading as configured = false with every attribute null.
	AlwaysPresent bool
	// MinPlatform names the platform release the resource first appeared in: a
	// read of an AlwaysPresent singleton, and a Previewed one's 404, say so.
	MinPlatform string
	// FollowsDefault names the WriteOnly list attribute (an API name) that puts
	// settings back on the organization default, and SetHere the answered list of
	// the settings a row holds itself. ModifyPlan turns a name in the first that
	// the second holds into a planned change, and Update sends exactly those.
	FollowsDefault string
	SetHere        string
	// Previewed: the API applies a write only against a preview the caller has
	// read. A PUT with no expected_fingerprint answers 428 and the preview, a stale
	// fingerprint 409 and the fresh one, and a matching one writes. The plan takes
	// the preview (the Local attribute preview_fingerprint holds its fingerprint)
	// and the apply sends that fingerprint. The WriteOnly attribute confirm names
	// the confirmations the preview asks for.
	Previewed bool
	// Confirmations maps each confirm value to the setting whose turning on needs
	// it and the platform's sentence for a configuration that lacks it.
	Confirmations map[string]Confirmation
	// DestroyBody and DestroyWhen: a destroy of a Previewed singleton sends this
	// body, as a previewed write, when the attribute DestroyWhen (an API name) is
	// not null in state; without it the row is left in place.
	DestroyBody map[string]any
	DestroyWhen string
	// DestroyWarning is said when the destroy took effect.
	DestroyWarning string
	// Cursor names the key of the answer that holds the next page's cursor; a spec
	// that sets it is a paged list, read until that key is null. PageSize is the
	// limit asked for each page (the API's list_max) and MaxPages the pages read
	// before the read fails naming the bound.
	Cursor   string
	PageSize int
	MaxPages int
	// QueryAttrs are the optional arguments of the list data source, sent as
	// query values under their API names (String and Bool only).
	QueryAttrs []Attr
	// ExtraAttrs are the computed attributes of the list data source read from
	// the keys beside the rows in the first page's answer.
	ExtraAttrs []Attr
	// DiscoveredAttr is the computed boolean that, true in state, makes a
	// destroy warn that a discovery sweep files the row again.
	DiscoveredAttr string
}

// Confirmation is what a Previewed write needs from the configuration before it
// turns Attr on.
type Confirmation struct {
	Attr     string
	Sentence string
}

// LowerTrim is the platform's normalization of service names.
func LowerTrim(v string) string { return strings.ToLower(strings.TrimSpace(v)) }

// Lower is the platform's normalization of mute patterns.
func Lower(v string) string { return strings.ToLower(v) }
