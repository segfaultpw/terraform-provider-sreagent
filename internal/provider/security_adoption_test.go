package provider

import (
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/segfaultpw/terraform-provider-sreagent/internal/fakefacade"
	"github.com/segfaultpw/terraform-provider-sreagent/internal/specs"
)

// A block written for a setting the organization already has adopts it with no import, and the
// apply overwrites what the block names. The plan is a read, so it says what the apply will move.

func writes(f *fakefacade.Fake, key string) int {
	return f.Calls("PUT", key) + f.Calls("POST", key) + f.Calls("DELETE", key)
}

func markFive(f *fakefacade.Fake) {
	f.SetAcceptedClasses("other:a", "other:b", "other:c", "other:d", "other:e")
	f.Mutate("security_code_classes", "security_code_classes", func(row map[string]any) {
		row["classes"] = []any{"other:a", "other:b", "other:c", "other:d", "other:e"}
		row["available_classes"] = []any{}
	})
}

func TestAdoptingCodeClassesWarnsEachClassItUnmarks(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	markFive(f)
	d := newDirect(t, specs.SecurityCodeClasses, f)
	resp := d.adopt(map[string]tftypes.Value{"classes": strList("other:a")})
	if resp.Diagnostics.HasError() {
		t.Fatalf("adopting a valid list is not an error: %v", resp.Diagnostics)
	}
	for _, c := range []string{"other:b", "other:c", "other:d", "other:e"} {
		if !warned(resp.Diagnostics, "Unmarking "+c+" closes its open security findings", "ends an acceptance") {
			t.Errorf("adopting must warn that %s is unmarked and its findings close: %v", c, resp.Diagnostics)
		}
	}
	if warned(resp.Diagnostics, "Unmarking other:a") {
		t.Errorf("a class the block keeps is not unmarked: %v", resp.Diagnostics)
	}
	if !warned(resp.Diagnostics, "adopts the settings the organization already has", "classes", "terraform import sreagent_security_code_classes.<name> security_code_classes") {
		t.Errorf("the plan must say the block adopts what exists and how to import first: %v", resp.Diagnostics)
	}
	if n := writes(f, "security_code_classes"); n != 0 {
		t.Errorf("a plan is a read, it made %d writes", n)
	}
}

func TestAdoptingCodeClassesThatChangeNothingSaysNothing(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	markFive(f)
	d := newDirect(t, specs.SecurityCodeClasses, f)
	resp := d.adopt(map[string]tftypes.Value{"classes": strList("other:e", "other:d", "other:c", "other:b", "other:a")})
	if resp.Diagnostics.HasError() || resp.Diagnostics.WarningsCount() != 0 {
		t.Fatalf("the same set in another order changes nothing: %v", resp.Diagnostics)
	}
}

// The checks a create used to leave to the apply run against the classes the organization
// holds, so a class it never accepted, or a built-in one, fails the plan.
func TestAdoptingCodeClassesFailsThePlanOnAClassTheOrganizationNeverAccepted(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	f.SetAcceptedClasses("other:real")
	d := newDirect(t, specs.SecurityCodeClasses, f)
	resp := d.adopt(map[string]tftypes.Value{"classes": strList("other:ghost")})
	if !resp.Diagnostics.HasError() || !warnedOrErrored(resp.Diagnostics, "other:ghost is not a class this organization accepted", "available_classes: other:real", "as of the last read") {
		t.Fatalf("an unaccepted class must fail the adopting plan: %v", resp.Diagnostics)
	}
	resp = d.adopt(map[string]tftypes.Value{"classes": strList("hardcoded_secret")})
	if !warnedOrErrored(resp.Diagnostics, "hardcoded_secret always counts as security and is not listed") {
		t.Fatalf("a built-in class must fail the adopting plan: %v", resp.Diagnostics)
	}
	if resp := d.adopt(map[string]tftypes.Value{"classes": strList("other:real")}); resp.Diagnostics.HasError() {
		t.Fatalf("an accepted class is fine: %v", resp.Diagnostics)
	}
}

func TestAdoptingTheSLAWarnsEachDayThatMoves(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	d := newDirect(t, specs.SecuritySLAPolicy, f)
	resp := d.adopt(map[string]tftypes.Value{"high_days": tfNum(14), "critical_days": tfNum(7), "info_days": tfNum(0)})
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	if !warned(resp.Diagnostics, "recomputes the due date of every active finding of this severity") {
		t.Errorf("a day that moves must warn that due dates move: %v", resp.Diagnostics)
	}
	var due int
	for _, w := range resp.Diagnostics.Warnings() {
		if strings.Contains(w.Detail(), "recomputes the due date") {
			due++
		}
	}
	if due != 1 {
		t.Errorf("only high_days moves (7 and no SLA are what the organization has), got %d due date warnings", due)
	}
	if !warned(resp.Diagnostics, "adopts the settings the organization already has", "high_days from 30 to 14") {
		t.Errorf("the plan must name the day, its current and its new value: %v", resp.Diagnostics)
	}
	if warned(resp.Diagnostics, "critical_days from") || warned(resp.Diagnostics, "info_days from") {
		t.Errorf("a day the block states as it is must not be listed: %v", resp.Diagnostics)
	}
	if n := writes(f, "security_sla_policy"); n != 0 {
		t.Errorf("a plan is a read, it made %d writes", n)
	}
	// Stating the policy as it is moves nothing and says nothing.
	same := d.adopt(map[string]tftypes.Value{"critical_days": tfNum(7), "high_days": tfNum(30), "medium_days": tfNum(90), "low_days": tfNum(180), "info_days": tfNum(0)})
	if same.Diagnostics.HasError() || same.Diagnostics.WarningsCount() != 0 {
		t.Errorf("the defaults over the defaults say nothing: %v", same.Diagnostics)
	}
}

