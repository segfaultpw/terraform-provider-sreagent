resource "sreagent_status_page_settings" "this" {
  enabled           = true
  title             = "Acme status"
  show_history_days = 30
}
