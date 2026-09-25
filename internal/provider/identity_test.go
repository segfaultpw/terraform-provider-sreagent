package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/segfaultpw/terraform-provider-sreagent/internal/fakefacade"
	"github.com/segfaultpw/terraform-provider-sreagent/internal/specs"
)

func TestServiceBindingImportsByEscapedIDAndByIdentity(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	cfg := providerBlock(f.URL) + "resource \"sreagent_service_binding\" \"b\" {\n  service = \"pay ments?\"\n  environment = \"prod\"\n  log_groups = [\"/aws/x\"]\n}\n"
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{Config: cfg, Check: resource.TestCheckResourceAttr("sreagent_service_binding.b", "id", "pay%20ments%3F?environment=prod")},
			{ResourceName: "sreagent_service_binding.b", ImportState: true, ImportStateVerify: true, Config: cfg},
			{ResourceName: "sreagent_service_binding.b", ImportState: true, ImportStateKind: resource.ImportBlockWithResourceIdentity, Config: cfg},
		},
	})
}
