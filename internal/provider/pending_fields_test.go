package provider

import (
	"fmt"
	"reflect"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/segfaultpw/terraform-provider-sreagent/internal/fakefacade"
	"github.com/segfaultpw/terraform-provider-sreagent/internal/specs"
)

// The fields the platform published ahead of the provider release that declares
// them: the card event settings of a webhook target, four fields of an SLI and the
// board summary switch. Each is sent only when the block states it, read back, and
// left alone when the block leaves it out.

func TestOutboundConfigCardEventKindsAndTextRoundTrip(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config: outboundConfigConfig(f.URL, "  card_events = true\n  card_event_kinds = [\"moved\", \"approvals\", \"closed\"]\n  card_event_text = true"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(outboundConfig, "card_event_kinds.#", "3"),
					resource.TestCheckResourceAttr(outboundConfig, "card_event_text", "true"),
					func(*terraform.State) error {
						body := f.LastBody("POST", "outbound_configs")
						if body["card_event_text"] != true {
							return fmt.Errorf("card_event_text sent as %v", body["card_event_text"])
						}
						if kinds, _ := body["card_event_kinds"].([]any); len(kinds) != 3 {
							return fmt.Errorf("card_event_kinds sent as %v", body["card_event_kinds"])
						}
						return nil
					},
				),
			},
			// The platform stores the set in its own order: the plan after it is empty whatever
			// order the block wrote.
			{Config: outboundConfigConfig(f.URL, "  card_events = true\n  card_event_kinds = [\"moved\", \"approvals\", \"closed\"]\n  card_event_text = true"), ConfigPlanChecks: emptyPlan},
			{
				// Left out, the settings keep what the platform holds and are not sent.
				Config: outboundConfigConfig(f.URL, "  card_events = true\n  enabled = false"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(outboundConfig, "card_event_text", "true"),
					resource.TestCheckResourceAttr(outboundConfig, "card_event_kinds.#", "3"),
					func(*terraform.State) error {
						body := f.LastBody("PUT", "outbound_configs")
						for _, k := range []string{"card_event_kinds", "card_event_text"} {
							if _, sent := body[k]; sent {
								return fmt.Errorf("an unstated %s was sent: %v", k, body)
							}
						}
						return nil
					},
				),
			},
			{
				Config: outboundConfigConfig(f.URL, "  card_events = true\n  enabled = false\n  card_event_kinds = [\"approvals\"]\n  card_event_text = false"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(outboundConfig, "card_event_kinds.#", "1"),
					resource.TestCheckResourceAttr(outboundConfig, "card_event_text", "false"),
				),
			},
		},
	})
}

// A kind the platform does not list is refused at plan, before anything is sent.
func TestOutboundConfigRefusesAnUnknownCardEventKind(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config:      outboundConfigConfig(f.URL, "  card_events = true\n  card_event_kinds = [\"everything\"]"),
			PlanOnly:    true,
			ExpectError: regexp.MustCompile(`(?s)card_event_kinds.*must\s+be\s+one\s+of`),
		}},
	})
}

const sliAddr = "sreagent_sli.s"

func sliConfig(url, body string) string {
	return providerBlock(url) + fmt.Sprintf("resource \"sreagent_sli\" \"s\" {\n  name = \"checkout\"\n  sli_type = \"availability\"\n%s\n}\n", body)
}

func TestSLIEventQueriesThresholdAndUnitRoundTripAndAreLeftWhenLeftOut(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	events := "  good_events_query = \"sum(rate(ok[5m]))\"\n  total_events_query = \"sum(rate(all[5m]))\"\n  unit = \"requests\"\n"
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config: sliConfig(f.URL, events),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(sliAddr, "good_events_query", "sum(rate(ok[5m]))"),
					resource.TestCheckResourceAttr(sliAddr, "total_events_query", "sum(rate(all[5m]))"),
					resource.TestCheckResourceAttr(sliAddr, "unit", "requests"),
					func(*terraform.State) error {
						if _, sent := f.LastBody("POST", "slis")["threshold_good"]; sent {
							return fmt.Errorf("an unstated threshold_good was sent")
						}
						return nil
					},
				),
			},
			{Config: sliConfig(f.URL, events), ConfigPlanChecks: emptyPlan},
			{
				Config: sliConfig(f.URL, "  threshold_good = 99.5\n  unit = \"ms\"\n"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(sliAddr, "threshold_good", "99.5"),
					// Left out, the event queries keep what the platform holds and are not sent.
					resource.TestCheckResourceAttr(sliAddr, "good_events_query", "sum(rate(ok[5m]))"),
					func(*terraform.State) error {
						row := f.Row("slis", mustSLIID(t, f))
						if fmt.Sprint(row["threshold_good"]) != "99.5" || row["good_events_query"] != "sum(rate(ok[5m]))" || row["total_events_query"] != "sum(rate(all[5m]))" {
							return fmt.Errorf("the platform holds %v", row)
						}
						body := f.LastBody("PUT", "slis")
						if _, sent := body["good_events_query"]; sent {
							return fmt.Errorf("an unstated good_events_query was sent: %v", body)
						}
						return nil
					},
				),
			},
			{ResourceName: sliAddr, ImportState: true, ImportStateVerify: true, Config: sliConfig(f.URL, "  threshold_good = 99.5\n  unit = \"ms\"\n")},
		},
	})
}

