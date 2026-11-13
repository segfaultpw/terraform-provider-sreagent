package contract

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"reflect"
	"regexp"
	"slices"
	"testing"
	"unicode/utf8"

	"github.com/segfaultpw/terraform-provider-sreagent/internal/engine"
	"github.com/segfaultpw/terraform-provider-sreagent/internal/specs"
)

func doc(t *testing.T) *Doc {
	t.Helper()
	d, err := Load("openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func typeOf(schema map[string]any) (string, bool) {
	switch t := schema["type"].(type) {
	case string:
		return t, false
	case []any:
		nullable := slices.Contains(t, any("null"))
		for _, x := range t {
			if x != "null" {
				return fmt.Sprint(x), nullable
			}
		}
	}
	return "", false
}

func kindFor(schema map[string]any) engine.Kind {
	t, _ := typeOf(schema)
	switch t {
	case "string":
		return engine.String
	case "integer":
		return engine.Int
	case "number":
		return engine.Float
	case "boolean":
		return engine.Bool
	case "array":
		if items, ok := schema["items"].(map[string]any); ok && items["type"] == "string" {
			return engine.StringList
		}
	}
	return engine.JSON
}

// sameJSON reports whether a spec's NullMeans literal and the contract's
// published value decode to the same JSON value.
func sameJSON(literal string, published any) bool {
	var want any
	if err := json.Unmarshal([]byte(literal), &want); err != nil {
		return false
	}
	return reflect.DeepEqual(want, published)
}

// specProblems is every way one spec disagrees with the contract document.
func specProblems(d *Doc, s engine.Spec) []string {
	var p []string
	add := func(format string, args ...any) { p = append(p, fmt.Sprintf(format, args...)) }
	res, ok := d.Resources[s.Key]
	if !ok {
		return append(p, fmt.Sprintf("%s is not in the API contract", s.Key))
	}
	if want := res.TerraformType != ""; s.Lifecycle != want {
		return append(p, fmt.Sprintf("Lifecycle %v, but the contract's terraform_type is %q", s.Lifecycle, res.TerraformType))
	}
	if s.Lifecycle && s.TypeName != res.TerraformType {
		return append(p, fmt.Sprintf("TypeName %q, contract terraform_type %q", s.TypeName, res.TerraformType))
	}
	if (s.Shape == engine.Singleton) != res.Singleton {
		return append(p, "singleton mismatch")
	}
	if s.Lifecycle && s.Upsert != res.Upsert {
		return append(p, fmt.Sprintf("Upsert %v, contract upsert %v", s.Upsert, res.Upsert))
	}
	row := d.Components.Schemas[s.Key+"_row"].Properties
	post := d.Paths["/"+s.Key]["post"].RequestBody.Content["application/json"].Schema
	// Clearable is read from the update body: a row schema allows null wherever
	// a read can answer null, but only a field the PUT body declares nullable
	// accepts the null an unset Clearable attribute sends.
	put := d.Paths["/"+s.Key+"/{id}"]["put"].RequestBody.Content["application/json"].Schema
	// A singleton has no POST; its enums live on its PUT body.
	enums := post
	if s.Shape == engine.Singleton {
		enums = d.Paths["/"+s.Key]["put"].RequestBody.Content["application/json"].Schema
		// A singleton has no POST and no /{id}: its create and its update are
		// the one PUT.
		post, put = enums, enums
	}
	listBounds := put.Properties
	byName := map[string]engine.Attr{}
	for _, a := range s.Attrs {
		byName[a.Name] = a
	}
	for _, f := range res.Fields {
		a, ok := byName[f]
		if !ok {
			add("field %s is missing from the spec", f)
			continue
		}
		// The contract names the writable fields no API key can write, so
		// the spec's Computed set is pinned to it rather than kept by hand.
		if a.Computed || a.Secret {
			if s.Lifecycle && !slices.Contains(res.TerraformReadOnly, f) {
				add("field %s is writable in the contract but %s in the spec", f, map[bool]string{true: "computed", false: "secret"}[a.Computed])
			}
		} else if slices.Contains(res.TerraformReadOnly, f) {
			add("field %s is terraform_read_only in the contract but writable in the spec", f)
		}
		// A Clearable field's null meaning is published too, so the
		// provider's NullMeans is held to the platform's, not remembered.
		if mean, ok := res.NullMeans[f]; ok {
			switch {
			case a.NullMeans == "":
				add("field %s has null meaning %v in the contract but none in the spec", f, mean)
			case !sameJSON(a.NullMeans, mean):
				add("field %s null means %q in the spec, %v in the contract", f, a.NullMeans, mean)
			}
		} else if a.NullMeans != "" {
			add("field %s has null meaning %q in the spec but none in the contract", f, a.NullMeans)
		}
		if got, want := a.Kind, kindFor(row[f]); got != want {
			add("field %s kind %d, contract %d", f, got, want)
		}
		if s.Lifecycle && s.Shape != engine.Singleton {
			if a.Required != slices.Contains(post.Required, f) {
				add("field %s Required %v, contract required %v", f, a.Required, post.Required)
			}
			createOnly := slices.Contains(res.CreateFields, f) && !slices.Contains(res.UpdateFields, f)
			if len(res.UpdateFields) == 0 {
				createOnly = slices.Contains(res.CreateFields, f)
			}
			if a.CreateOnly != createOnly {
				add("field %s CreateOnly %v, contract %v", f, a.CreateOnly, createOnly)
			}
			updateOnly := !slices.Contains(res.CreateFields, f) && slices.Contains(res.UpdateFields, f)
			if a.UpdateOnly != updateOnly {
				add("field %s UpdateOnly %v, contract %v", f, a.UpdateOnly, updateOnly)
			}
			_, nullable := typeOf(put.Properties[f])
			if a.Clearable != nullable {
				add("field %s Clearable %v, update body nullable %v", f, a.Clearable, nullable)
			}
		}
		if !s.Lifecycle {
			continue
		}
		published := post.Properties
		if _, ok := published[f]; !ok {
			published = put.Properties
		}
		checkKeywords(s.Key, a, f, published[f], add)
		checkNullAs(a, f, res, put.Properties[f], add)
	}
	for _, f := range res.SecretFields {
		if a, ok := byName[f]; !ok || !a.Secret {
			add("secret %s must be a Secret attribute", f)
		}
	}
	for _, a := range s.Attrs {
		switch {
		case a.Local, a.Kept:
		case a.WriteOnly:
			checkWriteOnly(a, res, post.Properties, put.Properties, add)
		case a.NotRead:
			if !slices.Contains(res.ActionArgs, a.Name) {
				add("NotRead %s is not an action argument", a.Name)
			}
		case len(a.Complement) > 0:
			if _, ok := res.ComputedFields[a.Name]; !ok {
				add("complement %s is derived from %s, which the API does not publish as a computed field", a.Attribute(), a.Name)
			}
			for _, name := range a.Complement {
				if !slices.Contains(res.Fields, name) {
					add("complement %s names %s, which is not a field of %s", a.Attribute(), name, s.Key)
				}
			}
		case a.Computed:
			if _, ok := res.ComputedFields[a.Name]; !ok && !slices.Contains(res.Fields, a.Name) {
				add("computed %s is not answered by the API", a.Name)
			}
		case a.Secret:
		default:
			if !slices.Contains(res.Fields, a.Name) {
				add("attribute %s is not a field of %s", a.Name, s.Key)
			}
		}
	}
	checkFollows(s, res, add)
	checkPaged(d, s, res, add)
	checkPreviewed(d, s, res, add)
	checkIgnored(s, enums.Properties, add)
	checkSecrets(d, s, res, add)
	checkAddressing(s, res, add)
	checkListBounds(s, listBounds, add)
	checkReadOnlySingleton(s, res, add)
	return p
}

// checkWriteOnly holds a write-only list to the API: an action argument whose
// create and update bodies declare an array of the attribute's OneOf values.
func checkWriteOnly(a engine.Attr, res Resource, post, put map[string]map[string]any, add func(string, ...any)) {
	if !slices.Contains(res.ActionArgs, a.Name) {
		add("write-only %s is not an action argument", a.Name)
	}
	for method, props := range map[string]map[string]map[string]any{"POST": {a.Name: post[a.Name]}, "PUT": {a.Name: put[a.Name]}} {
		body := props[a.Name]
		if t, _ := typeOf(body); t != "array" {
			add("write-only %s is not an array in the %s body", a.Name, method)
			continue
		}
		items, _ := body["items"].(map[string]any)
		enum, _ := items["enum"].([]any)
		want := make([]string, 0, len(enum))
		for _, e := range enum {
			want = append(want, fmt.Sprint(e))
		}
		if !slices.Equal(a.OneOf, want) {
			add("write-only %s OneOf %v, %s body enum %v", a.Name, a.OneOf, method, want)
		}
	}
}

// checkPaged holds a paged list data source to the API: the page size is the
// list's cap, every query argument is a parameter of the GET, the cursor and the
// extras are keys of its 200 answer, and every computed field is read.
func checkPaged(d *Doc, s engine.Spec, res Resource, add func(string, ...any)) {
	if s.Cursor == "" {
		return
	}
	get := d.Paths["/"+s.Key]["get"]
	if res.ListMax == nil || *res.ListMax != s.PageSize {
		add("PageSize %d, contract list_max %v", s.PageSize, res.ListMax)
	}
	have := map[string]bool{}
	for _, p := range get.Parameters {
		have[p.Name] = true
	}
	for _, q := range s.QueryAttrs {
		if !have[q.Name] {
			add("query argument %s is not a parameter of GET /%s", q.Name, s.Key)
		}
	}
	if !have["cursor"] || !have["limit"] {
		add("GET /%s no longer takes cursor and limit", s.Key)
	}
	answer := get.Responses["200"].Content["application/json"].Schema.Properties
	if _, ok := answer[s.Cursor]; !ok {
		add("the 200 answer of GET /%s has no %s", s.Key, s.Cursor)
	}
	for _, x := range s.ExtraAttrs {
		if _, ok := answer[x.Name]; !ok {
			add("extra %s is not a key of the 200 answer of GET /%s", x.Name, s.Key)
		}
	}
	read := map[string]bool{}
	for _, a := range s.Attrs {
		read[a.Name] = true
	}
	for name := range res.ComputedFields {
		if !read[name] {
			add("computed field %s is published by the contract but has no attribute", name)
		}
	}
}

// checkPreviewed holds a spec whose writes the API applies only against a
// preview to what the contract publishes: the fingerprint argument, the 428 and
// the 409 of the singleton's PUT, the arguments a destroy sends, and the
// confirmations the spec asks the configuration for.
func checkPreviewed(d *Doc, s engine.Spec, res Resource, add func(string, ...any)) {
	if !s.Previewed {
		return
	}
	if s.Shape != engine.Singleton {
		add("a previewed spec must be a singleton")
		return
	}
	if !slices.Contains(res.ActionArgs, "expected_fingerprint") {
		add("previewed %s does not publish the expected_fingerprint argument", s.Key)
	}
	put := d.Paths["/"+s.Key]["put"]
	for _, status := range []string{"428", "409"} {
		if _, ok := put.Responses[status]; !ok {
			add("the PUT of %s no longer documents a %s", s.Key, status)
		}
	}
	for arg := range s.DestroyBody {
		if !slices.Contains(res.ActionArgs, arg) {
			add("the destroy body sends %s, which is not an action argument", arg)
		}
	}
	if s.DestroyBody != nil {
		if _, ok := res.ComputedFields[s.DestroyWhen]; !ok {
			add("the destroy follows %s, which the API does not publish as a computed field", s.DestroyWhen)
		}
	}
	checkPreviewShape(put.Responses, add)
	body := put.RequestBody.Content["application/json"].Schema.Properties
	for name, c := range s.Confirmations {
		items, _ := body["confirm"]["items"].(map[string]any)
		if enum, _ := items["enum"].([]any); !slices.Contains(enum, any(name)) {
			add("confirmation %s is not in the PUT body's confirm enum", name)
		}
		if !slices.Contains(res.Fields, c.Attr) {
			add("confirmation %s names %s, which is not a field of %s", name, c.Attr, s.Key)
		}
	}
}

// previewFixture is the preview the platform emits, copied from its code: the
// keys the provider reads must all be there (the contract itself publishes
// preview as a bare object).
func previewFixture() (map[string]any, error) {
	raw, err := os.ReadFile("testdata/repository_defaults_preview.json")
	if err != nil {
		return nil, err
	}
	var doc struct {
		Preview map[string]any `json:"preview"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	return doc.Preview, nil
}

// checkPreviewShape holds the keys of preview.data the provider reads to what
// the platform publishes: the fixture of its answer, and the 428 and 409
// schemas once they list the preview's properties. A rename of one of those
// keys would otherwise read as a preview that moves nothing.
func checkPreviewShape(responses map[string]struct {
	Content map[string]struct {
		Schema struct {
			Properties map[string]map[string]any `json:"properties"`
		} `json:"schema"`
	} `json:"content"`
}, add func(string, ...any)) {
	fixture, err := previewFixture()
	switch {
	case err != nil:
		add("the preview fixture cannot be read: %v", err)
	case engine.CheckPreview(fixture) != nil:
		add("the preview fixture is not a preview the provider can read: %v", engine.CheckPreview(fixture))
	default:
		if kids, _ := fixture["children"].([]any); len(kids) == 0 {
			add("the preview fixture has no following child, so the child keys are not pinned")
		}
		if changes, _ := fixture["changes"].(map[string]any); len(changes) == 0 {
			add("the preview fixture has no change, so the change keys are not pinned")
		}
	}
	for _, status := range []string{"428", "409"} {
		preview := responses[status].Content["application/json"].Schema.Properties["preview"]
		for _, problem := range previewSchemaProblems(preview) {
			add("the %s answer %s", status, problem)
		}
	}
}

// previewSchemaProblems holds a published preview schema to the wire shape: the
// keys the provider reads are under preview.data (the body is {"preview":
// {"data": {...}}}), not directly under preview. A schema that publishes no
// properties (today's bare object, or a bare data object) is accepted; one that
// publishes them must publish them at that path and list every key.
func previewSchemaProblems(preview map[string]any) []string {
	props, published := preview["properties"].(map[string]any)
	if !published {
		return nil
	}
	data, ok := props["data"].(map[string]any)
	if !ok {
		return []string{"publishes preview properties without data, but the provider reads the preview under preview.data"}
	}
	dataProps, published := data["properties"].(map[string]any)
	if !published {
		return nil
	}
	var out []string
	for _, key := range engine.PreviewKeys() {
		if _, ok := dataProps[key]; !ok {
			out = append(out, fmt.Sprintf("publishes preview.data without %s, which the provider reads", key))
		}
	}
	return out
}

// checkIgnored holds each list entry the provider ignores to the items the
// write body publishes: an entry the platform stopped listing would be a dead
// rule, and one it never listed a typo.
func checkIgnored(s engine.Spec, body map[string]map[string]any, add func(string, ...any)) {
	for _, a := range s.Attrs {
		for _, v := range a.IgnoredValues {
			items, _ := body[a.Name]["items"].(map[string]any)
			if enum, _ := items["enum"].([]any); !slices.Contains(enum, any(v)) {
				add("%s ignores %q, which the update body does not publish", a.Name, v)
			}
		}
	}
}

// checkFollows holds the reset machinery to the API: the named write-only list
// exists, and the list of held settings is a published computed field.
func checkFollows(s engine.Spec, res Resource, add func(string, ...any)) {
	if s.FollowsDefault == "" && s.SetHere == "" {
		return
	}
	var writeOnly bool
	for _, a := range s.Attrs {
		if a.Name == s.FollowsDefault && a.WriteOnly {
			writeOnly = true
		}
	}
	if !writeOnly {
		add("FollowsDefault %q is not a write-only attribute", s.FollowsDefault)
	}
	if _, ok := res.ComputedFields[s.SetHere]; !ok {
		add("SetHere %q is not a computed field the API publishes", s.SetHere)
	}
}

// checkListBounds holds each list's MaxItems to the maxItems the update body
// publishes (a singleton's only body is its PUT), so a bound the platform
// changes fails here rather than at apply.
func checkListBounds(s engine.Spec, props map[string]map[string]any, add func(string, ...any)) {
	if !s.Lifecycle {
		return
	}
	for _, a := range s.Attrs {
		if a.Kind != engine.StringList {
			continue
		}
		published := 0
		if n, ok := props[a.Name]["maxItems"].(float64); ok {
			published = int(n)
		}
		if a.MaxItems != published {
			add("field %s MaxItems %d, contract maxItems %d", a.Name, a.MaxItems, published)
		}
	}
}

// unmodelled are the keywords a write body published before the engine modelled
// them, on resources the provider already released. Each is a plan-time
// strengthening nobody asked for (a stricter validator on a resource whose
// behaviour is frozen), so the keyword is named here instead of failing the
// contract test; a spec that models one is compared like any other, and a
// keyword the engine does not understand and this table does not name fails.
// The three organization_settings limits that are not listed (api_key_max_days,
// mfa_grace_days, session_idle_minutes) are read-only attributes, which the
// keyword check skips.
var unmodelled = map[string][]string{
	"change_notifications.event_types":                    {"items.enum"},
	"notification_settings.muted_events":                  {"items.enum"},
	"organization_settings.inheritance_stopped":           {"items.enum"},
	"organization_settings.alert_refire_cooldown_minutes": {"minimum", "maximum"},
}

// keywords is what a write body publishes about one property, in the forms the
// engine's attributes take.
type keywords struct {
	enum, itemEnum       []string
	min, max             *int64
	minLength, maxLength int
	minItems, maxItems   int
	pattern, format      string
	hasDefault           bool
	def                  any
	unknown              []string
	hasEnum, hasItemEnum bool
}

func intOf(v any) int {
	n, _ := v.(float64)
	return int(n)
}

func stringsOf(v any) []string {
	list, _ := v.([]any)
	out := make([]string, 0, len(list))
	for _, e := range list {
		out = append(out, fmt.Sprint(e))
	}
	return out
}

func readKeywords(schema map[string]any) keywords {
	var k keywords
	for key, v := range schema {
		switch key {
		case "type", "description", "uniqueItems":
		case "enum":
			k.hasEnum, k.enum = true, stringsOf(v)
		case "minimum":
			n := int64(intOf(v))
			k.min = &n
		case "maximum":
			n := int64(intOf(v))
			k.max = &n
		case "minLength":
			k.minLength = intOf(v)
		case "maxLength":
			k.maxLength = intOf(v)
		case "minItems":
			k.minItems = intOf(v)
		case "maxItems":
			k.maxItems = intOf(v)
		case "pattern":
			k.pattern, _ = v.(string)
		case "format":
			k.format, _ = v.(string)
		case "default":
			k.hasDefault, k.def = true, v
		case "items":
			items, _ := v.(map[string]any)
			for ik, iv := range items {
				switch ik {
				case "type", "description":
				case "enum":
					k.hasItemEnum, k.itemEnum = true, stringsOf(iv)
				default:
					k.unknown = append(k.unknown, "items."+ik)
				}
			}
		default:
			k.unknown = append(k.unknown, key)
		}
	}
	return k
}

// checkKeywords holds one attribute to every keyword its write body publishes,
// and every keyword the attribute models to a published one: limits, enums
// (items.enum for a list), defaults and patterns are the platform's, copied
// here and proved equal. A keyword the engine does not understand fails, so a
// new one is never ignored in silence. A read-only attribute is held to its
// enum alone, since nothing validates it.
func checkKeywords(resource string, a engine.Attr, f string, schema map[string]any, add func(string, ...any)) {
	// A JSON attribute is opaque text to the engine: whatever shape its schema
	// publishes is not a limit the engine could check.
	if a.Kind == engine.JSON {
		return
	}
	kw := readKeywords(schema)
	exempt := func(keyword string) bool {
		return slices.Contains(unmodelled[resource+"."+f], keyword)
	}
	enum, hasEnum := kw.enum, kw.hasEnum
	label := "enum"
	if a.Kind == engine.StringList {
		enum, hasEnum, label = kw.itemEnum, kw.hasItemEnum, "items.enum"
	}
	switch {
	case hasEnum && !slices.Equal(a.OneOf, enum):
		if len(a.OneOf) > 0 || !exempt(label) {
			add("field %s OneOf %v, contract %s %v", f, a.OneOf, label, enum)
		}
	case !hasEnum && len(a.OneOf) > 0:
		add("field %s has OneOf but the contract declares no %s", f, label)
	}
	if a.Computed || a.Secret {
		return
	}
	for _, u := range kw.unknown {
		add("field %s: the contract publishes %q, which the engine does not understand", f, u)
	}
	same := func(have, want *int64) bool {
		return (have == nil) == (want == nil) && (have == nil || *have == *want)
	}
	show := func(n *int64) any {
		if n == nil {
			return "none"
		}
		return *n
	}
	if !same(a.Min, kw.min) && (a.Min != nil || !exempt("minimum")) {
		add("field %s Min %v, contract minimum %v", f, show(a.Min), show(kw.min))
	}
	if !same(a.Max, kw.max) && (a.Max != nil || !exempt("maximum")) {
		add("field %s Max %v, contract maximum %v", f, show(a.Max), show(kw.max))
	}
	if a.MinLength != kw.minLength {
		add("field %s MinLength %d, contract minLength %d", f, a.MinLength, kw.minLength)
	}
	if a.MaxLength != kw.maxLength {
		add("field %s MaxLength %d, contract maxLength %d", f, a.MaxLength, kw.maxLength)
	}
	if a.Pattern != kw.pattern {
		add("field %s Pattern %q, contract pattern %q", f, a.Pattern, kw.pattern)
	}
	if a.MinItems != kw.minItems {
		add("field %s MinItems %d, contract minItems %d", f, a.MinItems, kw.minItems)
	}
	switch {
	case kw.hasDefault && a.Default == "":
		add("field %s has no Default, contract default %v", f, kw.def)
	case kw.hasDefault && !sameJSON(a.Default, kw.def):
		add("field %s Default %s, contract default %v", f, a.Default, kw.def)
	case !kw.hasDefault && a.Default != "":
		add("field %s Default %s, contract publishes no default", f, a.Default)
	}
	switch kw.format {
	case "", "password":
	default:
		add("field %s: the contract publishes format %q, which the engine does not understand", f, kw.format)
	}
}

// checkNullAs holds a NullAs to the contract's terraform_null_as: both name the
// field with the same value, the update body accepts the null it is sent as, and
// the value lies where no real value can (outside the published bounds or off
// the published pattern), so a real value is never mistaken for the null.
func checkNullAs(a engine.Attr, f string, res Resource, body map[string]any, add func(string, ...any)) {
	published, ok := res.TerraformNullAs[f]
	switch {
	case !ok && a.NullAs == "":
		return
	case !ok:
		add("field %s has null as %q in the spec but none in the contract", f, a.NullAs)
		return
	case a.NullAs == "":
		add("field %s has null as %v in the contract but none in the spec", f, published)
		return
	case !sameJSON(a.NullAs, published):
		add("field %s null as %q in the spec, %v in the contract", f, a.NullAs, published)
		return
	}
	if _, nullable := typeOf(body); !nullable {
		add("field %s reads a null as %v, but its update body does not accept null", f, published)
	}
	kw := readKeywords(body)
	switch v := published.(type) {
	case float64:
		inside := (kw.min == nil || v >= float64(*kw.min)) && (kw.max == nil || v <= float64(*kw.max))
		if kw.min == nil && kw.max == nil || inside {
			add("field %s: the null stand-in %v is a value the contract's bounds allow", f, v)
		}
	case string:
		off := kw.pattern != "" && !regexp.MustCompile(kw.pattern).MatchString(v)
		short := kw.minLength > 0 && utf8.RuneCountInString(v) < kw.minLength
		if !off && !short {
			add("field %s: the null stand-in %q is a value the contract's pattern allows", f, v)
		}
	}
}

// computedKind is the attribute kind a published computed field type maps to.
func computedKind(published any) engine.Kind {
	name := fmt.Sprint(published)
	if types, ok := published.([]any); ok {
		for _, t := range types {
			if t != "null" {
				name = fmt.Sprint(t)
			}
		}
	}
	switch name {
	case "boolean":
		return engine.Bool
	case "integer":
		return engine.Int
	case "number":
		return engine.Float
	case "string":
		return engine.String
	}
	return engine.JSON
}

// checkReadOnlySingleton holds a read-only singleton, one with no writable
// field, to every computed field the contract publishes: each needs a Computed
// attribute of the published kind, so a field the platform adds cannot pass
// the contract test unread.
func checkReadOnlySingleton(s engine.Spec, res Resource, add func(string, ...any)) {
	if s.Lifecycle || s.Shape != engine.Singleton || len(res.Fields) != 0 {
		return
	}
	byName := map[string]engine.Attr{}
	for _, a := range s.Attrs {
		byName[a.Name] = a
	}
	for name, published := range res.ComputedFields {
		if name == "id" {
			continue
		}
		a, ok := byName[name]
		switch {
		case !ok:
			add("computed field %s is published by the contract but has no attribute", name)
		case !a.Computed:
			add("computed field %s must be a Computed attribute", name)
		case a.Kind != computedKind(published):
			add("computed field %s kind %d, contract type %v", name, a.Kind, published)
		}
	}
}

func TestEverySpecMatchesTheContract(t *testing.T) {
	d := doc(t)
	for _, s := range specs.All() {
		t.Run(s.Key, func(t *testing.T) {
			for _, problem := range specProblems(d, s) {
				t.Error(problem)
			}
		})
	}
}

// checkSecrets holds each Secret attribute to secret_fields, and a secret's
// CreateOnly and Required to the create and update fields and the POST body.
func checkSecrets(d *Doc, s engine.Spec, res Resource, add func(string, ...any)) {
	post := d.Paths["/"+s.Key]["post"].RequestBody.Content["application/json"].Schema
	for _, a := range s.Attrs {
		if !a.Secret {
			continue
		}
		if !slices.Contains(res.SecretFields, a.Name) {
			add("Secret %s is not in the contract's secret_fields", a.Name)
		}
		if !s.Lifecycle || s.Shape == engine.Singleton {
			continue
		}
		createOnly := slices.Contains(res.CreateFields, a.Name) && !slices.Contains(res.UpdateFields, a.Name)
		if a.CreateOnly != createOnly {
			add("secret %s CreateOnly %v, contract %v", a.Name, a.CreateOnly, createOnly)
		}
		if a.Required != slices.Contains(post.Required, a.Name) {
			add("secret %s Required %v, contract required %v", a.Name, a.Required, post.Required)
		}
	}
}

// checkAddressing holds a spec's Shape and IDAttr to how the contract
// addresses a row: id_args and natural_key.
func checkAddressing(s engine.Spec, res Resource, add func(string, ...any)) {
	var want []string
	switch s.Shape {
	case engine.Singleton:
		want = []string{}
	case engine.Generated:
		want = []string{"id"}
		// A paged view is not stored configuration: it has no row to address.
		if s.Cursor != "" {
			want = []string{}
		}
	case engine.NaturalKey:
		want = []string{s.IDAttr}
		if !slices.Equal(res.NaturalKey, want) {
			add("natural key %v, contract natural_key %v", want, res.NaturalKey)
		}
	case engine.ServiceBinding:
		want = []string{"service", "environment"}
	}
	got := res.IDArgs
	if got == nil {
		got = []string{}
	}
	if !slices.Equal(got, want) {
		add("Shape %d addresses a row by %v, contract id_args %v", s.Shape, want, got)
	}
}

func TestEveryContractResourceHasASpec(t *testing.T) {
	d := doc(t)
	have := map[string]bool{}
	for _, s := range specs.All() {
		have[s.Key] = true
	}
	for key := range d.Resources {
		if !have[key] {
			t.Errorf("the API serves %s but the provider has no spec for it", key)
		}
	}
}

// The contract test must fail when the document drifts from a spec: each
// mutation below is a change the server could ship.
func TestTheContractTestCatchesDrift(t *testing.T) {
	for _, c := range []struct {
		name, key string
		mutate    func(d *Doc)
	}{
		{"a secret leaves secret_fields", "outbound_configs", func(d *Doc) {
			r := d.Resources["outbound_configs"]
			r.SecretFields = slices.DeleteFunc(slices.Clone(r.SecretFields), func(f string) bool { return f == "routing_key" })
			d.Resources["outbound_configs"] = r
		}},
		{"a secret leaves update_fields", "connectors", func(d *Doc) {
			r := d.Resources["connectors"]
			r.UpdateFields = slices.DeleteFunc(slices.Clone(r.UpdateFields), func(f string) bool { return f == "config" })
			d.Resources["connectors"] = r
		}},
		{"a secret stops being required", "connectors", func(d *Doc) {
			op := d.Paths["/connectors"]["post"]
			body := op.RequestBody.Content["application/json"]
			body.Schema.Required = slices.DeleteFunc(slices.Clone(body.Schema.Required), func(f string) bool { return f == "config" })
			op.RequestBody.Content["application/json"] = body
			d.Paths["/connectors"]["post"] = op
		}},
		{"a natural key becomes a generated id", "ticket_integrations", func(d *Doc) {
			r := d.Resources["ticket_integrations"]
			r.IDArgs = []string{"id"}
			d.Resources["ticket_integrations"] = r
		}},
		{"the natural key changes", "ticket_import_rules", func(d *Doc) {
			r := d.Resources["ticket_import_rules"]
			r.NaturalKey = []string{"name"}
			d.Resources["ticket_import_rules"] = r
		}},
		{"a read-only field leaves terraform_read_only", "organization_settings", func(d *Doc) {
			r := d.Resources["organization_settings"]
			r.TerraformReadOnly = slices.DeleteFunc(slices.Clone(r.TerraformReadOnly), func(f string) bool { return f == "social_joins_enabled" })
			d.Resources["organization_settings"] = r
		}},
		{"mfa_required leaves terraform_read_only", "organization_settings", func(d *Doc) {
			r := d.Resources["organization_settings"]
			r.TerraformReadOnly = slices.DeleteFunc(slices.Clone(r.TerraformReadOnly), func(f string) bool { return f == "mfa_required" })
			d.Resources["organization_settings"] = r
		}},
		{"a read-only collection gains a field", "secret_allow_rules", func(d *Doc) {
			r := d.Resources["secret_allow_rules"]
			r.Fields = append(slices.Clone(r.Fields), "severity")
			d.Resources["secret_allow_rules"] = r
		}},
		{"fix_request_floor leaves terraform_read_only", "organization_settings", func(d *Doc) {
			r := d.Resources["organization_settings"]
			r.TerraformReadOnly = slices.DeleteFunc(slices.Clone(r.TerraformReadOnly), func(f string) bool { return f == "fix_request_floor" })
			d.Resources["organization_settings"] = r
		}},
		{"fix_request_floor gains an enum value", "organization_settings", func(d *Doc) {
			op := d.Paths["/organization_settings"]["put"]
			op.RequestBody.Content["application/json"].Schema.Properties["fix_request_floor"]["enum"] = []any{"org_admin", "member", "viewer"}
			d.Paths["/organization_settings"]["put"] = op
		}},
		{"inheritance_stopped becomes terraform_read_only", "organization_settings", func(d *Doc) {
			r := d.Resources["organization_settings"]
			r.TerraformReadOnly = append(slices.Clone(r.TerraformReadOnly), "inheritance_stopped")
			d.Resources["organization_settings"] = r
		}},
		{"a null meaning changes", "ai_providers", func(d *Doc) {
			r := d.Resources["ai_providers"]
			r.NullMeans["temperature"] = 0.5
			d.Resources["ai_providers"] = r
		}},
		{"a read-only singleton gains a computed field", "request_signing", func(d *Doc) {
			r := d.Resources["request_signing"]
			r.ComputedFields = maps.Clone(r.ComputedFields)
			r.ComputedFields["rotation_due"] = "string"
			d.Resources["request_signing"] = r
		}},
		{"a read-only singleton's computed field changes type", "request_signing", func(d *Doc) {
			r := d.Resources["request_signing"]
			r.ComputedFields = maps.Clone(r.ComputedFields)
			r.ComputedFields["remote_locations_pending"] = "string"
			d.Resources["request_signing"] = r
		}},
		{"the fence list's maxItems changes", "repo_settings", func(d *Doc) {
			op := d.Paths["/repo_settings/{id}"]["put"]
			op.RequestBody.Content["application/json"].Schema.Properties["path_prefixes"]["maxItems"] = float64(50)
			d.Paths["/repo_settings/{id}"]["put"] = op
		}},
		{"follows_default leaves action_args", "repo_settings", func(d *Doc) {
			r := d.Resources["repo_settings"]
			r.ActionArgs = slices.DeleteFunc(slices.Clone(r.ActionArgs), func(f string) bool { return f == "follows_default" })
			d.Resources["repo_settings"] = r
		}},
		{"follows_default gains a setting", "repo_settings", func(d *Doc) {
			op := d.Paths["/repo_settings/{id}"]["put"]
			items := op.RequestBody.Content["application/json"].Schema.Properties["follows_default"]["items"].(map[string]any)
			items["enum"] = append(slices.Clone(items["enum"].([]any)), "service")
			d.Paths["/repo_settings/{id}"]["put"] = op
		}},
		{"set_here leaves the computed fields", "repo_settings", func(d *Doc) {
			r := d.Resources["repo_settings"]
			r.ComputedFields = maps.Clone(r.ComputedFields)
			delete(r.ComputedFields, "set_here")
			d.Resources["repo_settings"] = r
		}},
		{"inheritance_stopped stops listing repository_defaults", "organization_settings", func(d *Doc) {
			op := d.Paths["/organization_settings"]["put"]
			items := op.RequestBody.Content["application/json"].Schema.Properties["inheritance_stopped"]["items"].(map[string]any)
			items["enum"] = []any{"slack", "github_installation", "ai_providers", "outbound_targets", "ticket_integrations"}
			d.Paths["/organization_settings"]["put"] = op
		}},
		{"draft_prs stops being nullable", "repo_settings", func(d *Doc) {
			op := d.Paths["/repo_settings/{id}"]["put"]
			op.RequestBody.Content["application/json"].Schema.Properties["draft_prs"]["type"] = "boolean"
			d.Paths["/repo_settings/{id}"]["put"] = op
		}},
		{"the 428 publishes a preview without a key the provider reads", "repository_defaults", func(d *Doc) {
			op := d.Paths["/repository_defaults"]["put"]
			schema := op.Responses["428"].Content["application/json"].Schema
			schema.Properties["preview"] = map[string]any{"type": "object", "properties": map[string]any{"data": map[string]any{"type": "object", "properties": map[string]any{"fingerprint": map[string]any{"type": "string"}}}}}
			d.Paths["/repository_defaults"]["put"] = op
		}},
		{"the 428 publishes the preview's keys one level too high", "repository_defaults", func(d *Doc) {
			op := d.Paths["/repository_defaults"]["put"]
			schema := op.Responses["428"].Content["application/json"].Schema
			flat := map[string]any{}
			for _, k := range engine.PreviewKeys() {
				flat[k] = map[string]any{}
			}
			schema.Properties["preview"] = map[string]any{"type": "object", "properties": flat}
			d.Paths["/repository_defaults"]["put"] = op
		}},
		{"a list's maxItems changes", "service_bindings", func(d *Doc) {
			op := d.Paths["/service_bindings/{id}"]["put"]
			op.RequestBody.Content["application/json"].Schema.Properties["log_inline_fields"]["maxItems"] = float64(5)
			d.Paths["/service_bindings/{id}"]["put"] = op
		}},
		{"the SLA days lose terraform_null_as", "security_sla_policy", func(d *Doc) {
			r := d.Resources["security_sla_policy"]
			r.TerraformNullAs = nil
			d.Resources["security_sla_policy"] = r
		}},
		{"the SLA null stand-in changes", "security_sla_policy", func(d *Doc) {
			r := d.Resources["security_sla_policy"]
			r.TerraformNullAs = map[string]any{"critical_days": float64(500), "high_days": float64(0), "medium_days": float64(0), "low_days": float64(0), "info_days": float64(0)}
			d.Resources["security_sla_policy"] = r
		}},
		{"the SLA minimum drops to the null stand-in", "security_sla_policy", func(d *Doc) {
			op := d.Paths["/security_sla_policy"]["put"]
			op.RequestBody.Content["application/json"].Schema.Properties["high_days"]["minimum"] = float64(0)
			d.Paths["/security_sla_policy"]["put"] = op
		}},
		{"the SLA days stop being nullable", "security_sla_policy", func(d *Doc) {
			op := d.Paths["/security_sla_policy"]["put"]
			op.RequestBody.Content["application/json"].Schema.Properties["info_days"]["type"] = "integer"
			d.Paths["/security_sla_policy"]["put"] = op
		}},
		{"a day's maximum moves", "security_sla_policy", func(d *Doc) {
			op := d.Paths["/security_sla_policy"]["put"]
			op.RequestBody.Content["application/json"].Schema.Properties["low_days"]["maximum"] = float64(366)
			d.Paths["/security_sla_policy"]["put"] = op
		}},
		{"a rule name's maxLength moves", "security_notification_rules", func(d *Doc) {
			op := d.Paths["/security_notification_rules"]["post"]
			op.RequestBody.Content["application/json"].Schema.Properties["name"]["maxLength"] = float64(81)
			d.Paths["/security_notification_rules"]["post"] = op
		}},
		{"min_severity gains a value", "security_notification_rules", func(d *Doc) {
			op := d.Paths["/security_notification_rules"]["post"]
			p := op.RequestBody.Content["application/json"].Schema.Properties["min_severity"]
			p["enum"] = append(slices.Clone(p["enum"].([]any)), "fatal")
			d.Paths["/security_notification_rules"]["post"] = op
		}},
		{"the areas' items gain a value", "security_notification_rules", func(d *Doc) {
			op := d.Paths["/security_notification_rules"]["post"]
			items := op.RequestBody.Content["application/json"].Schema.Properties["areas"]["items"].(map[string]any)
			items["enum"] = append(slices.Clone(items["enum"].([]any)), "network")
			d.Paths["/security_notification_rules"]["post"] = op
		}},
		{"a published rule default changes", "security_notification_rules", func(d *Doc) {
			op := d.Paths["/security_notification_rules"]["post"]
			op.RequestBody.Content["application/json"].Schema.Properties["min_severity"]["default"] = "low"
			d.Paths["/security_notification_rules"]["post"] = op
		}},
		{"a published rule default disappears", "security_notification_rules", func(d *Doc) {
			op := d.Paths["/security_notification_rules"]["post"]
			delete(op.RequestBody.Content["application/json"].Schema.Properties["page"], "default")
			d.Paths["/security_notification_rules"]["post"] = op
		}},
		{"events stop requiring one item", "security_notification_rules", func(d *Doc) {
			op := d.Paths["/security_notification_rules"]["post"]
			op.RequestBody.Content["application/json"].Schema.Properties["events"]["minItems"] = float64(0)
			d.Paths["/security_notification_rules"]["post"] = op
		}},
		{"a keyword the engine does not understand appears", "security_notification_rules", func(d *Doc) {
			op := d.Paths["/security_notification_rules"]["post"]
			op.RequestBody.Content["application/json"].Schema.Properties["name"]["exclusiveMinimum"] = float64(0)
			d.Paths["/security_notification_rules"]["post"] = op
		}},
		{"classes maxItems moves", "security_code_classes", func(d *Doc) {
			op := d.Paths["/security_code_classes"]["put"]
			op.RequestBody.Content["application/json"].Schema.Properties["classes"]["maxItems"] = float64(60)
			d.Paths["/security_code_classes"]["put"] = op
		}},
		{"the channel pattern changes", "security_digest_settings", func(d *Doc) {
			op := d.Paths["/security_digest_settings"]["put"]
			op.RequestBody.Content["application/json"].Schema.Properties["slack_channel"]["pattern"] = "^#[a-z]+$"
			d.Paths["/security_digest_settings"]["put"] = op
		}},
		{"the digest channel's null stand-in starts matching its pattern", "security_digest_settings", func(d *Doc) {
			op := d.Paths["/security_digest_settings"]["put"]
			op.RequestBody.Content["application/json"].Schema.Properties["slack_channel"]["pattern"] = "^.*$"
			d.Paths["/security_digest_settings"]["put"] = op
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			d := doc(t)
			var spec engine.Spec
			for _, s := range specs.All() {
				if s.Key == c.key {
					spec = s
				}
			}
			// A resource still read as a data source has no Lifecycle spec to hold to
			// the document; its stand-in is the spec it will be.
			if standIn := standIns[c.key]; !spec.Lifecycle && standIn != nil {
				spec = standIn()
			}
			if problems := specProblems(d, spec); len(problems) != 0 {
				t.Fatalf("the untouched document already disagrees: %v", problems)
			}
			c.mutate(d)
			if len(specProblems(d, spec)) == 0 {
				t.Fatal("the contract test missed this drift")
			}
		})
	}
}

func TestPreviewSchemaIsHeldToTheWireShape(t *testing.T) {
	full := map[string]any{}
	for _, k := range engine.PreviewKeys() {
		full[k] = map[string]any{}
	}
	partial := map[string]any{"fingerprint": map[string]any{}}
	nested := func(keys map[string]any) map[string]any {
		return map[string]any{"type": "object", "properties": map[string]any{"data": map[string]any{"type": "object", "properties": keys}}}
	}
	for _, c := range []struct {
		name    string
		preview map[string]any
		bad     bool
	}{
		{"a bare object is accepted until the platform publishes the shape", map[string]any{"type": "object"}, false},
		{"a bare data object is accepted", map[string]any{"type": "object", "properties": map[string]any{"data": map[string]any{"type": "object"}}}, false},
		{"the full nested set passes", nested(full), false},
		{"a nested set missing a key fails", nested(partial), true},
		{"the keys one level too high fail", map[string]any{"type": "object", "properties": full}, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := previewSchemaProblems(c.preview); (len(got) > 0) != c.bad {
				t.Fatalf("problems = %v, want bad = %v", got, c.bad)
			}
		})
	}
}

// Every published pattern the engine copies must compile in the engine's regular
// expression syntax, or a schema would panic at the first plan.
func TestEveryPatternTheEngineHoldsCompiles(t *testing.T) {
	for _, s := range append(specs.All(), standIns["security_digest_settings"](), standIns["security_notification_rules"]()) {
		for _, a := range s.Attrs {
			if a.Pattern == "" {
				continue
			}
			if _, err := regexp.Compile(a.Pattern); err != nil {
				t.Errorf("%s.%s: pattern %q does not compile: %v", s.Key, a.Name, a.Pattern, err)
			}
		}
	}
}
