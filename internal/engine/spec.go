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
	Name        string
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
	// Computed: answered, never written.
	Computed bool
	// Secret: exposed as <name>_wo, <name>_wo_version and <name>_set.
	Secret bool
	// NotRead: an action argument the server never answers; state keeps config.
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
}

// LowerTrim is the platform's normalization of service names.
func LowerTrim(v string) string { return strings.ToLower(strings.TrimSpace(v)) }
