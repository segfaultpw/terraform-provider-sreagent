package specs

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/segfaultpw/terraform-provider-sreagent/internal/engine"
)

// bedrockRegions are the regions the platform accepts for a Bedrock provider:
// only those every account has by default, since the role is assumed through
// the region's own STS endpoint.
var bedrockRegions = []string{
	"us-east-1", "us-east-2", "us-west-1", "us-west-2",
	"ca-central-1", "sa-east-1",
	"eu-west-1", "eu-west-2", "eu-west-3", "eu-central-1", "eu-north-1",
	"ap-south-1", "ap-southeast-1", "ap-southeast-2", "ap-northeast-1", "ap-northeast-2", "ap-northeast-3",
	"us-gov-west-1", "us-gov-east-1",
}

// roleARNShape is the platform's: an IAM role ARN in the aws or aws-us-gov
// partition, with IAM's own path and name alphabet.
var roleARNShape = regexp.MustCompile(`^arn:(aws|aws-us-gov):iam::[0-9]{12}:role/[A-Za-z0-9_+=,.@/-]+$`)

const roleARNMax = 2048

func partitionOfRegion(region string) string {
	if strings.HasPrefix(region, "us-gov-") {
		return "aws-us-gov"
	}
	return "aws"
}

// aiProviderChecks mirrors what the platform refuses for a Bedrock row: it
// authenticates only by assuming role_arn in region, so both are required, it
// takes no API key, and no other provider takes either field.
func aiProviderChecks(c engine.Config) []engine.Issue {
	provider, state := c.String("provider")
	if state != engine.Present {
		return nil
	}
	role, roleState := c.String("role_arn")
	region, regionState := c.String("region")
	var issues []engine.Issue
	if provider != "bedrock" {
		for _, f := range []struct {
			name  string
			state engine.Presence
		}{{"role_arn", roleState}, {"region", regionState}} {
			if f.state == engine.Present {
				issues = append(issues, engine.Issue{Attr: f.name, Summary: "Bedrock only",
					Detail: fmt.Sprintf("%s applies to a provider_type of bedrock; this provider is %q. Remove it.", f.name, provider)})
			}
		}
		return issues
	}
	if roleState == engine.Absent {
		issues = append(issues, engine.Issue{Attr: "role_arn", Summary: "Missing role_arn",
			Detail: "A bedrock provider authenticates by assuming an IAM role in your own AWS account, so role_arn is required."})
	}
	if regionState == engine.Absent {
		issues = append(issues, engine.Issue{Attr: "region", Summary: "Missing region",
			Detail: "A bedrock provider calls its models in one region, so region is required, such as us-east-1."})
	}
	if _, keyState := c.String("api_key"); keyState == engine.Present {
		issues = append(issues, engine.Issue{Attr: "api_key", Summary: "Bedrock takes no API key",
			Detail: "A bedrock provider authenticates by assuming the IAM role in role_arn. Remove api_key_wo and api_key_wo_version."})
	}
	roleOK, regionOK := false, false
	if roleState == engine.Present {
		switch {
		case utf8.RuneCountInString(role) > roleARNMax:
			issues = append(issues, engine.Issue{Attr: "role_arn", Summary: "role_arn too long",
				Detail: fmt.Sprintf("An IAM role ARN is at most %d characters.", roleARNMax)})
		case !roleARNShape.MatchString(role):
			issues = append(issues, engine.Issue{Attr: "role_arn", Summary: "Not an IAM role ARN",
				Detail: fmt.Sprintf("%q must be an IAM role ARN such as arn:aws:iam::123456789012:role/sre-agent-bedrock, with no surrounding whitespace.", role)})
		default:
			roleOK = true
		}
	}
	if regionState == engine.Present {
		if slices.Contains(bedrockRegions, region) {
			regionOK = true
		} else {
			issues = append(issues, engine.Issue{Attr: "region", Summary: "Not a Bedrock region",
				Detail: fmt.Sprintf("%q is not a region the platform calls Bedrock in, written in lowercase. One of: %s.", region, strings.Join(bedrockRegions, ", "))})
		}
	}
	// STS cannot assume a role across partitions, so the platform refuses a pair that disagrees.
	if roleOK && regionOK {
		if want := partitionOfRegion(region); !strings.HasPrefix(role, "arn:"+want+":") {
			issues = append(issues, engine.Issue{Attr: "role_arn", Summary: "Wrong partition",
				Detail: fmt.Sprintf("role_arn must be in the %s partition, the partition of the region %s.", want, region)})
		}
	}
	return issues
}

// AIProvider mirrors x-sreagent-resources.ai_providers. A provider inherited
// from a parent organization reads as not found.
var AIProvider = engine.Spec{
	Key: "ai_providers", TypeName: "ai_provider", ListName: "ai_providers",
	Description: "A model provider the organization brings. A provider inherited from a parent organization is not visible here.",
	Shape:       engine.Generated, Lifecycle: true,
	Checks: []func(engine.Config) []engine.Issue{aiProviderChecks},
	Attrs: []engine.Attr{
		{Name: "name", Kind: engine.String, Required: true, Description: "Unique within the organization."},
		{Name: "provider", TFName: "provider_type", Kind: engine.String, Required: true, OneOf: []string{"anthropic", "openai", "openrouter", "ollama", "bedrock", "gemini", "portkey", "langchain"}, Description: "The provider."},
		{Name: "model", Kind: engine.String, Clearable: true, Description: "The default model."},
		{Name: "base_url", Kind: engine.String, Clearable: true, Description: "A custom endpoint."},
		{Name: "enabled", Kind: engine.Bool, Description: "Whether it is routed to."},
		{Name: "priority", Kind: engine.Int, Description: "Routing order."},
		{Name: "auto_upgrade_model", Kind: engine.Bool, Description: "Follow the provider's newest model."},
		{Name: "model_overrides", Kind: engine.JSON, MergedObject: true, Description: "A model per purpose, as jsonencode({...}). \"auto\" hands a purpose to the platform's own model selection. A purpose removed from the map is cleared on the platform."},
		{Name: "max_tokens", Kind: engine.Int, Clearable: true, NullMeans: "4096", Description: "Output token cap; unset means 4096."},
		{Name: "temperature", Kind: engine.Float, Clearable: true, NullMeans: "0.7", Description: "Sampling temperature; a new provider starts at 0.7."},
		{Name: "timeout_ms", Kind: engine.Int, Clearable: true, NullMeans: "60000", Description: "Request timeout; a new provider starts at 60000."},
		{Name: "rate_limit_rpm", Kind: engine.Int, Clearable: true, Description: "Requests per minute."},
		{Name: "role_arn", Kind: engine.String, Clearable: true, Description: "Bedrock only, and required for it: the ARN of the IAM role in your own AWS account that SRE Agent assumes to call Bedrock. Its trust policy allows the principal and the ExternalId from data.sreagent_aws_external_id. Not a secret. Refused for every other provider."},
		{Name: "region", Kind: engine.String, Clearable: true, Description: "Bedrock only, and required for it: the region the models are called in, such as us-east-1 (lowercase, one of the regions the platform supports). Refused for every other provider."},
		{Name: "api_key", Kind: engine.String, Secret: true, DroppedWhenAttr: "provider", DroppedWhenValue: "bedrock", Description: "The provider's API key. Not for bedrock, which has no key and is refused one: it takes role_arn and region; switching a provider to bedrock discards the key it held."},
	},
}
