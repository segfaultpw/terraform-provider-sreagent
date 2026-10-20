package provider

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"

	"github.com/segfaultpw/terraform-provider-sreagent/internal/fakefacade"
	"github.com/segfaultpw/terraform-provider-sreagent/internal/specs"
)

const aiAddr = "sreagent_ai_provider.p"

var aiVersions = []tfversion.TerraformVersionCheck{tfversion.SkipBelow(tfversion.Version1_12_0)}

const (
	bedrockRole   = "arn:aws:iam::123456789012:role/sre-agent-bedrock"
	bedrockRole2  = "arn:aws:iam::123456789012:role/team/sre-agent-bedrock-2"
	bedrockSignIn = "  name = \"bedrock\"\n  provider_type = \"bedrock\"\n"
)

func aiConfig(url, body string) string {
	return providerBlock(url) + "resource \"sreagent_ai_provider\" \"p\" {\n" + body + "}\n"
}

// aiBodyField checks a field of the latest write; want nil means the body names it with a null.
func aiBodyField(f *fakefacade.Fake, method, key string, want any) resource.TestCheckFunc {
	return func(*terraform.State) error {
		body := f.LastBody(method, "ai_providers")
		if body == nil {
			return fmt.Errorf("no %s reached the platform", method)
		}
		got, named := body[key]
		if !named || got != want {
			return fmt.Errorf("the %s body has %s = %v (named: %v), want %v (body %v)", method, key, got, named, want, body)
		}
		return nil
	}
}

// aiBodyOmits checks that the latest write does not name key.
func aiBodyOmits(f *fakefacade.Fake, method, key string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		body := f.LastBody(method, "ai_providers")
		if body == nil {
			return fmt.Errorf("no %s reached the platform", method)
		}
		if _, named := body[key]; named {
			return fmt.Errorf("the %s body names %s, which it must not (body %v)", method, key, body)
		}
		return nil
	}
}

// A Bedrock row is created with its role and region and no key, a region
// change applies, and switching the provider away removes both from state and
// sends null for each, which is how the platform clears them.
func TestAIProviderBedrockLifecycle(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	bedrock := func(role, region string) string {
		return aiConfig(f.URL, bedrockSignIn+fmt.Sprintf("  role_arn = %q\n  region = %q\n", role, region))
	}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		TerraformVersionChecks:   aiVersions,
		Steps: []resource.TestStep{
			{
				Config: bedrock(bedrockRole, "us-east-1"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(aiAddr, "role_arn", bedrockRole),
					resource.TestCheckResourceAttr(aiAddr, "region", "us-east-1"),
					resource.TestCheckResourceAttr(aiAddr, "api_key_set", "false"),
					aiBodyField(f, "POST", "role_arn", bedrockRole),
					aiBodyField(f, "POST", "region", "us-east-1"),
					aiBodyOmits(f, "POST", "api_key"),
				),
			},
			{Config: bedrock(bedrockRole, "us-east-1"), PlanOnly: true},
			{
				Config: bedrock(bedrockRole2, "eu-west-1"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(aiAddr, "role_arn", bedrockRole2),
					resource.TestCheckResourceAttr(aiAddr, "region", "eu-west-1"),
					aiBodyField(f, "PUT", "region", "eu-west-1"),
				),
			},
			{ResourceName: aiAddr, ImportState: true, ImportStateVerify: true, Config: bedrock(bedrockRole2, "eu-west-1")},
			{
				Config: aiConfig(f.URL, "  name = \"bedrock\"\n  provider_type = \"anthropic\"\n  api_key_wo = \"sk-ant-unit\"\n  api_key_wo_version = 1\n"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr(aiAddr, "role_arn"),
					resource.TestCheckNoResourceAttr(aiAddr, "region"),
					resource.TestCheckResourceAttr(aiAddr, "api_key_set", "true"),
					aiBodyField(f, "PUT", "role_arn", nil),
					aiBodyField(f, "PUT", "region", nil),
				),
			},
			{
				Config:   aiConfig(f.URL, "  name = \"bedrock\"\n  provider_type = \"anthropic\"\n  api_key_wo = \"sk-ant-unit\"\n  api_key_wo_version = 1\n"),
				PlanOnly: true,
			},
		},
	})
}

// A row moved to Bedrock from a keyed provider loses the key on the platform,
// so the plan says api_key_set becomes false rather than failing the apply as
// an inconsistent result.
func TestAIProviderSwitchToBedrockDiscardsTheKey(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		TerraformVersionChecks:   aiVersions,
		Steps: []resource.TestStep{
			{
				Config: aiConfig(f.URL, "  name = \"main\"\n  provider_type = \"anthropic\"\n  api_key_wo = \"sk-ant-unit\"\n  api_key_wo_version = 1\n"),
				Check:  resource.TestCheckResourceAttr(aiAddr, "api_key_set", "true"),
			},
			{
				Config: aiConfig(f.URL, "  name = \"main\"\n  provider_type = \"bedrock\"\n  role_arn = \""+bedrockRole+"\"\n  region = \"us-west-2\"\n"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(aiAddr, "api_key_set", "false"),
					resource.TestCheckResourceAttr(aiAddr, "role_arn", bedrockRole),
				),
			},
			{
				Config:   aiConfig(f.URL, "  name = \"main\"\n  provider_type = \"bedrock\"\n  role_arn = \""+bedrockRole+"\"\n  region = \"us-west-2\"\n"),
				PlanOnly: true,
			},
		},
	})
}

