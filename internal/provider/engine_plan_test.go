package provider

import (
	"fmt"
	"regexp"
	"slices"
	"sort"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/segfaultpw/terraform-provider-sreagent/internal/engine"
)

// scratchAllow is a collection with no update door, the shape of the secret allow
// rule: every field is create-only, and the optional ones are nullable on create.
var scratchAllow = engine.Spec{
	Key: "scratch_allow", TypeName: "scratch_allow", ListName: "scratch_allow", Shape: engine.Generated, Lifecycle: true,
	Description: "A scratch collection with no update door.",
	Attrs: []engine.Attr{
		{Name: "path_glob", Kind: engine.String, Required: true, CreateOnly: true, Trim: true, MinLength: 1, MaxLength: 200, Description: "Paths."},
		{Name: "repository", Kind: engine.String, CreateOnly: true, Clearable: true, Normalize: engine.Lower, Description: "owner/name."},
		{Name: "expires_at", Kind: engine.String, CreateOnly: true, Clearable: true, Timestamp: true, Description: "When it ends."},
		{Name: "created_at", Kind: engine.String, Computed: true, Description: "When it was added."},
	},
}

// microseconds is the form the platform answers a time in: UTC with six digits.
func microseconds(row map[string]any, key string) {
	if v, ok := row[key].(string); ok {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			row[key] = t.UTC().Format("2006-01-02T15:04:05.000000Z07:00")
		}
	}
}

// Removing a nullable field of a collection with no update door replaces the row,
// so the new one does not keep the scope the old one had; and the same instant in
// another spelling is neither a change nor a replacement.
func TestAClearableCreateOnlyNullReplaces(t *testing.T) {
	f, factories := scratch(t, scratchAllow)
	f.OnWrite("scratch_allow", func(row map[string]any) { microseconds(row, "expires_at") })
	cfg := func(body string) string {
		return providerBlock(f.URL) + fmt.Sprintf("resource \"sreagent_scratch_allow\" \"a\" {\n  path_glob = \"**/testdata/**\"\n%s}\n", body)
	}
	var firstID string
	idOf := func(s *terraform.State) string {
		return s.RootModule().Resources["sreagent_scratch_allow.a"].Primary.ID
	}
	empty := resource.ConfigPlanChecks{PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config:           cfg("  repository = \"Acme/Web\"\n  expires_at = \"2027-01-01T01:00:00+01:00\"\n"),
				ConfigPlanChecks: empty,
				Check: func(s *terraform.State) error {
					firstID = idOf(s)
					row := f.Row("scratch_allow", firstID)
					if row["repository"] != "acme/web" || row["expires_at"] != "2027-01-01T00:00:00.000000Z" {
						return fmt.Errorf("the platform stores %v", row)
					}
					return nil
				},
			},
			{
				// The same instant, written in UTC: no diff, and above all no replacement.
				Config:           cfg("  repository = \"Acme/Web\"\n  expires_at = \"2027-01-01T00:00:00Z\"\n"),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
			},
			{
				// A refresh sees the platform's spelling and keeps the one in state.
				Config:   cfg("  repository = \"Acme/Web\"\n  expires_at = \"2027-01-01T00:00:00Z\"\n"),
				PlanOnly: true,
			},
			{
				// Removing a nullable create-only field replaces the row: it must not keep its scope.
				Config: cfg("  expires_at = \"2027-01-01T00:00:00Z\"\n"),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction("sreagent_scratch_allow.a", plancheck.ResourceActionDestroyBeforeCreate),
				}},
				Check: func(s *terraform.State) error {
					if idOf(s) == firstID {
						return fmt.Errorf("the row must have been replaced")
					}
					if row := f.Row("scratch_allow", idOf(s)); row["repository"] != nil {
						return fmt.Errorf("the new rule must cover every repository, got %v", row["repository"])
					}
					return nil
				},
			},
			{Config: cfg("  expires_at = \"2027-01-02T00:00:00Z\"\n"), ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
				plancheck.ExpectResourceAction("sreagent_scratch_allow.a", plancheck.ResourceActionDestroyBeforeCreate),
			}}},
			{Config: cfg("  expires_at = \"2027-01-02\"\n"), PlanOnly: true, ExpectError: wrapped("Not a time with a zone", "time zone")},
			{
				Config:      providerBlock(f.URL) + "resource \"sreagent_scratch_allow\" \"a\" {\n  path_glob = \" padded\"\n}\n",
				PlanOnly:    true,
				ExpectError: wrapped("Surrounding whitespace", "padded"),
			},
			{Config: cfg("  expires_at = \"2027-01-02T00:00:00Z\"\n")},
		},
	})
}

// scratchClasses is a singleton list the platform stores sorted and deduplicated.
var scratchClasses = engine.Spec{
	Key: "scratch_classes", TypeName: "scratch_classes", ListName: "scratch_classes", Shape: engine.Singleton, Lifecycle: true,
	Description: "A scratch singleton with a sorted list.",
	Attrs: []engine.Attr{
		{Name: "classes", Kind: engine.StringList, Unordered: true, MaxItems: 3, RemovedWarning: "Unmarking %s closes its findings.", Description: "The classes."},
		{Name: "limit", Kind: engine.Int, Computed: true, Description: "The most classes."},
	},
}