func mustSLIID(t *testing.T, f *fakefacade.Fake) string {
	t.Helper()
	ids := f.IDs("slis")
	if len(ids) != 1 {
		t.Fatalf("want one SLI, got %v", ids)
	}
	return ids[0]
}

func TestBoardSummaryModelEnabledRoundTripsAndIsLeftAloneWhenLeftOut(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config: orgSettingsConfig(f.URL, "  board_summary_model_enabled = true"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(orgSettings, "board_summary_model_enabled", "true"),
					func(*terraform.State) error {
						if orgRow(f)["board_summary_model_enabled"] != true {
							return fmt.Errorf("the platform holds %v", orgRow(f)["board_summary_model_enabled"])
						}
						return nil
					},
				),
			},
			{Config: orgSettingsConfig(f.URL, "  board_summary_model_enabled = true"), ConfigPlanChecks: emptyPlan},
			{
				Config: orgSettingsConfig(f.URL, "  alert_storm_threshold = 14"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(orgSettings, "board_summary_model_enabled", "true"),
					func(*terraform.State) error {
						if orgPutHas(f, "board_summary_model_enabled") {
							return fmt.Errorf("an unstated board_summary_model_enabled was sent")
						}
						return nil
					},
				),
			},
		},
	})
}

// The effective settings of a repository say which GitHub account it is read
// through, and null for a repository no account was named for.
func TestEffectiveRepoSettingsNameTheAccountEachRepositoryIsReadThrough(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	f.Reachable("acme/web", "jane/tools", "acme/orphan")
	f.ReadThrough("acme", "acme/web")
	f.ReadThrough("jane", "jane/tools")
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config: effectiveConfig(f.URL, ""),
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr(effective, "items.#", "3"),
				func(s *terraform.State) error {
					attrs := s.RootModule().Resources["data.sreagent_effective_repo_settings.all"].Primary.Attributes
					got := map[string]string{}
					for i := 0; i < 3; i++ {
						key := fmt.Sprintf("items.%d.", i)
						repo, hasAccount := attrs[key+"repo"], attrs[key+"account"]
						got[repo] = hasAccount
						if _, set := attrs[key+"account"]; !set && repo != "acme/orphan" {
							return fmt.Errorf("%s has no account in state: %v", repo, attrs)
						}
					}
					want := map[string]string{"acme/web": "acme", "jane/tools": "jane", "acme/orphan": ""}
					if !reflect.DeepEqual(got, want) {
						return fmt.Errorf("accounts %v, want %v", got, want)
					}
					return nil
				},
			),
		}},
	})
}

// The four data sources the platform serves for the board's templates, recurring
// cards and rules and for SRE Agent's settings read what the platform answers and
// send nothing but reads.

func TestTicketTemplatesAndSchedulesAreReadWholeAndNeverWritten(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	f.Seed("ticket_templates", map[string]any{
		"id": "tpl-1", "name": "Weekly review", "title": "Review the board", "description": "Walk the columns.",
		"labels": []any{"ops", "weekly"}, "priority": "medium", "team_id": nil, "service": "checkout",
		"checklist": []any{"Check blocked", "Check due dates"}, "updated_at": "2026-11-20T10:00:00Z",
	})
	f.Seed("ticket_schedules", map[string]any{
		"id": "sch-1", "name": "Every Monday", "template_id": "tpl-1", "rhythm": "weekly", "weekdays": []any{1},
		"interval_weeks": 1, "day_of_month": nil, "interval_months": nil, "hour": 9, "anchor_on": nil,
		"assignee_mode": "on_call", "assignee_user_id": nil, "oncall_schedule_id": "oc-1", "due_in_days": 2, "enabled": true,
		"description": "Every Monday at 09:00", "last_run": map[string]any{"outcome": "created", "ticket_id": "t-9"},
		"last_run_at": "2026-11-23T09:00:00Z", "next_run_at": "2026-11-30T09:00:00Z", "paused_reason": nil, "updated_at": "2026-11-20T10:00:00Z",
	})
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config: providerBlock(f.URL) + "data \"sreagent_ticket_templates\" \"t\" {}\ndata \"sreagent_ticket_schedules\" \"s\" {}\n" +
				"output \"weekday\" {\n  value = jsondecode(data.sreagent_ticket_schedules.s.items[0].weekdays)[0]\n}\n",
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr("data.sreagent_ticket_templates.t", "items.#", "1"),
				resource.TestCheckResourceAttr("data.sreagent_ticket_templates.t", "items.0.name", "Weekly review"),
				resource.TestCheckResourceAttr("data.sreagent_ticket_templates.t", "items.0.labels.#", "2"),
				resource.TestCheckResourceAttr("data.sreagent_ticket_templates.t", "items.0.checklist.1", "Check due dates"),
				resource.TestCheckResourceAttr("data.sreagent_ticket_schedules.s", "items.#", "1"),
				resource.TestCheckResourceAttr("data.sreagent_ticket_schedules.s", "items.0.rhythm", "weekly"),
				resource.TestCheckResourceAttr("data.sreagent_ticket_schedules.s", "items.0.hour", "9"),
				resource.TestCheckResourceAttr("data.sreagent_ticket_schedules.s", "items.0.template_id", "tpl-1"),
				resource.TestCheckResourceAttrSet("data.sreagent_ticket_schedules.s", "items.0.last_run"),
				resource.TestCheckOutput("weekday", "1"),
				func(*terraform.State) error {
					for _, key := range []string{"ticket_templates", "ticket_schedules"} {
						for _, m := range []string{"POST", "PUT", "DELETE"} {
							if n := f.Calls(m, key); n != 0 {
								return fmt.Errorf("a data source sent %d %s requests to %s", n, m, key)
							}
						}
					}
					return nil
				},
			),
		}},
	})
}

