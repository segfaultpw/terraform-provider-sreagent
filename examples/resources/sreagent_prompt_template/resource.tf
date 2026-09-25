resource "sreagent_prompt_template" "short_summary" {
  name        = "short summary"
  prompt_type = "custom"
  content     = "Summarize the alert in two lines."
}
