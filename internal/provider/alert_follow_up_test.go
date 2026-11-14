package provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/segfaultpw/terraform-provider-sreagent/internal/engine"
	"github.com/segfaultpw/terraform-provider-sreagent/internal/fakefacade"
	"github.com/segfaultpw/terraform-provider-sreagent/internal/specs"
)

const followUp = "data.sreagent_alert_follow_up_settings.f"

func TestAlertFollowUpSettingsAreReadWholeAndNeverWritten(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	f.Mutate("alert_follow_up_settings", "alert_follow_up_settings", func(r map[string]any) {
		r["enabled"] = true
		r["still_active_enabled"] = false
		r["informative_expire_hours"] = 24
		r["resolve_modes"] = map[string]any{"critical": "suggest", "high": "suggest", "medium": "auto"}
		r["ages"] = map[string]any{"critical": map[string]any{"check": 30, "nudge": 60}}
		r["defaults"] = false
		r["demoted"] = []any{map[string]any{"kind_key": "prometheus/HighLatency", "alerts": []any{"a1", "a2"}}}
		r["informative_kinds"] = []any{map[string]any{"id": "k1", "label": "Nightly job"}}
		r["updated_at"] = "2026-11-12T10:00:00Z"
		r["week"] = map[string]any{"nudges": 3, "cards": 1, "escalations": 0}
	})
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config: providerBlock(f.URL) + "data \"sreagent_alert_follow_up_settings\" \"f\" {}\noutput \"mode\" {\n  value = jsondecode(data.sreagent_alert_follow_up_settings.f.resolve_modes).critical\n}\noutput \"kind\" {\n  value = jsondecode(data.sreagent_alert_follow_up_settings.f.demoted)[0].kind_key\n}\n",
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr(followUp, "enabled", "true"),
				resource.TestCheckResourceAttr(followUp, "still_active_enabled", "false"),
				resource.TestCheckResourceAttr(followUp, "informative_expire_hours", "24"),
				resource.TestCheckResourceAttr(followUp, "defaults", "false"),
				resource.TestCheckResourceAttr(followUp, "updated_at", "2026-11-12T10:00:00Z"),
				resource.TestCheckResourceAttr(followUp, "configured", "true"),
				resource.TestCheckResourceAttrSet(followUp, "ages"),
				resource.TestCheckResourceAttrSet(followUp, "week"),
				resource.TestCheckResourceAttrSet(followUp, "informative_kinds"),
				resource.TestCheckOutput("mode", "suggest"),
				resource.TestCheckOutput("kind", "prometheus/HighLatency"),
				func(*terraform.State) error {
					for _, m := range []string{"POST", "PUT", "DELETE"} {
						if n := f.Calls(m, "alert_follow_up_settings"); n != 0 {
							return fmt.Errorf("a data source sent %d %s requests", n, m)
						}
					}
					return nil
				},
			),
		}},
	})
}

// An organization that never switched the follow-up on reads the platform's
// defaults: the row answers nulls for what it does not hold.
func TestAlertFollowUpSettingsReadTheDefaultsAnOrganizationNeverChanged(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	f.Mutate("alert_follow_up_settings", "alert_follow_up_settings", func(r map[string]any) {
		r["enabled"] = false
		r["still_active_enabled"] = false
		r["informative_expire_hours"] = nil
		r["resolve_modes"] = nil
		r["ages"] = nil
		r["defaults"] = true
		r["updated_at"] = nil
	})
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config: providerBlock(f.URL) + "data \"sreagent_alert_follow_up_settings\" \"f\" {}\n",
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr(followUp, "enabled", "false"),
				resource.TestCheckResourceAttr(followUp, "defaults", "true"),
				resource.TestCheckNoResourceAttr(followUp, "updated_at"),
				resource.TestCheckNoResourceAttr(followUp, "informative_expire_hours"),
			),
		}},
	})
}

// Every organization has the setting, so a 404 is a platform older than it: an
// error naming the release, never a read of nothing.
func TestAlertFollowUpSettingsOnAPlatformWithoutThemIsAnError(t *testing.T) {
	var older []engine.Spec
	for _, s := range specs.All() {
		if s.Key != "alert_follow_up_settings" {
			older = append(older, s)
		}
	}
	f := fakefacade.New(t, older)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config:      providerBlock(f.URL) + "data \"sreagent_alert_follow_up_settings\" \"f\" {}\n",
			ExpectError: regexp.MustCompile(`(?s)platform does not serve alert_follow_up_settings.*v0\.489\.0 or later`),
		}},
	})
}

func TestNoAlertFollowUpResourceExists(t *testing.T) {
	for _, s := range specs.All() {
		if (s.Key == "alert_follow_up_settings" || s.Key == "alert_informative_kinds") && s.Lifecycle {
			t.Errorf("%s is a resource: the platform publishes no Terraform type for it", s.Key)
		}
	}
}

const kinds = "data.sreagent_alert_informative_kinds.k"

