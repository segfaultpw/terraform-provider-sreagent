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

// DataSources returns a list data source per collection and a read data source per singleton.
func DataSources(specs []Spec) []func() datasource.DataSource {
	out := []func() datasource.DataSource{}
	for _, s := range specs {
		spec := s
		list := spec.Shape != Singleton
		out = append(out, func() datasource.DataSource { return &facadeDataSource{spec: spec, list: list} })
	}
	return out
}
