package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
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

// codePoints refuses a string outside the published length bounds, counted in
// code points: the platform counts characters, while the framework's own
// length validator counts bytes.
type codePoints struct{ min, max int }

var _ validator.String = codePoints{}

func (v codePoints) Description(context.Context) string {
	switch {
	case v.min > 0 && v.max > 0:
		return fmt.Sprintf("must be %d to %d characters", v.min, v.max)
	case v.min > 0:
		return fmt.Sprintf("must be at least %d characters", v.min)
	}
	return fmt.Sprintf("must be at most %d characters", v.max)
}

func (v codePoints) MarkdownDescription(ctx context.Context) string { return v.Description(ctx) }

func (v codePoints) ValidateString(ctx context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	n := utf8.RuneCountInString(req.ConfigValue.ValueString())
	if (v.min > 0 && n < v.min) || (v.max > 0 && n > v.max) {
		resp.Diagnostics.AddAttributeError(req.Path, "Wrong length", fmt.Sprintf("%s %s, got %d.", req.Path, v.Description(ctx), n))
	}
}

// matchesPattern refuses a string the published pattern does not match.
type matchesPattern struct {
	re  *regexp.Regexp
	raw string
}

var _ validator.String = matchesPattern{}

func (v matchesPattern) Description(context.Context) string {
	return fmt.Sprintf("must match the pattern %s", v.raw)
}

func (v matchesPattern) MarkdownDescription(ctx context.Context) string { return v.Description(ctx) }

func (v matchesPattern) ValidateString(ctx context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if got := req.ConfigValue.ValueString(); !v.re.MatchString(got) {
		resp.Diagnostics.AddAttributeError(req.Path, "Wrong format", fmt.Sprintf("%s %s, got %q.", req.Path, v.Description(ctx), got))
	}
}

// intBounds refuses an integer outside the published minimum and maximum; a nil
// bound is open.
type intBounds struct{ min, max *int64 }

var _ validator.Int64 = intBounds{}

func (v intBounds) Description(context.Context) string {
	switch {
	case v.min != nil && v.max != nil:
		return fmt.Sprintf("must be between %d and %d", *v.min, *v.max)
	case v.min != nil:
		return fmt.Sprintf("must be at least %d", *v.min)
	}
	return fmt.Sprintf("must be at most %d", *v.max)
}

func (v intBounds) MarkdownDescription(ctx context.Context) string { return v.Description(ctx) }

func (v intBounds) ValidateInt64(ctx context.Context, req validator.Int64Request, resp *validator.Int64Response) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	n := req.ConfigValue.ValueInt64()
	if (v.min != nil && n < *v.min) || (v.max != nil && n > *v.max) {
		resp.Diagnostics.AddAttributeError(req.Path, "Out of range", fmt.Sprintf("%s %s, got %d.", req.Path, v.Description(ctx), n))
	}
}

// nullAsString lets the value an attribute reads a null as through a validator
// that would refuse it: the sentinel is outside the published bounds on purpose,
// so no real value can be mistaken for it.
type nullAsString struct {
	sentinel string
	inner    validator.String
}

var _ validator.String = nullAsString{}

func (v nullAsString) Description(ctx context.Context) string {
	return fmt.Sprintf("%s, or %q for none", v.inner.Description(ctx), v.sentinel)
}

func (v nullAsString) MarkdownDescription(ctx context.Context) string { return v.Description(ctx) }

func (v nullAsString) ValidateString(ctx context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if !req.ConfigValue.IsNull() && !req.ConfigValue.IsUnknown() && req.ConfigValue.ValueString() == v.sentinel {
		return
	}
	v.inner.ValidateString(ctx, req, resp)
}

// nullAsInt is nullAsString for an integer.
type nullAsInt struct {
	sentinel int64
	inner    validator.Int64
}

var _ validator.Int64 = nullAsInt{}

func (v nullAsInt) Description(ctx context.Context) string {
	return fmt.Sprintf("%s, or %d for none", v.inner.Description(ctx), v.sentinel)
}

func (v nullAsInt) MarkdownDescription(ctx context.Context) string { return v.Description(ctx) }

func (v nullAsInt) ValidateInt64(ctx context.Context, req validator.Int64Request, resp *validator.Int64Response) {
	if !req.ConfigValue.IsNull() && !req.ConfigValue.IsUnknown() && req.ConfigValue.ValueInt64() == v.sentinel {
		return
	}
	v.inner.ValidateInt64(ctx, req, resp)
}

// rfc3339 refuses a time that is not RFC 3339 with a zone: the platform stores an
// instant, and a time with no zone has none.
type rfc3339 struct{}

var _ validator.String = rfc3339{}

func (rfc3339) Description(context.Context) string {
	return "must be an RFC 3339 time with a time zone, such as 2027-01-01T00:00:00Z"
}

func (v rfc3339) MarkdownDescription(ctx context.Context) string { return v.Description(ctx) }

func (v rfc3339) ValidateString(ctx context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if got := req.ConfigValue.ValueString(); !isInstant(got) {
		resp.Diagnostics.AddAttributeError(req.Path, "Not a time with a zone", fmt.Sprintf("%s %s, got %q.", req.Path, v.Description(ctx), got))
	}
}

// isInstant reports that the text parses as an RFC 3339 time.
func isInstant(v string) bool {
	_, err := time.Parse(time.RFC3339, v)
	return err == nil
}
