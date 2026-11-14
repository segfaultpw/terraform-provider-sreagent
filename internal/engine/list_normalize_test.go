package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func selectorless(v string) string { return strings.NewReplacer("\ufe0e", "", "\ufe0f", "").Replace(v) }

func tfList(items ...string) types.List {
	elems := make([]string, len(items))
	copy(elems, items)
	vals := make([]types.String, len(elems))
	for i, e := range elems {
		vals[i] = types.StringValue(e)
	}
	l, _ := types.ListValueFrom(context.Background(), types.StringType, vals)
	return l
}

func TestAListWhoseEntriesNormalizeEqualIsTheStoredList(t *testing.T) {
	a := Attr{Name: "row", Kind: StringList, Normalize: selectorless}
	cases := []struct {
		name         string
		before, want types.List
		same         bool
	}{
		{"the same entries", tfList("❤\ufe0f", "x"), tfList("❤\ufe0f", "x"), true},
		{"a missing selector", tfList("❤", "x"), tfList("❤\ufe0f", "x"), true},
		{"an extra selector", tfList("❤\ufe0f"), tfList("❤"), true},
		{"another order is another list", tfList("a", "b"), tfList("b", "a"), false},
		{"another length", tfList("a"), tfList("a", "b"), false},
		{"another emoji", tfList("❤"), tfList("❣\ufe0f"), false},
		{"a ZWJ sequence keeps its joiners", tfList("\U0001F468\u200d\U0001F469"), tfList("\U0001F468\U0001F469"), false},
		{"a skin tone is part of the emoji", tfList("\U0001F44D"), tfList("\U0001F44D\U0001F3FD"), false},
		{"null is no list", types.ListNull(types.StringType), tfList("a"), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := keep(a, c.before, c.want); got != c.same {
				t.Fatalf("keep = %v, want %v", got, c.same)
			}
		})
	}
	if keep(Attr{Name: "plain", Kind: StringList}, tfList("❤"), tfList("❤\ufe0f")) {
		t.Fatal("a list with no Normalize must not be read as equal after one")
	}
}

func TestUniqueAfterNormalizationRefusesASpellingRepeat(t *testing.T) {
	v := uniqueAfter{normalize: selectorless}
	check := func(list types.List) bool {
		var resp validator.ListResponse
		v.ValidateList(context.Background(), validator.ListRequest{Path: path.Root("row"), ConfigValue: list}, &resp)
		return resp.Diagnostics.HasError()
	}
	if !check(tfList("❤", "❤\ufe0f")) {
		t.Error("two spellings of one emoji were accepted")
	}
	if check(tfList("❤", "\U0001F680")) {
		t.Error("two emoji were refused")
	}
	if check(types.ListNull(types.StringType)) || check(types.ListUnknown(types.StringType)) {
		t.Error("a null or unknown list was judged")
	}
}

func TestAnEntryIsCountedInCodePoints(t *testing.T) {
	v := itemLength{max: 7}
	check := func(s string) bool {
		var resp validator.StringResponse
		v.ValidateString(context.Background(), validator.StringRequest{Path: path.Root("row"), ConfigValue: types.StringValue(s)}, &resp)
		return resp.Diagnostics.HasError()
	}
	// The family emoji is seven code points and 25 bytes: the bound is the platform's, in characters.
	if check("\U0001F468\u200d\U0001F469\u200d\U0001F467\u200d\U0001F466") {
		t.Error("seven code points were refused by a bound of seven")
	}
	if !check("\U0001F468\u200d\U0001F469\u200d\U0001F467\u200d\U0001F466\ufe0f") {
		t.Error("eight code points were accepted by a bound of seven")
	}
}