func seedKinds(f *fakefacade.Fake) {
	for _, k := range []map[string]any{
		{"id": "k1", "label": "Nightly backup", "kind_key": "alertmanager/NightlyBackup", "source": "alertmanager", "matchers": map[string]any{"service": "db", "cluster": "prod"}, "status": "approved", "action": "expire_after", "expire_after_hours": 24, "expire_after_allowed": true, "proposed_by": "Ana", "proposed_at": "2026-11-10T10:00:00Z", "decided_by": "Ben", "decided_at": "2026-11-10T11:00:00Z", "retired_reason": nil, "version": "v1", "open_alerts": 2},
		{"id": "k2", "label": "Canary", "kind_key": "grafana/Canary", "source": "grafana", "matchers": map[string]any{"service": "web"}, "status": "proposed", "action": "resolve_when_cleared", "expire_after_hours": 0, "expire_after_allowed": false, "proposed_by": "a former member", "proposed_at": "2026-11-11T10:00:00Z", "decided_by": nil, "decided_at": nil, "retired_reason": nil, "version": "v1", "open_alerts": 1},
		{"id": "k3", "label": "Old noise", "kind_key": "alertmanager/Old", "source": "alertmanager", "matchers": map[string]any{}, "status": "retired", "action": "expire_after", "expire_after_hours": 12, "expire_after_allowed": true, "proposed_by": "Ana", "proposed_at": "2026-10-10T10:00:00Z", "decided_by": "Ben", "decided_at": "2026-10-10T11:00:00Z", "retired_reason": "the alert was fixed", "version": "v3", "open_alerts": 0},
	} {
		f.Seed("alert_informative_kinds", k)
	}
}

func TestAlertInformativeKindsListsProposalsAndApprovedByDefault(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	seedKinds(f)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config: providerBlock(f.URL) + "data \"sreagent_alert_informative_kinds\" \"k\" {}\noutput \"cluster\" {\n  value = jsondecode(data.sreagent_alert_informative_kinds.k.items[0].matchers).cluster\n}\noutput \"approved\" {\n  value = jsondecode(data.sreagent_alert_informative_kinds.k.counts).approved\n}\n",
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr(kinds, "items.#", "2"),
				resource.TestCheckResourceAttr(kinds, "items.0.id", "k1"),
				resource.TestCheckResourceAttr(kinds, "items.0.label", "Nightly backup"),
				resource.TestCheckResourceAttr(kinds, "items.0.status", "approved"),
				resource.TestCheckResourceAttr(kinds, "items.0.action", "expire_after"),
				resource.TestCheckResourceAttr(kinds, "items.0.expire_after_hours", "24"),
				resource.TestCheckResourceAttr(kinds, "items.0.expire_after_allowed", "true"),
				resource.TestCheckResourceAttr(kinds, "items.0.open_alerts", "2"),
				resource.TestCheckResourceAttr(kinds, "items.0.decided_by", "Ben"),
				resource.TestCheckNoResourceAttr(kinds, "items.0.retired_reason"),
				resource.TestCheckResourceAttr(kinds, "items.1.id", "k2"),
				resource.TestCheckResourceAttr(kinds, "items.1.expire_after_allowed", "false"),
				resource.TestCheckNoResourceAttr(kinds, "items.1.decided_at"),
				resource.TestCheckResourceAttr(kinds, "items.1.proposed_by", "a former member"),
				resource.TestCheckOutput("cluster", "prod"),
				resource.TestCheckOutput("approved", "1"),
				resource.TestCheckResourceAttrSet(kinds, "limits"),
				resource.TestCheckResourceAttr(kinds, "truncated", "false"),
			),
		}},
	})
}

func TestAlertInformativeKindsFilterByStatus(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	seedKinds(f)
	cases := map[string]struct {
		args string
		want string
		ids  []string
	}{
		"retired": {"status = \"retired\"", "1", []string{"k3"}},
		"all":     {"status = \"all\"", "3", []string{"k1", "k2", "k3"}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			checks := []resource.TestCheckFunc{resource.TestCheckResourceAttr(kinds, "items.#", c.want)}
			for i, id := range c.ids {
				checks = append(checks, resource.TestCheckResourceAttr(kinds, fmt.Sprintf("items.%d.id", i), id))
			}
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: factories,
				Steps: []resource.TestStep{{
					Config: providerBlock(f.URL) + "data \"sreagent_alert_informative_kinds\" \"k\" {\n  " + c.args + "\n}\n",
					Check:  resource.ComposeTestCheckFunc(checks...),
				}},
			})
		})
	}
}

func TestAlertInformativeKindsRefuseAStatusThePlatformDoesNotList(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config:      providerBlock(f.URL) + "data \"sreagent_alert_informative_kinds\" \"k\" {\n  status = \"archived\"\n}\n",
			ExpectError: regexp.MustCompile(`(?s)status.*value must be one of.*"proposed".*"approved".*"declined".*"retired".*"all"`),
		}},
	})
	if n := f.Calls("GET", "alert_informative_kinds"); n != 0 {
		t.Fatalf("a refused configuration still sent %d GETs", n)
	}
}

func TestAlertInformativeKindsAnEmptyOrganizationReadsAnEmptyList(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config: providerBlock(f.URL) + "data \"sreagent_alert_informative_kinds\" \"k\" {}\n",
			Check:  resource.TestCheckResourceAttr(kinds, "items.#", "0"),
		}},
	})
}