// What the platform refuses for a Bedrock row and for every other provider,
// refused at plan with nothing written.
func TestAIProviderRefusesAtPlanWhatThePlatformRefuses(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	other := "  name = \"main\"\n  provider_type = \"openai\"\n"
	for _, c := range []struct{ name, body, want string }{
		{"bedrock without a role", bedrockSignIn + "  region = \"us-east-1\"\n", `(?s)Missing role_arn.*role_arn is required`},
		{"bedrock without a region", bedrockSignIn + "  role_arn = \"" + bedrockRole + "\"\n", `(?s)Missing region.*region is required`},
		{"bedrock with neither", bedrockSignIn, `(?s)Missing role_arn.*Missing region`},
		{"bedrock with a key", bedrockSignIn + "  role_arn = \"" + bedrockRole + "\"\n  region = \"us-east-1\"\n  api_key_wo = \"sk-unit\"\n  api_key_wo_version = 1\n", `(?s)Bedrock takes no API key`},
		{"a role that is not an ARN", bedrockSignIn + "  role_arn = \"sre-agent-bedrock\"\n  region = \"us-east-1\"\n", `(?s)Not an IAM role ARN`},
		{"a user ARN", bedrockSignIn + "  role_arn = \"arn:aws:iam::123456789012:user/bob\"\n  region = \"us-east-1\"\n", `(?s)Not an IAM role ARN`},
		{"a padded ARN", bedrockSignIn + "  role_arn = \" " + bedrockRole + "\"\n  region = \"us-east-1\"\n", `(?s)Not an IAM role ARN`},
		{"a blank ARN", bedrockSignIn + "  role_arn = \"\"\n  region = \"us-east-1\"\n", `(?s)Not an IAM role ARN`},
		{"an over-long ARN", bedrockSignIn + "  role_arn = \"arn:aws:iam::123456789012:role/" + strings.Repeat("a", 2048) + "\"\n  region = \"us-east-1\"\n", `(?s)role_arn too long`},
		{"a region Bedrock is not run in", bedrockSignIn + "  role_arn = \"" + bedrockRole + "\"\n  region = \"mars-east-1\"\n", `(?s)Not a Bedrock region`},
		{"an uppercase region", bedrockSignIn + "  role_arn = \"" + bedrockRole + "\"\n  region = \"US-EAST-1\"\n", `(?s)Not a Bedrock region`},
		{"a partition that disagrees with the region", bedrockSignIn + "  role_arn = \"arn:aws-us-gov:iam::123456789012:role/r\"\n  region = \"us-east-1\"\n", `(?s)Wrong partition.*aws partition`},
		{"a commercial role in a GovCloud region", bedrockSignIn + "  role_arn = \"" + bedrockRole + "\"\n  region = \"us-gov-west-1\"\n", `(?s)Wrong partition.*aws-us-gov partition`},
		{"openai with a region", other + "  region = \"us-east-1\"\n", `(?s)Bedrock only.*region applies to a provider_type of bedrock`},
		{"openai with a role", other + "  role_arn = \"" + bedrockRole + "\"\n", `(?s)Bedrock only.*role_arn applies to a provider_type of bedrock`},
		{"anthropic with an empty region", "  name = \"main\"\n  provider_type = \"anthropic\"\n  region = \"\"\n", `(?s)Bedrock only`},
	} {
		t.Run(c.name, func(t *testing.T) {
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: factories,
				TerraformVersionChecks:   aiVersions,
				Steps:                    []resource.TestStep{{Config: aiConfig(f.URL, c.body), PlanOnly: true, ExpectError: regexp.MustCompile(c.want)}},
			})
		})
	}
	if f.Rows("ai_providers") != 0 || f.Calls("POST", "ai_providers") != 0 {
		t.Fatal("a refused plan must write nothing")
	}
}

// A value known only at apply is never refused at plan: a role created in the
// same apply has no ARN yet.
func TestAIProviderBedrockRoleKnownOnlyAtApply(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	cfg := providerBlock(f.URL) + fmt.Sprintf(`
resource "terraform_data" "role" {
  input = %q
}

resource "sreagent_ai_provider" "p" {
  name          = "bedrock"
  provider_type = "bedrock"
  role_arn      = terraform_data.role.output
  region        = "us-east-1"
}
`, bedrockRole)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		TerraformVersionChecks:   aiVersions,
		Steps: []resource.TestStep{{
			Config: cfg,
			Check:  resource.TestCheckResourceAttr(aiAddr, "role_arn", bedrockRole),
		}},
	})
}
