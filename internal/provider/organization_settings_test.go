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

const orgSettings = "sreagent_organization_settings.o"

func orgSettingsConfig(url, body string) string {
	return providerBlock(url) + fmt.Sprintf("resource \"sreagent_organization_settings\" \"o\" {\n%s\n}\n", body)
}

// fix_request_floor is decided by an org admin in the app, so the provider
// reads it into state and never sends it, whatever else an apply writes.
func TestFixRequestFloorIsReadAndNeverWritten(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				PreConfig: func() {
					f.Mutate("organization_settings", "organization_settings", func(r map[string]any) { r["fix_request_floor"] = "member" })
				},
				Config: orgSettingsConfig(f.URL, "  alert_storm_threshold = 12"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(orgSettings, "fix_request_floor", "member"),
					resource.TestCheckResourceAttr(orgSettings, "alert_storm_threshold", "12"),
					func(*terraform.State) error {
						body := f.LastBody("PUT", "organization_settings")
						if _, sent := body["fix_request_floor"]; sent {
							return fmt.Errorf("fix_request_floor was written: %v", body)
						}
						if _, sent := body["inheritance_stopped"]; sent {
							return fmt.Errorf("inheritance_stopped was written without being configured: %v", body)
						}
						return nil
					},
				),
			},
			{
				// An org admin changes it in the app; the next plan shows it and proposes nothing.
				PreConfig: func() {
					f.Mutate("organization_settings", "organization_settings", func(r map[string]any) { r["fix_request_floor"] = "org_admin" })
				},
				Config: orgSettingsConfig(f.URL, "  alert_storm_threshold = 12"),
				Check:  resource.TestCheckResourceAttr(orgSettings, "fix_request_floor", "org_admin"),
			},
			{ResourceName: orgSettings, ImportState: true, ImportStateId: "organization_settings", ImportStateVerify: true, Config: orgSettingsConfig(f.URL, "  alert_storm_threshold = 12")},
		},
	})
}

func TestFixRequestFloorCannotBeConfigured(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config:      orgSettingsConfig(f.URL, "  fix_request_floor = \"member\""),
			ExpectError: regexp.MustCompile(`(?s)fix_request_floor.*(read-only|Computed|cannot be configured)`),
		}},
	})
	if n := f.Calls("PUT", "organization_settings"); n != 0 {
		t.Fatalf("a refused configuration still sent %d PUTs", n)
	}
}

func TestFixRequestFloorIsReadByTheDataSource(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	f.Mutate("organization_settings", "organization_settings", func(r map[string]any) { r["fix_request_floor"] = "member" })
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config: providerBlock(f.URL) + "data \"sreagent_organization_settings\" \"o\" {}\n",
			Check:  resource.TestCheckResourceAttr("data.sreagent_organization_settings.o", "fix_request_floor", "member"),
		}},
	})
}

// inheritance_stopped is the whole set of a parent's connections the
// organization does not use; the contract lists it as writable, so it is
// sent when configured and left alone otherwise.
func TestInheritanceStoppedRoundTripsAndIsSentOnlyWhenConfigured(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config: orgSettingsConfig(f.URL, "  inheritance_stopped = [\"slack\", \"ai_providers\"]"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(orgSettings, "inheritance_stopped.#", "2"),
					resource.TestCheckResourceAttr(orgSettings, "inheritance_stopped.0", "slack"),
					resource.TestCheckResourceAttr(orgSettings, "inheritance_stopped.1", "ai_providers"),
					func(*terraform.State) error {
						if got := f.Row("organization_settings", "organization_settings")["inheritance_stopped"]; !reflect.DeepEqual(got, []any{"slack", "ai_providers"}) {
							return fmt.Errorf("the platform holds %v", got)
						}
						return nil
					},
				),
			},
			{
				// Dropping the argument leaves the stored set in place, like every other setting.
				Config: orgSettingsConfig(f.URL, "  alert_storm_threshold = 9"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(orgSettings, "inheritance_stopped.#", "2"),
					func(*terraform.State) error {
						if _, sent := f.LastBody("PUT", "organization_settings")["inheritance_stopped"]; sent {
							return fmt.Errorf("an unset inheritance_stopped was sent: %v", f.LastBody("PUT", "organization_settings"))
						}
						return nil
					},
				),
			},
			{
				Config: orgSettingsConfig(f.URL, "  inheritance_stopped = []"),
				Check:  resource.TestCheckResourceAttr(orgSettings, "inheritance_stopped.#", "0"),
			},
		},
	})
}

// A platform from before the two fields does not answer them: they read as
// null and nothing about the rest of the settings changes.
func TestOrganizationSettingsReadsAnOlderPlatformWithoutTheFields(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	f.Mutate("organization_settings", "organization_settings", func(r map[string]any) {
		delete(r, "fix_request_floor")
		delete(r, "inheritance_stopped")
	})
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config: orgSettingsConfig(f.URL, "  alert_storm_threshold = 7"),
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr(orgSettings, "alert_storm_threshold", "7"),
				resource.TestCheckNoResourceAttr(orgSettings, "fix_request_floor"),
				resource.TestCheckNoResourceAttr(orgSettings, "inheritance_stopped.#"),
			),
		}},
	})
}

// The sign-in and key limits are decided by an org admin in the app: the
// provider reads them (and the effective values the platform derives) into
// state and never sends them.
func TestTheMFAAndSessionLimitsAreReadAndNeverWritten(t *testing.T) {
	f := fakefacade.New(t, specs.All())
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				PreConfig: func() {
					f.Mutate("organization_settings", "organization_settings", func(r map[string]any) {
						r["mfa_required"] = true
						r["mfa_grace_days"] = 7
						r["session_idle_minutes"] = 60
						r["api_key_max_days"] = 90
						r["mfa_effective"] = map[string]any{"required": true, "source": "own"}
						r["session_idle_effective"] = 60
					})
				},
				Config: orgSettingsConfig(f.URL, "  alert_storm_threshold = 12"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(orgSettings, "mfa_required", "true"),
					resource.TestCheckResourceAttr(orgSettings, "mfa_grace_days", "7"),
					resource.TestCheckResourceAttr(orgSettings, "session_idle_minutes", "60"),
					resource.TestCheckResourceAttr(orgSettings, "api_key_max_days", "90"),
					resource.TestCheckResourceAttr(orgSettings, "session_idle_effective", "60"),
					resource.TestCheckResourceAttrSet(orgSettings, "mfa_effective"),
					func(*terraform.State) error {
						body := f.LastBody("PUT", "organization_settings")
						for _, k := range []string{"mfa_required", "mfa_grace_days", "session_idle_minutes", "api_key_max_days", "mfa_effective", "session_idle_effective"} {
							if _, sent := body[k]; sent {
								return fmt.Errorf("%s was written: %v", k, body)
							}
						}
						return nil
					},
				),
			},
		},
	})
}

func TestTheMFAAndSessionLimitsCannotBeConfigured(t *testing.T) {
	for _, name := range []string{"mfa_required = true", "mfa_grace_days = 3", "session_idle_minutes = 30", "api_key_max_days = 30"} {
		t.Run(name, func(t *testing.T) {
			f := fakefacade.New(t, specs.All())
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: factories,
				Steps: []resource.TestStep{{
					Config:      orgSettingsConfig(f.URL, "  "+name),
					ExpectError: regexp.MustCompile(`(?s)Read-Only Attribute`),
				}},
			})
			if n := f.Calls("PUT", "organization_settings"); n != 0 {
				t.Fatalf("a refused configuration still sent %d PUTs", n)
			}
		})
	}
}
