package provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/segfaultpw/terraform-provider-sreagent/internal/fakefacade"
	"github.com/segfaultpw/terraform-provider-sreagent/internal/specs"
)

func TestSingletonAdoptsManagesOnlyWhatItNamesAndForgetsOnDestroy(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	cfg := func(body string) string {
		return providerBlock(f.URL) + fmt.Sprintf("resource \"sreagent_status_page_settings\" \"s\" {\n%s\n}\n", body)
	}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				PreConfig: func() {
					// A title set in the browser before Terraform arrives.
					f.Mutate("status_page_settings", "status_page_settings", func(r map[string]any) { r["title"] = "Acme status" })
				},
				Config: cfg("enabled = true"),
				Check:  resource.TestCheckResourceAttr("sreagent_status_page_settings.s", "title", "Acme status"),
			},
			{ResourceName: "sreagent_status_page_settings.s", ImportState: true, ImportStateId: "status_page_settings", ImportStateVerify: true, Config: cfg("enabled = true")},
			{ResourceName: "sreagent_status_page_settings.s", ImportState: true, ImportStateId: "wrong", Config: cfg("enabled = true"), ExpectError: regexp.MustCompile(`Import a singleton by its name`)},
			{
				Config:  providerBlock(f.URL),
				Destroy: false,
				Check: func(*terraform.State) error {
					if f.Row("status_page_settings", "status_page_settings")["enabled"] != true {
						return fmt.Errorf("destroying a singleton must leave the platform's settings in place")
					}
					return nil
				},
			},
		},
	})
}
