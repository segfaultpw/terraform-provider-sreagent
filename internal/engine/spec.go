// Package engine turns a declarative Spec of one configuration API resource
// into a Terraform resource and data source.
package engine

import (
	"strings"
	"time"
)

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
	// StringMap is a map of string to string whose keys are the attribute's MapKeys.
	StringMap
)

// Attr is one field of a resource. Flags mirror x-sreagent-resources in the
// OpenAPI document; internal/contract proves each one against it.
type Attr struct {
	// Name is the API's field name.
	Name string
	// ReadDescription is what the data source and the list rows say of this
	// attribute when the resource's own Description would be false of a read: a
	// null the resource states as 0, a limit only a write meets.
	ReadDescription string
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
	// On a StringList it is applied to each entry, in order: a list whose
	// entries equal the stored ones after it is the stored list, so the
	// configuration's spelling stays and plans no change. On a StringMap it is
	// applied to each value, and the keys must be the same.
	Normalize func(string) string
	// ItemMaxLength is the published items.maxLength of a StringList, or the
	// published maxLength of each value of a StringMap, in code points. Zero means
	// none.
	ItemMaxLength int
	// MapKeys are the keys a StringMap may hold, as its write body publishes them
	// (properties, with additionalProperties false). A key outside them is refused
	// at plan. The map replaces the whole set on a write, so a key left out is
	// removed.
	MapKeys []string
	// ItemCheck refuses a StringList entry (or a StringMap value) the platform would store as a
	// different value than written (it returns the sentence), so the
	// configuration is refused at plan instead of planning a change on every run.
	ItemCheck func(entry string) string
	// OneOf bounds a String to these values and, on a StringList, each item.
	OneOf []string
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
	// KeepUnknown: a StringList bounded by OneOf whose answer may hold entries a
	// later platform release adds (this provider release does not know them). The
	// platform replaces the whole list on a write, so a write that sent only the
	// configuration's entries would switch such an entry back off. A read keeps
	// those entries out of state (the plan shows no change for them) and records
	// them in private state; a write sends the configuration's entries plus them;
	// a plan that names the attribute warns once, naming them. A configuration can
	// still not name one (OneOf refuses it at plan).
	KeepUnknown bool
	// Kept: an Optional list of the configuration that the provider keeps in
	// state and never sends or reads from the API: a value a later operation
	// that has no configuration (a destroy) needs. It is never a write by itself.
	Kept bool
	// Address: the attribute names the row, as the path segment (Spec.IDAttr) or
	// the query value (Spec.QueryAttr) of a PathAndQuery spec. It is Required and
	// a change replaces the row, it is never sent in a body, and state keeps what
	// the configuration says (the platform may answer its canonical spelling, a
	// child by id where the configuration wrote its name). The contract lists it
	// as an id argument of the resource, not as a field.
	Address bool
	// Local: Computed and kept by the provider alone. It is never sent, never read
	// from the API, and carried through a refresh from the prior state.
	Local bool
	// NullAs is a JSON literal the resource reads a null answer as and sends null
	// for: a null that is a real setting on a singleton (no SLA, no channel), whose
	// attributes are Optional and Computed so that leaving one out leaves it as it
	// is. The data source reads the null. The contract publishes the literal as
	// terraform_null_as and the contract test pins it, and a value equal to it is
	// accepted by Min, Max and Pattern even though it lies outside them.
	NullAs string
	// NullAsMeaning finishes the sentence "use <NullAs> ..." in the plan refusal of a
	// value outside the bounds: what the stand-in means for this attribute. Empty
	// reads "for none".
	NullAsMeaning string
	// Min and Max are the published minimum and maximum of an Int, checked at plan.
	Min, Max *int64
	// IntOneOf bounds an Int to these values (the published enum, less the null a NullAs stands for).
	IntOneOf []int64
	// MinLength and MaxLength are the published length bounds of a String, in code
	// points (the platform counts code points; the framework's own validator counts
	// bytes). Zero means none.
	MinLength, MaxLength int
	// Pattern is the published pattern of a String (RE2 syntax, matched anywhere
	// the pattern is not anchored).
	Pattern string
	// MinItems is the published minItems of a StringList. Zero means none.
	MinItems int
	// Unordered: a StringList the platform stores sorted and deduplicated, so a
	// list equal to the stored one as a set is equal: a reordered configuration
	// plans nothing and a refresh keeps the order the configuration wrote.
	Unordered bool
	// Timestamp: an RFC 3339 time with a zone. The platform answers its own form
	// (microseconds, UTC), so equal instants are equal whatever their spelling.
	Timestamp bool
	// Trim: the platform trims the value, so surrounding space is refused at plan
	// instead of reading back changed.
	Trim bool
	// ChangeWarning is said at plan when the planned value differs from state: a
	// change the platform applies to more than the setting itself.
	ChangeWarning string
	// RemovedWarning is said at plan, for a StringList, once for each entry state
	// holds that the plan drops (%s is the entry): a removal with effects beyond
	// the list.
	RemovedWarning string
	// ClearedWarning is said at plan when the plan removes a value state holds (a
	// Clearable attribute going to null): a removal that widens what the row covers.
	ClearedWarning string
	// Default is a JSON literal: the platform's published create default. The
	// attribute is Optional and Computed and takes it when the configuration
	// leaves it out, on a create and on an update that removes a stated value,
	// where every other attribute leaves the stored value as it is.
	Default string
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
	// Bool and Int read a boolean and an integer attribute.
	Bool(name string) (bool, Presence)
	Int(name string) (int64, Presence)
	// Strings reads a list attribute: Present with its entries (an empty list is
	// stated), Unknown when the list or any entry is not known yet. A JSON
	// attribute that holds an array of strings reads the same way.
	Strings(name string) ([]string, Presence)
	// List reports whether a list attribute is stated; an empty list is stated.
	List(name string) Presence
}

