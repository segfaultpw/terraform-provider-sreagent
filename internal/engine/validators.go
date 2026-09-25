package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

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
