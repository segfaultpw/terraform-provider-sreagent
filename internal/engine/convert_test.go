package engine

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func decode(t *testing.T, s string) map[string]any {
	t.Helper()
	d := json.NewDecoder(strings.NewReader(s))
	d.UseNumber()
	var m map[string]any
	if err := d.Decode(&m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestFromJSONKeepsIntegersExact(t *testing.T) {
	row := decode(t, `{"n": 9007199254740993}`)
	v, err := fromJSON(Int, row["n"])
	if err != nil || v.(types.Int64).ValueInt64() != 9007199254740993 {
		t.Fatalf("got %v, %v", v, err)
	}
}

func TestJSONRoundTripIsKeyOrderStable(t *testing.T) {
	row := decode(t, `{"c": {"b": 1, "a": [2, {"y": 1, "x": 2}]}}`)
	v, err := fromJSON(JSON, row["c"])
	if err != nil {
		t.Fatal(err)
	}
	if v.(jsontypes.Normalized).ValueString() != `{"a":[2,{"x":2,"y":1}],"b":1}` {
		t.Fatalf("got %s", v.(jsontypes.Normalized).ValueString())
	}
}

func TestKeepConfiguredWhenTheServerNormalized(t *testing.T) {
	a := Attr{Name: "service", Kind: String, Normalize: LowerTrim}
	if !keep(a, types.StringValue(" Checkout"), types.StringValue("checkout")) {
		t.Fatal("a value equal after normalization must keep the configured form")
	}
	if keep(a, types.StringValue("checkout"), types.StringValue("payments")) {
		t.Fatal("a different value must win")
	}
}

func TestKeepConfiguredFloatWithinRounding(t *testing.T) {
	a := Attr{Name: "target", Kind: Float}
	if !keep(a, types.Float64Value(99.9), types.Float64Value(99.90000000000001)) {
		t.Fatal("float noise must not be drift")
	}
}

func TestKeepConfiguredJSONRegardlessOfSpacing(t *testing.T) {
	a := Attr{Name: "config", Kind: JSON}
	if !keep(a, jsontypes.NewNormalizedValue(`{"a": 1, "b": 2}`), jsontypes.NewNormalizedValue(`{"b":2,"a":1}`)) {
		t.Fatal("equal JSON must keep the configured text")
	}
}

func TestToJSONList(t *testing.T) {
	l, _ := types.ListValueFrom(t.Context(), types.StringType, []string{"a", "b"})
	v, err := toJSON(l)
	if err != nil || strings.Join(v.([]string), ",") != "a,b" {
		t.Fatalf("got %v %v", v, err)
	}
}

func TestAnEmptyObjectIsAnUnsetClearableObject(t *testing.T) {
	a := Attr{Name: "default_severity_mapping", Kind: JSON, Clearable: true}
	if !keep(a, jsontypes.NewNormalizedNull(), jsontypes.NewNormalizedValue(`{}`)) {
		t.Fatal("{} answered for an unset Clearable object must keep null")
	}
	if keep(a, jsontypes.NewNormalizedNull(), jsontypes.NewNormalizedValue(`{"critical":"P1"}`)) {
		t.Fatal("a real mapping must not read as unset")
	}
	if keep(Attr{Name: "config", Kind: JSON}, jsontypes.NewNormalizedNull(), jsontypes.NewNormalizedValue(`{}`)) {
		t.Fatal("only a Clearable object treats {} as unset")
	}
}

func TestTheNullDefaultIsAnUnsetClearableValue(t *testing.T) {
	a := Attr{Name: "cooldown_minutes", Kind: Int, Clearable: true, NullMeans: "30"}
	if !keep(a, types.Int64Null(), types.Int64Value(30)) {
		t.Fatal("the default a null means must keep an unset configuration null")
	}
	if keep(a, types.Int64Null(), types.Int64Value(45)) {
		t.Fatal("a value set in the app is drift")
	}
}