// Issue is one refusal a Check raises at plan, against the attribute named by
// Attr (an API field name). A Warning is said and does not stop the plan.
type Issue struct {
	Attr    string
	Summary string
	Detail  string
	Warning bool
}

// Shape is how a row is addressed.
type Shape int

// Generated rows are addressed by a platform uuid, NaturalKey rows by one
// of their own fields, a Singleton by its resource key, a ServiceBinding
// by its service plus an optional environment query, and a PathAndQuery row by
// the value of the spec's IDAttr in the path plus the value of its QueryAttr in
// the query string (a child organization and one of its parent's GitHub
// accounts).
const (
	Generated Shape = iota
	NaturalKey
	Singleton
	ServiceBinding
	PathAndQuery
)

// Spec is one facade resource.
type Spec struct {
	Key         string
	TypeName    string
	ListName    string
	Description string
	// ReadDescription is what the data source says of the resource when the
	// resource's Description speaks of writing it.
	ReadDescription string
	Shape           Shape
	IDAttr          string
	// QueryAttr names the attribute whose value a PathAndQuery row is addressed
	// by in the query string, beside IDAttr in the path. The id Terraform holds
	// is <path value>?<QueryAttr>=<value>, which is also the import format.
	QueryAttr string
	Lifecycle bool
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
	// PlanChecks run in ModifyPlan on every create and update, over the planned
	// values (a value the plan carries from state is planned) and the prior state
	// (nil on a create). They refuse what the platform refuses across attributes
	// when the configuration leaves one of them to state, which a Check, reading
	// the configuration alone, cannot see. An Unknown planned value is never
	// judged.
	PlanChecks []func(plan, prior Config) []Issue
	// PlanFeature names the plan feature a 403 refers to ("the infrastructure
	// suite"). A 403 that is the platform's plan refusal then fails the operation
	// naming the feature and the ways out, never as a row deleted outside Terraform.
	PlanFeature string
	// PlanAdoption: a singleton whose create adopts the settings the organization
	// already has. Terraform shows that as a plain create, so the plan reads the
	// current row (a GET, never a write), judges the block against it as it would
	// judge an update against state, and warns what the apply will change. A read
	// that cannot be made is said, and the plan goes on.
	PlanAdoption bool
	// DestroyNote is added to the warning a destroy of a singleton says when it
	// leaves the settings in place: what stays in force.
	DestroyNote string
	// AdoptOnly: the platform has no door that creates the row (it exists when
	// something in the app made it), so create reads it by its key, judges the
	// block against what it holds as PlanAdoption does for a singleton, and writes
	// the difference through the update door. A row that does not exist is the
	// error the platform's 404 stands for, with AdoptHint added. Every attribute
	// of such a spec is UpdateOnly, which the contract derives from an empty
	// create_fields and a non-empty update_fields.
	AdoptOnly bool
	// AdoptHint finishes the sentence that says there is nothing to adopt.
	AdoptHint string
	// LeaveOnDestroy: the platform has no door that removes the row, so a destroy
	// writes nothing and says this: what stays, and how to change it.
	LeaveOnDestroy string
	// ListIDAttr names a list row when the list answers rows without an id.
	ListIDAttr string
	// StartsEmpty: a singleton an organization has no row of until its first
	// write. Read only by internal/fakefacade.
	StartsEmpty bool
	// AlwaysPresent: every organization has this singleton, so a 404 on read means the
	// platform predates the resource. Its data source then fails naming the release needed
	// instead of reading as configured = false with every attribute null.
	AlwaysPresent bool
	// AbsentWhen: the platform also answers 404 for an organization that is not
	// offered the resource (a feature that is off on the platform, outside its
	// rollout, or not in the plan), so the 404 error and the configured
	// description name that cause beside an older platform. Set with AlwaysPresent.
	AbsentWhen string
	// RefusedWhen: the platform answers 403 with its own sentence, not 404, when
	// the organization's plan lacks the feature. Only the configured description
	// reads it; the refusal reaches the read as the platform wrote it.
	RefusedWhen string
	// MinPlatform names the platform release the resource first appeared in: a
	// read of an AlwaysPresent singleton, and a Previewed one's 404, say so.
	MinPlatform string
	// FollowsDefault names the WriteOnly list attribute (an API name) that puts
	// settings back on the organization default, and SetHere the answered list of
	// the settings a row holds itself. ModifyPlan turns a name in the first that
	// the second holds into a planned change, and Update sends exactly those.
	FollowsDefault string
	SetHere        string
	// FollowsMinPlatform is the platform release that answers SetHere; an older
	// platform ignores FollowsDefault, which the plan says.
	FollowsMinPlatform string
	// RepositoryWide names the settings the platform applies to every row of the
	// repository the row belongs to (the String attribute RepositoryAttr names)
	// whenever one row writes or resets them, so a plan that sends one warns.
	RepositoryWide []string
	RepositoryAttr string
	// Previewed: the API applies a write only against a preview the caller has
	// read. A PUT with no expected_fingerprint answers 428 and the preview, a stale
	// fingerprint 409 and the fresh one, and a matching one writes. The plan takes
	// the preview (the Local attribute preview_fingerprint holds its fingerprint)
	// and the apply sends that fingerprint. The WriteOnly attribute confirm names
	// the confirmations the preview asks for.
	Previewed bool
	// ProbeFingerprint is the fingerprint a Previewed collection row sends to ask
	// for a preview without a write. The platform writes a change that takes
	// nothing away at once, so a PUT with no fingerprint cannot be a plan's
	// question; it answers a fingerprint no preview carries as it answers a stale
	// one, with the fresh preview, and writes nothing. A 200 to the probe means the
	// platform has nothing to write.
	ProbeFingerprint string
	// AccessPreview: the preview this spec's 428 and 409 carry is the child access
	// preview (before, after, what the child stops and starts reaching, what the
	// change holds), not the repository defaults' one.
	AccessPreview bool
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
	// View: a list the API answers with no door for one row (no id_args), which a
	// data source reads whole. A paged list is a view too.
	View bool
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

// RFC3339 is the platform's form of a time: the instant in UTC. A value that is
// not RFC 3339 is returned as it is.
func RFC3339(v string) string {
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return v
	}
	return t.UTC().Format(time.RFC3339Nano)
}

// normalizer is the form the platform stores a String in, if it normalizes it.
func (a Attr) normalizer() func(string) string {
	switch {
	case a.Normalize != nil:
		return a.Normalize
	case a.Timestamp:
		return RFC3339
	}
	return nil
}
