package provider

import (
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/segfaultpw/terraform-provider-sreagent/internal/fakefacade"
	"github.com/segfaultpw/terraform-provider-sreagent/internal/specs"
)

// A write or a reset of fix_runner, platform_fallback or auto_dispatch reaches
// every row of the repository; the plan says so for the row that carries it.

func repoPlan(t *testing.T, f *fakefacade.Fake, state, config, plan map[string]tftypes.Value) diag.Diagnostics {
	t.Helper()
	d := modifyPlanOf(t, f, "repo_settings", state, config, plan, false)
	if d.HasError() {
		t.Fatalf("the plan failed: %v", d)
	}
	return d
}

func planRow(extra map[string]tftypes.Value) map[string]tftypes.Value {
	row := map[string]tftypes.Value{"id": str("00000000-0000-0000-0000-000000000001"), "repo": str("acme/api")}
	for k, v := range extra {
		row[k] = v
	}
	return row
}

func TestACreateThatStatesARepositoryWideSettingWarnsItReachesTheWholeRepository(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	cfg := planRow(map[string]tftypes.Value{"branch": str("main"), "fix_runner": str("platform")})
	text := warningText(repoPlan(t, f, nil, cfg, cfg))
	for _, want := range []string{"fix_runner", "every row of acme/api", "branch entries included"} {
		if !strings.Contains(text, want) {
			t.Errorf("the warning lacks %q:\n%s", want, text)
		}
	}
}

func TestACreateThatNamesARepositoryWideResetWarnsItMayReachTheWholeRepository(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	cfg := planRow(map[string]tftypes.Value{"branch": str("staging"), "follows_default": strList("platform_fallback", "check_network")})
	text := warningText(repoPlan(t, f, nil, cfg, cfg))
	if !strings.Contains(text, "platform_fallback") || !strings.Contains(text, "every row of acme/api") || !strings.Contains(text, "If the new row holds a value for it") {
		t.Fatalf("the warning is:\n%s", text)
	}
	if strings.Contains(text, "check_network") {
		t.Errorf("a setting that belongs to one row is named as repository-wide:\n%s", text)
	}
}

func TestAnUpdateThatChangesARepositoryWideSettingWarnsItReachesTheWholeRepository(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	state := planRow(map[string]tftypes.Value{"fix_runner": str("platform")})
	cfg := planRow(map[string]tftypes.Value{"fix_runner": str("self_hosted")})
	text := warningText(repoPlan(t, f, state, cfg, cfg))
	if !strings.Contains(text, "fix_runner") || !strings.Contains(text, "every row of acme/api") {
		t.Fatalf("the warning is:\n%s", text)
	}
}

func TestAResetOfARepositoryWideSettingSaysItReachesTheWholeRepository(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	held := tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, []tftypes.Value{str("auto_dispatch"), str("check_network")})
	state := planRow(map[string]tftypes.Value{"auto_dispatch": boolean(false), "check_network": boolean(true), "set_here": held})
	cfg := planRow(map[string]tftypes.Value{"check_network": boolean(true), "follows_default": strList("auto_dispatch")})
	text := warningText(repoPlan(t, f, state, cfg, state))
	if !strings.Contains(text, "auto_dispatch") || !strings.Contains(text, "puts it back on the organization default for every row of acme/api") {
		t.Fatalf("the warning is:\n%s", text)
	}
}

func TestAWriteOfASettingOfOneRowWarnsNothing(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	state := planRow(map[string]tftypes.Value{"check_network": boolean(false), "fix_runner": str("platform")})
	cfg := planRow(map[string]tftypes.Value{"check_network": boolean(true), "fix_runner": str("platform")})
	if d := repoPlan(t, f, state, cfg, cfg); len(d) != 0 {
		t.Fatalf("a write that reaches one row warned %v", d)
	}
	if d := repoPlan(t, f, state, state, state); len(d) != 0 {
		t.Fatalf("a plan with no change warned %v", d)
	}
}

// A platform before v0.482.0 never says which settings a row holds, so it
// ignores follows_default; the plan says it will be ignored.
func TestFollowsDefaultOnAPlatformWithoutSetHereWarnsItIsIgnored(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	state := planRow(map[string]tftypes.Value{"check_network": boolean(true)})
	cfg := planRow(map[string]tftypes.Value{"follows_default": strList("check_network")})
	text := warningText(repoPlan(t, f, state, cfg, state))
	if !strings.Contains(text, "ignores follows_default") || !strings.Contains(text, "v0.482.0") {
		t.Fatalf("the warning is:\n%s", text)
	}
	held := tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, []tftypes.Value{str("check_network")})
	withSetHere := planRow(map[string]tftypes.Value{"check_network": boolean(true), "set_here": held})
	if text := warningText(repoPlan(t, f, withSetHere, cfg, withSetHere)); strings.Contains(text, "ignores follows_default") {
		t.Fatalf("a platform that answers set_here was told its follows_default is ignored:\n%s", text)
	}
}
func strList(items ...string) tftypes.Value {
	vals := make([]tftypes.Value, 0, len(items))
	for _, s := range items {
		vals = append(vals, str(s))
	}
	return tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, vals)
}
