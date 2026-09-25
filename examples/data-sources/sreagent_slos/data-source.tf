data "sreagent_slos" "all" {}

# Filter in HCL.
output "slos_below_three_nines" {
  value = [for s in data.sreagent_slos.all.items : s.name if s.target < 99.9]
}
