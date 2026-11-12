# What every repository follows until it holds a value of its own. A change is
# previewed by the platform at plan time (the plan lists the repositories that
# move) and applied against that preview.
resource "sreagent_repository_defaults" "this" {
  auto_dispatch     = false
  platform_fallback = true

  # Turning allow_ci_config or check_network on needs the matching entry here.
  allow_ci_config = true
  confirm         = ["ci_config"]
}
