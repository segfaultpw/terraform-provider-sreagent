package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

// refuseKeys refuses a JSON object that names any of keys at its top level.
type refuseKeys struct{ keys []string }

var _ validator.String = refuseKeys{}

func (v refuseKeys) Description(context.Context) string {
	return fmt.Sprintf("must not name %s", strings.Join(v.keys, " or "))
}

func (v refuseKeys) MarkdownDescription(ctx context.Context) string { return v.Description(ctx) }

func (v refuseKeys) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	var obj map[string]any
	// Anything but an object is left to the platform, which refuses it in its own words.
	if json.Unmarshal([]byte(req.ConfigValue.ValueString()), &obj) != nil {
		return
	}
	for _, k := range v.keys {
		if _, ok := obj[k]; ok {
			resp.Diagnostics.AddAttributeError(req.Path, "Not managed by Terraform",
				fmt.Sprintf("%s.%s can carry a credential, so the platform never answers it and Terraform could only keep it in state. Set it in the app instead.", req.Path, k))
		}
	}
}

// fieldName refuses what the platform refuses in a JSON field name it keeps as
// written: a blank one, one over 128 code points, one with a control character.
// Surrounding whitespace is not refused, because the platform does not strip it.
type fieldName struct{}

var _ validator.String = fieldName{}

func (fieldName) Description(context.Context) string {
	return "must be 1 to 128 characters, not blank, with no control character"
}

func (v fieldName) MarkdownDescription(ctx context.Context) string { return v.Description(ctx) }

func (fieldName) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	v := req.ConfigValue.ValueString()
	switch {
	case strings.TrimSpace(v) == "":
		resp.Diagnostics.AddAttributeError(req.Path, "Blank field name", fmt.Sprintf("%q is blank and names no field; write a JSON field such as \"user.id\".", v))
	case utf8.RuneCountInString(v) > fieldNameMax:
		resp.Diagnostics.AddAttributeError(req.Path, "Field name too long", fmt.Sprintf("A field name is 1 to %d characters, this one has %d.", fieldNameMax, utf8.RuneCountInString(v)))
	case strings.ContainsFunc(v, func(r rune) bool { return r < 0x20 || r == 0x7f }):
		resp.Diagnostics.AddAttributeError(req.Path, "Control character", fmt.Sprintf("%q holds a control character, which the platform refuses in a field name.", v))
	}
}

// fieldNameMax is the platform's bound, in code points.
const fieldNameMax = 128

// trimmed refuses a string with surrounding whitespace, which the platform
// would strip.
type trimmed struct{}

var _ validator.String = trimmed{}

func (trimmed) Description(context.Context) string { return "must not start or end with whitespace" }

func (v trimmed) MarkdownDescription(ctx context.Context) string { return v.Description(ctx) }

func (trimmed) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if v := req.ConfigValue.ValueString(); v != strings.TrimSpace(v) {
		resp.Diagnostics.AddAttributeError(req.Path, "Surrounding whitespace", fmt.Sprintf("%q starts or ends with whitespace, which the platform strips; write %q.", v, strings.TrimSpace(v)))
	}
}