func TestAListThePlatformSortsPlansCleanInAnyOrder(t *testing.T) {
	f, factories := scratch(t, scratchClasses)
	f.OnWrite("scratch_classes", func(row map[string]any) {
		if list, ok := row["classes"].([]any); ok {
			names := make([]string, 0, len(list))
			for _, e := range list {
				if s := fmt.Sprint(e); !slices.Contains(names, s) {
					names = append(names, s)
				}
			}
			sort.Strings(names)
			sorted := make([]any, 0, len(names))
			for _, n := range names {
				sorted = append(sorted, n)
			}
			row["classes"] = sorted
		}
	})
	cfg := func(list string) string {
		return providerBlock(f.URL) + fmt.Sprintf("resource \"sreagent_scratch_classes\" \"c\" {\n  classes = %s\n}\n", list)
	}
	empty := resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config:           cfg(`["other:b", "other:a"]`),
				ConfigPlanChecks: resource.ConfigPlanChecks{PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
				Check: func(*terraform.State) error {
					got := fmt.Sprint(f.Row("scratch_classes", "scratch_classes")["classes"])
					if got != "[other:a other:b]" {
						return fmt.Errorf("the platform stores them sorted, got %s", got)
					}
					return nil
				},
			},
			// The same entries in any order are no change, before and after a refresh.
			{Config: cfg(`["other:a", "other:b"]`), ConfigPlanChecks: empty},
			{Config: cfg(`["other:b", "other:a"]`), PlanOnly: true},
			{Config: cfg(`["other:a", "other:b", "other:c"]`), Check: resource.TestCheckResourceAttr("sreagent_scratch_classes.c", "classes.#", "3")},
			{Config: cfg(`["other:a", "other:b", "other:c", "other:d"]`), PlanOnly: true, ExpectError: regexp.MustCompile(`(?s)classes.*3`)},
			{Config: cfg(`[]`), Check: resource.TestCheckResourceAttr("sreagent_scratch_classes.c", "classes.#", "0")},
		},
	})
}

// scratchPaging judges the merged row, so a value the configuration leaves to
// state counts: the platform refuses a rule that pages below critical whichever
// attribute the change came from.
var scratchPaging = engine.Spec{
	Key: "scratch_paging", TypeName: "scratch_paging", ListName: "scratch_paging", Shape: engine.Generated, Lifecycle: true,
	Description: "A scratch collection judged across attributes.",
	Attrs: []engine.Attr{
		{Name: "name", Kind: engine.String, Required: true, Description: "The name."},
		{Name: "page", Kind: engine.Bool, Description: "Page on call."},
		{Name: "min_severity", Kind: engine.String, Default: `"high"`, OneOf: []string{"high", "critical"}, Description: "Severity."},
	},
	PlanChecks: []func(plan, prior engine.Config) []engine.Issue{func(plan, _ engine.Config) []engine.Issue {
		page, pageSeen := plan.Bool("page")
		severity, severitySeen := plan.String("min_severity")
		if pageSeen == engine.Present && page && severitySeen == engine.Present && severity != "critical" {
			return []engine.Issue{{Attr: "page", Summary: "Paging needs a critical rule", Detail: "Paging is only allowed on a rule whose minimum severity is critical. Set min_severity = \"critical\", or page = false."}}
		}
		return nil
	}},
}

func TestAPlanCheckJudgesWhatTheConfigurationLeavesToState(t *testing.T) {
	f, factories := scratch(t, scratchPaging)
	cfg := func(body string) string {
		return providerBlock(f.URL) + fmt.Sprintf("resource \"sreagent_scratch_paging\" \"p\" {\n  name = \"a\"\n%s}\n", body)
	}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{Config: cfg("  page = true\n  min_severity = \"critical\"\n"), Check: resource.TestCheckResourceAttr("sreagent_scratch_paging.p", "page", "true")},
			// page is left out, so state's true is what the plan carries: a high rule would page.
			{Config: cfg("  min_severity = \"high\"\n"), PlanOnly: true, ExpectError: wrapped("Paging needs a critical rule", "min_severity = \"critical\"")},
			// min_severity is removed, so its default is what the plan carries.
			{Config: cfg("  page = true\n"), PlanOnly: true, ExpectError: wrapped("Paging needs a critical rule")},
			// A create is judged too.
			{Config: providerBlock(f.URL) + "resource \"sreagent_scratch_paging\" \"q\" {\n  name = \"b\"\n  page = true\n}\n", PlanOnly: true, ExpectError: wrapped("Paging needs a critical rule")},
			{Config: cfg("  page = false\n  min_severity = \"high\"\n"), Check: resource.TestCheckResourceAttr("sreagent_scratch_paging.p", "page", "false")},
		},
	})
}
