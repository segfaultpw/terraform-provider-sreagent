// Package contract reads the configuration API's OpenAPI document.
package contract

import (
	"encoding/json"
	"os"
)

// Resource is one entry of x-sreagent-resources.
type Resource struct {
	Singleton      bool           `json:"singleton"`
	Lifecycle      bool           `json:"lifecycle"`
	TerraformType  string         `json:"terraform_type"`
	Fields         []string       `json:"fields"`
	SecretFields   []string       `json:"secret_fields"`
	ComputedFields map[string]any `json:"computed_fields"`
	ActionArgs     []string       `json:"action_args"`
	CreateFields   []string       `json:"create_fields"`
	UpdateFields   []string       `json:"update_fields"`
	IDArgs         []string       `json:"id_args"`
}

// Doc is the part of the document the provider is held to.
type Doc struct {
	Resources  map[string]Resource `json:"x-sreagent-resources"`
	Components struct {
		Schemas map[string]struct {
			Properties map[string]map[string]any `json:"properties"`
		} `json:"schemas"`
	} `json:"components"`
	Paths map[string]map[string]struct {
		RequestBody struct {
			Content map[string]struct {
				Schema struct {
					Properties map[string]map[string]any `json:"properties"`
					Required   []string                  `json:"required"`
				} `json:"schema"`
			} `json:"content"`
		} `json:"requestBody"`
	} `json:"paths"`
}

// Load reads a document from disk.
func Load(path string) (*Doc, error) {
	b, err := os.ReadFile(path) //nolint:gosec // the caller names the vendored document
	if err != nil {
		return nil, err
	}
	var d Doc
	return &d, json.Unmarshal(b, &d)
}
