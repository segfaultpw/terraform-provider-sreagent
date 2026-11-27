package provider

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"

	"github.com/segfaultpw/terraform-provider-sreagent/internal/fakefacade"
	"github.com/segfaultpw/terraform-provider-sreagent/internal/specs"
)

var githubKeys = map[string]string{
	"sreagent_github_settings":     "github_settings",
	"sreagent_github_account":      "github_accounts",
	"sreagent_github_child_access": "github_child_access",
}

func adoptRenderedGitHubFile(t *testing.T, path string, expect map[string]int) {
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	file, diags := hclsyntax.ParseConfig(src, path, hcl.InitialPos)
	if diags.HasErrors() {
		t.Fatal(diags.Error())
	}

	type blk struct {
		typeName, label string
		values          map[string]any
		text            string
	}
	var resources, imports []byte
	var blocks []blk
	ids := map[string]string{}
	for _, b := range file.Body.(*hclsyntax.Body).Blocks {
		text := src[b.Range().Start.Byte:b.Range().End.Byte]
		switch b.Type {
		case "resource":
			values := map[string]any{}
			for name, attr := range b.Body.Attributes {
				v, d := attr.Expr.Value(nil)
				if d.HasErrors() {
					t.Fatalf("%s: %s", name, d.Error())
				}
				values[name] = goValue(t, v)
			}
			blocks = append(blocks, blk{b.Labels[0], b.Labels[1], values, string(text)})
			resources = append(resources, text...)
			resources = append(resources, '\n', '\n')
		case "import":
			tr := b.Body.Attributes["to"].Expr.(*hclsyntax.ScopeTraversalExpr)
			address := tr.Traversal.RootName() + "." + tr.Traversal[1].(hcl.TraverseAttr).Name
			v, d := b.Body.Attributes["id"].Expr.Value(nil)
			if d.HasErrors() {
				t.Fatal(d.Error())
			}
			ids[address] = v.AsString()
			imports = append(imports, text...)
			imports = append(imports, '\n', '\n')
		}
	}
	seen := map[string]int{}
	for _, b := range blocks {
		seen[b.typeName]++
	}
	for typeName, n := range expect {
		if seen[typeName] != n {
			t.Fatalf("the file holds %d %s, want %d: the proof would not cover it", seen[typeName], typeName, n)
		}
	}
	if len(ids) != len(blocks) {
		t.Fatalf("%d blocks and %d imports", len(blocks), len(ids))
	}

	f := fakefacade.New(t, specs.All())

	// accounts of the parent, with every repository any selected block names
	repos := map[string]map[string]bool{}
	shared := map[string]bool{}
	for _, b := range blocks {
		if b.typeName != "sreagent_github_child_access" {
			continue
		}
		acct := b.values["account"].(string)
		if repos[acct] == nil {
			repos[acct] = map[string]bool{}
		}
		if list, ok := b.values["repositories"].([]any); ok {
			for _, r := range list {
				repos[acct][r.(string)] = true
			}
		}
		child := b.values["child_organization"].(string)
		if v, ok := b.values["context_repo_shared"].(bool); ok {
			shared[child] = v
		} else if _, set := shared[child]; !set {
			shared[child] = true
		}
	}
	for acct, set := range repos {
		var names []string
		for r := range set {
			names = append(names, r)
		}
		sort.Strings(names)
		f.GitHubAccount(acct, names...)
	}
	nameRe := regexp.MustCompile(`# child organization: (.+)`)
	for _, b := range blocks {
		switch b.typeName {
		case "sreagent_github_child_access":
			child := b.values["child_organization"].(string)
			name := child
			if m := nameRe.FindStringSubmatch(b.text); m != nil {
				name = m[1]
			}
			mode, _ := b.values["mode"].(string)
			var rs []string
			if list, ok := b.values["repositories"].([]any); ok {
				for _, r := range list {
					rs = append(rs, r.(string))
				}
			}
			f.LinkChild(child, name, b.values["account"].(string), mode, shared[child], rs...)
		case "sreagent_github_account":
			row := map[string]any{"id": b.values["account"], "managed": false, "status": "active"}
			for k, v := range b.values {
				row[k] = v
			}
			f.Seed("github_accounts", row)
		case "sreagent_github_settings":
			f.Mutate("github_settings", "github_settings", func(stored map[string]any) {
				for k, v := range b.values {
					stored[k] = v
				}
			})
		}
	}

	noWrites := func(*terraform.State) error {
		for _, key := range githubKeys {
			for _, method := range []string{"PUT", "POST", "DELETE"} {
				if n := f.Calls(method, key); n != 0 {
					return fmt.Errorf("adopting the rendered file sent %d %s requests to %s", n, method, key)
				}
			}
		}
		return nil
	}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		TerraformVersionChecks:   []tfversion.TerraformVersionCheck{tfversion.SkipBelow(tfversion.Version1_12_0)},
		Steps: []resource.TestStep{
			{
				Config:           providerBlock(f.URL) + string(resources) + string(imports),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{importsOnly{}}},
				Check:            noWrites,
			},
			{
				Config:           providerBlock(f.URL) + string(resources),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
				Check:            noWrites,
			},
		},
	})
}

func TestTheRenderedParentFileAdoptsAndChangesNothing(t *testing.T) {
	adoptRenderedGitHubFile(t, "testdata/export_github_parent.tf", map[string]int{
		"sreagent_github_settings": 1, "sreagent_github_account": 1, "sreagent_github_child_access": 4,
	})
}

func TestTheRenderedMixedChildFileAdoptsAndChangesNothing(t *testing.T) {
	adoptRenderedGitHubFile(t, "testdata/export_github_mixed_child.tf", map[string]int{
		"sreagent_github_settings": 1, "sreagent_github_account": 1, "sreagent_github_child_access": 0,
	})
}