func TestBoardRulesAreReadWholeAndNeverWritten(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	f.Seed("board_rules", map[string]any{
		"id": "rule-1", "name": "Triage new cards", "enabled": true,
		"trigger":    map[string]any{"kind": "card_created"},
		"conditions": []any{map[string]any{"kind": "has_label", "label": "bug"}},
		"actions":    []any{map[string]any{"kind": "set_priority", "priority": "high"}},
		"version":    3, "last_run_at": nil, "updated_at": "2026-11-20T10:00:00Z",
	})
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config: providerBlock(f.URL) + "data \"sreagent_board_rules\" \"r\" {}\noutput \"action\" {\n  value = jsondecode(data.sreagent_board_rules.r.items[0].actions)[0].kind\n}\n",
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr("data.sreagent_board_rules.r", "items.#", "1"),
				resource.TestCheckResourceAttr("data.sreagent_board_rules.r", "items.0.name", "Triage new cards"),
				resource.TestCheckResourceAttr("data.sreagent_board_rules.r", "items.0.version", "3"),
				resource.TestCheckOutput("action", "set_priority"),
				func(*terraform.State) error {
					for _, m := range []string{"POST", "PUT", "DELETE"} {
						if n := f.Calls(m, "board_rules"); n != 0 {
							return fmt.Errorf("a data source sent %d %s requests", n, m)
						}
					}
					return nil
				},
			),
		}},
	})
}

func TestLeadAgentSettingsAreReadWholeAndNeverWritten(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	f.Mutate("lead_agent_settings", "lead_agent_settings", func(r map[string]any) {
		r["card_mode"] = "suggest"
		r["slack_mode"] = "off"
		r["alert_mode"] = "act"
		r["plan_approval"] = true
		r["max_wakes"] = 60
		r["max_tokens"] = 2000000
		r["max_hours"] = 168
		r["alert_paging"] = false
		r["alert_act_unattended"] = false
		r["alert_min_severity"] = "high"
		r["alert_starts_per_hour"] = 6
		r["bounds"] = map[string]any{"max_wakes": map[string]any{"min": 1, "max": 500}}
		r["defaults"] = false
		r["modes"] = []any{"off", "suggest", "act"}
		r["updated_at"] = "2026-11-24T10:00:00Z"
	})
	const addr = "data.sreagent_lead_agent_settings.l"
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config: providerBlock(f.URL) + "data \"sreagent_lead_agent_settings\" \"l\" {}\noutput \"max\" {\n  value = jsondecode(data.sreagent_lead_agent_settings.l.bounds).max_wakes.max\n}\n",
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr(addr, "card_mode", "suggest"),
				resource.TestCheckResourceAttr(addr, "alert_mode", "act"),
				resource.TestCheckResourceAttr(addr, "plan_approval", "true"),
				resource.TestCheckResourceAttr(addr, "max_tokens", "2000000"),
				resource.TestCheckResourceAttr(addr, "alert_starts_per_hour", "6"),
				resource.TestCheckResourceAttr(addr, "defaults", "false"),
				resource.TestCheckResourceAttr(addr, "configured", "true"),
				resource.TestCheckOutput("max", "500"),
				func(*terraform.State) error {
					for _, m := range []string{"POST", "PUT", "DELETE"} {
						if n := f.Calls(m, "lead_agent_settings"); n != 0 {
							return fmt.Errorf("a data source sent %d %s requests", n, m)
						}
					}
					return nil
				},
			),
		}},
	})
}
