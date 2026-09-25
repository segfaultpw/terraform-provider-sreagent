package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// valueSource is what tfsdk.Plan, tfsdk.State and tfsdk.Config share.
type valueSource interface {
	GetAttribute(ctx context.Context, p path.Path, target any) diag.Diagnostics
}

func decodeRow(raw json.RawMessage) (map[string]any, error) {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var row map[string]any
	if err := d.Decode(&row); err != nil {
		return nil, fmt.Errorf("the API answered a row that is not an object: %w", err)
	}
	return row, nil
}

func getValue(ctx context.Context, src valueSource, a Attr) (attr.Value, diag.Diagnostics) {
	p := path.Root(a.Name)
	switch a.Kind {
	case String:
		var v types.String
		return v, src.GetAttribute(ctx, p, &v)
	case Int:
		var v types.Int64
		return v, src.GetAttribute(ctx, p, &v)
	case Float:
		var v types.Float64
		return v, src.GetAttribute(ctx, p, &v)
	case Bool:
		var v types.Bool
		return v, src.GetAttribute(ctx, p, &v)
	case StringList:
		var v types.List
		return v, src.GetAttribute(ctx, p, &v)
	case JSON:
		var v jsontypes.Normalized
		return v, src.GetAttribute(ctx, p, &v)
	}
	return nil, nil
}

func toJSON(v attr.Value) (any, error) {
	switch t := v.(type) {
	case types.String:
		return t.ValueString(), nil
	case jsontypes.Normalized:
		var out any
		if err := json.Unmarshal([]byte(t.ValueString()), &out); err != nil {
			return nil, fmt.Errorf("not valid JSON: %w", err)
		}
		return out, nil
	case types.Int64:
		return t.ValueInt64(), nil
	case types.Float64:
		return t.ValueFloat64(), nil
	case types.Bool:
		return t.ValueBool(), nil
	case types.List:
		out := make([]string, 0, len(t.Elements()))
		for _, e := range t.Elements() {
			s, ok := e.(types.String)
			if !ok {
				return nil, fmt.Errorf("list element %s is not a string", e)
			}
			out = append(out, s.ValueString())
		}
		return out, nil
	}
	return nil, fmt.Errorf("unsupported value %T", v)
}

func fromJSON(k Kind, raw any) (attr.Value, error) {
	switch k {
	case String:
		if raw == nil {
			return types.StringNull(), nil
		}
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("want a string, got %T", raw)
		}
		return types.StringValue(s), nil
	case Int:
		if raw == nil {
			return types.Int64Null(), nil
		}
		n, ok := raw.(json.Number)
		if !ok {
			return nil, fmt.Errorf("want an integer, got %T", raw)
		}
		i, err := n.Int64()
		if err != nil {
			return nil, err
		}
		return types.Int64Value(i), nil
	case Float:
		if raw == nil {
			return types.Float64Null(), nil
		}
		n, ok := raw.(json.Number)
		if !ok {
			return nil, fmt.Errorf("want a number, got %T", raw)
		}
		f, err := n.Float64()
		if err != nil {
			return nil, err
		}
		return types.Float64Value(f), nil
	case Bool:
		if raw == nil {
			return types.BoolNull(), nil
		}
		b, ok := raw.(bool)
		if !ok {
			return nil, fmt.Errorf("want a boolean, got %T", raw)
		}
		return types.BoolValue(b), nil
	case StringList:
		if raw == nil {
			return types.ListNull(types.StringType), nil
		}
		items, ok := raw.([]any)
		if !ok {
			return nil, fmt.Errorf("want a list, got %T", raw)
		}
		elems := make([]attr.Value, 0, len(items))
		for _, it := range items {
			s, ok := it.(string)
			if !ok {
				return nil, fmt.Errorf("want a list of strings, got %T", it)
			}
			elems = append(elems, types.StringValue(s))
		}
		l, d := types.ListValue(types.StringType, elems)
		if d.HasError() {
			return nil, fmt.Errorf("building a list: %v", d)
		}
		return l, nil
	case JSON:
		if raw == nil {
			return jsontypes.NewNormalizedNull(), nil
		}
		b, err := json.Marshal(raw)
		if err != nil {
			return nil, err
		}
		return jsontypes.NewNormalizedValue(string(b)), nil
	}
	return nil, fmt.Errorf("unsupported kind %d", k)
}

// keep reports whether the configured value should stay in state although
// the server answered a different spelling of the same value.
func keep(a Attr, before, remote attr.Value) bool {
	// The platform defaults a Clearable object to {} on create and stores null
	// when one is cleared; both mean none, so an unset configuration stays null.
	if a.Kind == JSON && a.Clearable && before != nil && before.IsNull() && isEmptyObject(remote) {
		return true
	}
	if a.NullMeans != "" && a.Clearable && before != nil && before.IsNull() && remote != nil && !remote.IsNull() {
		d := json.NewDecoder(strings.NewReader(a.NullMeans))
		d.UseNumber()
		var raw any
		if d.Decode(&raw) == nil {
			if def, err := fromJSON(a.Kind, raw); err == nil && def.Equal(remote) {
				return true
			}
		}
	}
	if before == nil || before.IsNull() || before.IsUnknown() || remote == nil || remote.IsNull() {
		return false
	}
	if before.Equal(remote) {
		return true
	}
	switch a.Kind {
	case String:
		if a.Normalize == nil {
			return false
		}
		return a.Normalize(before.(types.String).ValueString()) == a.Normalize(remote.(types.String).ValueString())
	case Float:
		x, y := before.(types.Float64).ValueFloat64(), remote.(types.Float64).ValueFloat64()
		return math.Abs(x-y) <= 1e-9*math.Max(1, math.Abs(x))
	case JSON:
		var x, y any
		if json.Unmarshal([]byte(before.(jsontypes.Normalized).ValueString()), &x) != nil ||
			json.Unmarshal([]byte(remote.(jsontypes.Normalized).ValueString()), &y) != nil {
			return false
		}
		return reflect.DeepEqual(x, y)
	}
	return false
}

func isEmptyObject(v attr.Value) bool {
	n, ok := v.(jsontypes.Normalized)
	if !ok || n.IsNull() || n.IsUnknown() {
		return false
	}
	var obj map[string]any
	return json.Unmarshal([]byte(n.ValueString()), &obj) == nil && obj != nil && len(obj) == 0
}