func TestAdoptingTheDigestSaysWhatChanges(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	d := newDirect(t, specs.SecurityDigestSettings, f)
	resp := d.adopt(map[string]tftypes.Value{"enabled": tfBool(true), "email": tfBool(true), "hour": tfNum(7)})
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	if !warned(resp.Diagnostics, "adopts the settings the organization already has", "enabled from false to true", "email from false to true", "hour from 9 to 7") {
		t.Errorf("the plan must list each field the digest changes: %v", resp.Diagnostics)
	}
	if warned(resp.Diagnostics, "cadence from") {
		t.Errorf("a field the block leaves out stays as it is and is not listed: %v", resp.Diagnostics)
	}
	if n := writes(f, "security_digest_settings"); n != 0 {
		t.Errorf("a plan is a read, it made %d writes", n)
	}
}

// A field the block leaves out is left as it is, so the destination check judges the row
// the apply would write: a digest switched on over a row with no channel and no email.
func TestAdoptingADigestOnWithoutADestinationFailsThePlan(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	d := newDirect(t, specs.SecurityDigestSettings, f)
	resp := d.adopt(map[string]tftypes.Value{"enabled": tfBool(true)})
	if !warnedOrErrored(resp.Diagnostics, "No destination for the digest", "Choose a Slack channel or email") || !resp.Diagnostics.HasError() {
		t.Fatalf("a digest on with nowhere to go must fail the plan: %v", resp.Diagnostics)
	}
	// The destination the organization already has counts.
	f.Mutate("security_digest_settings", "security_digest_settings", func(row map[string]any) { row["email"] = true })
	if resp := d.adopt(map[string]tftypes.Value{"enabled": tfBool(true)}); resp.Diagnostics.HasError() {
		t.Fatalf("an email already on is a destination: %v", resp.Diagnostics)
	}
}

// When the plan cannot read the settings it says so and still plans: the apply answers.
func TestAdoptingWhenThePlanCannotReadTheSettingsSaysSo(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	f.RefusePlan("security_sla_policy", infrastructureRefusal)
	d := newDirect(t, specs.SecuritySLAPolicy, f)
	resp := d.adopt(map[string]tftypes.Value{"high_days": tfNum(14)})
	if resp.Diagnostics.HasError() {
		t.Fatalf("an unreadable current row must not fail the plan: %v", resp.Diagnostics)
	}
	if !warned(resp.Diagnostics, "could not read", "what the apply will change", infrastructureRefusal, "overwrites what the block names", "terraform import sreagent_security_sla_policy.<name> security_sla_policy") {
		t.Errorf("the plan must say it could not read the settings: %v", resp.Diagnostics)
	}
}

// Importing first is the way to see the change as a plan: with state there is no adoption.
func TestAnImportedSingletonIsNotAnAdoption(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	d := newDirect(t, specs.SecuritySLAPolicy, f)
	before := f.Calls("GET", "security_sla_policy")
	state := d.value(map[string]tftypes.Value{"id": str("security_sla_policy"), "critical_days": tfNum(7), "high_days": tfNum(30), "medium_days": tfNum(90), "low_days": tfNum(180), "info_days": tfNum(0), "defaults": tfBool(true), "updated_at": str("x")})
	resp := d.plan(state, d.value(map[string]tftypes.Value{"id": str("security_sla_policy"), "critical_days": tfNum(7), "high_days": tfNum(14), "medium_days": tfNum(90), "low_days": tfNum(180), "info_days": tfNum(0), "defaults": tfBool(true), "updated_at": str("x")}))
	if warned(resp.Diagnostics, "adopts the settings") {
		t.Errorf("a resource with state is not adopting: %v", resp.Diagnostics)
	}
	if n := f.Calls("GET", "security_sla_policy") - before; n != 0 {
		t.Errorf("an update plan must not read the platform, it made %d GETs", n)
	}
}

// What the released singletons do on a create is unchanged: no read at plan, no warning.
func TestAReleasedSingletonDoesNotReadAtPlanOnACreate(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	d := newDirect(t, specs.OrganizationSettings, f)
	before := f.Calls("GET", "organization_settings")
	resp := d.adopt(map[string]tftypes.Value{})
	if resp.Diagnostics.WarningsCount() != 0 {
		t.Errorf("a released singleton says nothing on a create: %v", resp.Diagnostics)
	}
	if n := f.Calls("GET", "organization_settings") - before; n != 0 {
		t.Errorf("a released singleton must not read at plan, it made %d GETs", n)
	}
}
