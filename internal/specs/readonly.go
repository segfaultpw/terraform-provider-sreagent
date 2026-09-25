package specs

import "github.com/segfaultpw/terraform-provider-sreagent/internal/engine"

func ro(name string, k engine.Kind, desc string) engine.Attr {
	return engine.Attr{Name: name, Kind: k, Computed: true, Description: desc}
}

// Terraform reads these but does not manage them: compliance periods are
// immutable audit windows, an incident's update appends a message rather than
// setting an attribute, and the ExternalId is a two-value read.

// CompliancePeriod is data.sreagent_compliance_periods.
var CompliancePeriod = engine.Spec{
	Key: "compliance_periods", TypeName: "compliance_period", ListName: "compliance_periods",
	Description: "Audit windows; immutable once opened.",
	Shape:       engine.Generated,
	Attrs: []engine.Attr{
		ro("name", engine.String, "The window's name."),
		ro("framework", engine.String, "The framework."),
		ro("starts_on", engine.String, "First day."),
		ro("ends_on", engine.String, "Last day."),
		ro("status", engine.String, "open or exported."),
		ro("export_top_hash", engine.String, "The exported bundle's manifest hash."),
	},
}

// StatusPageIncident is data.sreagent_status_page_incidents.
var StatusPageIncident = engine.Spec{
	Key: "status_page_incidents", TypeName: "status_page_incident", ListName: "status_page_incidents",
	Description: "Active incidents and those resolved inside the page's history window (capped at 100).",
	Shape:       engine.Generated,
	Attrs: []engine.Attr{
		ro("title", engine.String, "The headline."),
		ro("status", engine.String, "The phase."),
		ro("impact", engine.String, "The severity."),
		ro("scheduled_for", engine.String, "Maintenance start."),
		ro("scheduled_until", engine.String, "Maintenance end."),
		ro("component_ids", engine.StringList, "Affected components."),
		ro("started_at", engine.String, "When it started."),
		ro("resolved_at", engine.String, "When it was resolved."),
	},
}

// AWSExternalID is data.sreagent_aws_external_id: what a customer's IAM role
// trust policy needs, in the same apply as the data source that assumes it.
var AWSExternalID = engine.Spec{
	Key: "aws_external_id", TypeName: "aws_external_id", ListName: "aws_external_id",
	Description: "The organization's AWS ExternalId and the platform principal an IAM role trusts. Use both in the role's trust policy.",
	Shape:       engine.Singleton,
	Attrs: []engine.Attr{
		ro("external_id", engine.String, "The organization's ExternalId; minted on first read."),
		ro("trust_principal_arn", engine.String, "The platform principal the role trusts."),
	},
}
