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
	// hold them in state, so configuration naming one is refused. And because a
	// write replaces the stored object as a whole, the attribute is sent on
	// update only when it changed.
	HiddenKeys []string
	// HiddenSetBy: computed attributes that say hidden keys are stored (a true
	// bool or a non-empty list). While one does, changing the attribute is
	// refused, since the write would silently drop what the app set.
	HiddenSetBy []string
}

// Attribute is the Terraform attribute name.
func (a Attr) Attribute() string {
	if a.TFName != "" {
		return a.TFName
	}
	return a.Name
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
	Attrs       []Attr
	ListAttrs   []Attr
	// ListIDAttr names a list row when the list answers rows without an id.
	ListIDAttr string
	// DiscoveredAttr is the computed boolean that, true in state, makes a
	// destroy warn that a discovery sweep files the row again.
	DiscoveredAttr string
}

// LowerTrim is the platform's normalization of service names.
func LowerTrim(v string) string { return strings.ToLower(strings.TrimSpace(v)) }
