# Every GitHub account the organization connected, with its settings and the
# repositories it reaches (at most 500 each).
data "sreagent_github_accounts" "all" {}

output "github_accounts" {
  value = [for a in data.sreagent_github_accounts.all.items : "${a.account} (${a.repository_count} repositories, managed: ${a.managed})"]
}
