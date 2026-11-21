package engine

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func strMap(t *testing.T, entries map[string]string) types.Map {
	t.Helper()
	elems := map[string]attr.Value{}
	for k, v := range entries {
		elems[k] = types.StringValue(v)
	}
	m, d := types.MapValue(types.StringType, elems)
	if d.HasError() {
		t.Fatal(d)
	}
	return m
}

func stripSelectors(v string) string {
	return strings.NewReplacer("︎", "", "️", "").Replace(v)
}

// An object of strings is a map; null is a null map, {} is an empty map, and a key with a null
// value is a key left out (the platform answers it so for a column with no emoji).
func TestStringMapReadsAnObjectOfStrings(t *testing.T) {
	row := decode(t, `{"m": {"done": "x", "todo": null}, "empty": {}, "none": null}`)
	got, err := fromJSON(StringMap, row["m"])
	if err != nil || !got.Equal(strMap(t, map[string]string{"done": "x"})) {
		t.Fatalf("got %v, %v", got, err)
	}
	empty, err := fromJSON(StringMap, row["empty"])
	if err != nil || empty.IsNull() || len(empty.(types.Map).Elements()) != 0 {
		t.Fatalf("{} must read as an empty map, got %v, %v", empty, err)
	}
	null, err := fromJSON(StringMap, row["none"])
	if err != nil || !null.IsNull() {
		t.Fatalf("null must read as a null map, got %v, %v", null, err)
	}
	if _, err := fromJSON(StringMap, []any{"a"}); err == nil {
		t.Fatal("a list is not a map")
	}
	if _, err := fromJSON(StringMap, map[string]any{"a": float64(1)}); err == nil {
		t.Fatal("a number is not a string")
	}
}

func TestStringMapIsSentAsAnObject(t *testing.T) {
	j, err := toJSON(strMap(t, map[string]string{"done": "x"}))
	if err != nil || !reflect.DeepEqual(j, map[string]any{"done": "x"}) {
		t.Fatalf("got %v, %v", j, err)
	}
	empty, err := toJSON(strMap(t, nil))
	if m, ok := empty.(map[string]any); err != nil || !ok || m == nil || len(m) != 0 {
		t.Fatalf("an empty map must be sent as {}, got %#v, %v", empty, err)
	}
}

// Values equal after the platform's normalization are the same map, whatever the key order; a
// different key, a different value or a different size is not.
func TestKeepAMapTheServerNormalized(t *testing.T) {
	a := Attr{Name: "m", Kind: StringMap, Normalize: stripSelectors}
	held := strMap(t, map[string]string{"todo": "❤", "done": "x"})
	if !keep(a, held, strMap(t, map[string]string{"done": "x", "todo": "❤️"})) {
		t.Fatal("a value equal after normalization must keep the configured spelling")
	}
	for name, remote := range map[string]map[string]string{
		"another value": {"todo": "y", "done": "x"},
		"another key":   {"todo": "❤", "blocked": "x"},
		"a longer map":  {"todo": "❤", "done": "x", "triage": "z"},
		"a shorter map": {"done": "x"},
	} {
		if keep(a, held, strMap(t, remote)) {
			t.Errorf("%s was taken for the same map", name)
		}
	}
	if keep(Attr{Name: "m", Kind: StringMap}, held, strMap(t, map[string]string{"done": "x", "todo": "❤️"})) {
		t.Error("with no normalization the spellings differ")
	}
	if keep(a, held, types.MapNull(types.StringType)) || keep(a, types.MapNull(types.StringType), held) {
		t.Error("a null map is never kept for a known one")
	}
}

func TestMapShapeRefusesWhatThePlatformWouldRefuseOrStoreDifferently(t *testing.T) {
	v := mapShape{keys: []string{"done", "todo"}, maxLen: 8, check: func(s string) string {
		if s == "slack" {
			return "a Slack name"
		}
		return ""
	}}
	nullValue, d := types.MapValue(types.StringType, map[string]attr.Value{"done": types.StringNull()})
	if d.HasError() {
		t.Fatal(d)
	}
	for name, c := range map[string]struct {
		in   types.Map
		want string
	}{
		"fine":          {strMap(t, map[string]string{"done": "ok", "todo": "ok"}), ""},
		"unknown key":   {strMap(t, map[string]string{"review": "ok"}), "not one of the keys"},
		"no value":      {nullValue, "has no value"},
		"too long":      {strMap(t, map[string]string{"done": "123456789"}), "at most 8 characters"},
		"checked value": {strMap(t, map[string]string{"done": "slack"}), "a Slack name"},
		"empty":         {strMap(t, nil), ""},
	} {
		t.Run(name, func(t *testing.T) {
			resp := &validator.MapResponse{}
			v.ValidateMap(context.Background(), validator.MapRequest{Path: path.Root("m"), ConfigValue: c.in}, resp)
			text := ""
			for _, e := range resp.Diagnostics.Errors() {
				text += e.Summary() + " " + e.Detail()
			}
			switch {
			case c.want == "" && resp.Diagnostics.HasError():
				t.Fatalf("refused: %s", text)
			case c.want != "" && !strings.Contains(text, c.want):
				t.Fatalf("the refusal says %q, want it to say %q", text, c.want)
			}
		})
	}
	// An unknown map is never judged.
	resp := &validator.MapResponse{}
	v.ValidateMap(context.Background(), validator.MapRequest{Path: path.Root("m"), ConfigValue: types.MapUnknown(types.StringType)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("an unknown map was refused")
	}
}
