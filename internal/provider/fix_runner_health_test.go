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

const runnerHealth = "data.sreagent_fix_runner_health.h"

func TestFixRunnerHealthIsReadWholeAndNeverWritten(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	f.Mutate("fix_runner_health", "fix_runner_health", func(r map[string]any) {
		r["state"] = "refusing"
		r["since"] = "2026-11-15T08:00:00Z"
		r["consecutive_refusals"] = 3
		r["last_refusal"] = map[string]any{"class": "quota", "class_phrase": "the AI provider's quota is used up", "model": "model-a", "status": 429, "provider_message": "quota exceeded", "at": "2026-11-15T08:00:00Z"}
		r["waiting"] = 2
		r["degraded_since"] = nil
		r["hold_while_provider_down"] = true
		r["next_check_at"] = "2026-11-15T09:00:00Z"
	})
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config: providerBlock(f.URL) + "data \"sreagent_fix_runner_health\" \"h\" {}\noutput \"class\" {\n  value = jsondecode(data.sreagent_fix_runner_health.h.last_refusal).class\n}\n",
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr(runnerHealth, "state", "refusing"),
				resource.TestCheckResourceAttr(runnerHealth, "since", "2026-11-15T08:00:00Z"),
				resource.TestCheckResourceAttr(runnerHealth, "consecutive_refusals", "3"),
				resource.TestCheckResourceAttr(runnerHealth, "waiting", "2"),
				resource.TestCheckResourceAttr(runnerHealth, "hold_while_provider_down", "true"),
				resource.TestCheckResourceAttr(runnerHealth, "next_check_at", "2026-11-15T09:00:00Z"),
				resource.TestCheckNoResourceAttr(runnerHealth, "degraded_since"),
				resource.TestCheckResourceAttr(runnerHealth, "configured", "true"),
				resource.TestCheckOutput("class", "quota"),
				func(*terraform.State) error {
					for _, m := range []string{"POST", "PUT", "DELETE"} {
						if n := f.Calls(m, "fix_runner_health"); n != 0 {
							return fmt.Errorf("a data source sent %d %s requests", n, m)
						}
					}
					return nil
				},
			),
		}},
	})
}

// With no runner registered the platform answers state none and nulls.
func TestFixRunnerHealthWithNoRunnerReadsNone(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	f.Mutate("fix_runner_health", "fix_runner_health", func(r map[string]any) {
		r["state"] = "none"
		r["since"] = nil
		r["consecutive_refusals"] = 0
		r["last_refusal"] = nil
		r["waiting"] = 0
		r["degraded_since"] = nil
		r["hold_while_provider_down"] = true
		r["next_check_at"] = nil
	})
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config: providerBlock(f.URL) + "data \"sreagent_fix_runner_health\" \"h\" {}\n",
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr(runnerHealth, "state", "none"),
				resource.TestCheckResourceAttr(runnerHealth, "waiting", "0"),
				resource.TestCheckResourceAttr(runnerHealth, "hold_while_provider_down", "true"),
				resource.TestCheckNoResourceAttr(runnerHealth, "last_refusal"),
				resource.TestCheckNoResourceAttr(runnerHealth, "since"),
			),
		}},
	})
}

// Every organization with the runner in its plan has the reading, so a 404 is a
// platform older than the resource: an error naming the release, never a read of nothing.
func TestFixRunnerHealthOnAPlatformWithoutItIsAnError(t *testing.T) {
	var older []engine.Spec
	for _, s := range specs.All() {
		if s.Key != "fix_runner_health" {
			older = append(older, s)
		}
	}
	f := fakefacade.New(t, older)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config:      providerBlock(f.URL) + "data \"sreagent_fix_runner_health\" \"h\" {}\n",
			ExpectError: regexp.MustCompile(`(?s)platform does not serve fix_runner_health.*v0\.503\.0 or later`),
		}},
	})
}

// A plan without the GitHub integration or the self-hosted runner is answered
// 403 with the platform's own sentence, which the read shows.
func TestFixRunnerHealthRefusedByThePlanShowsThePlatformsSentence(t *testing.T) {
	for name, c := range map[string]struct{ sentence, want string }{
		"no self-hosted runner": {"A fix runner on your own machine is not included in your plan.", `(?s)answered 403.*fix\s+runner\s+on\s+your\s+own\s+machine\s+is\s+not\s+included\s+in\s+your\s+plan`},
		"no GitHub":             {"GitHub is not included in your plan.", `(?s)answered 403.*GitHub\s+is\s+not\s+included\s+in\s+your\s+plan`},
	} {
		t.Run(name, func(t *testing.T) {
			f := fakefacade.New(t, specs.All())
			f.RefusePlan("fix_runner_health", c.sentence)
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: factories,
				Steps: []resource.TestStep{{
					Config:      providerBlock(f.URL) + "data \"sreagent_fix_runner_health\" \"h\" {}\n",
					ExpectError: regexp.MustCompile(c.want),
				}},
			})
		})
	}
}

func TestNoFixRunnerHealthResourceExists(t *testing.T) {
	for _, s := range specs.All() {
		if s.Key == "fix_runner_health" && s.Lifecycle {
			t.Errorf("%s is a resource: the platform publishes no Terraform type for it", s.Key)
		}
	}
}
