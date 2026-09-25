package engine

import (
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

// Resources returns a resource for every spec with a lifecycle.
func Resources(specs []Spec) []func() resource.Resource {
	out := []func() resource.Resource{}
	for _, s := range specs {
		if s.Lifecycle {
			out = append(out, NewResource(s))
		}
	}
	return out
}

// DataSources returns the data sources; filled in by Task 25.
func DataSources(_ []Spec) []func() datasource.DataSource {
	return nil
}
