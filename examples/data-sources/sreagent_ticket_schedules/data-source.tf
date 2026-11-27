# Read only: an organization admin writes recurring cards on the board's Templates page.
data "sreagent_ticket_schedules" "all" {}

output "next_runs" {
  value = { for s in data.sreagent_ticket_schedules.all.items : s.name => s.next_run_at }
}
