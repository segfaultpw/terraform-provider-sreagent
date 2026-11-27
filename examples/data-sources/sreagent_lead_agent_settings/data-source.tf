# Read only: an organization admin changes what SRE Agent does with work it is
# given in Settings, SRE Agent on cards.
data "sreagent_lead_agent_settings" "this" {}

output "lead_agent_modes" {
  value = {
    card  = data.sreagent_lead_agent_settings.this.card_mode
    slack = data.sreagent_lead_agent_settings.this.slack_mode
    alert = data.sreagent_lead_agent_settings.this.alert_mode
  }
}
