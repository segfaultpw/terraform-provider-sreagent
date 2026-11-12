# What every repository follows until it holds a value of its own. A change is
# previewed by the platform at plan time (the plan lists the repositories that
# move) and applied against that preview.
resource "sreagent_repository_defaults" "this" {
  auto_dispatch     = false
  platform_fallback = true
}

# Turning allow_ci_config or check_network on needs the matching entry in
# confirm for the apply that turns it on. Remove confirm afterwards: a
# confirmation left in the configuration also confirms every later apply that
# turns the setting on again.
#
#   allow_ci_config = true
#   confirm         = ["ci_config"]

# In a child organization that follows its parent's defaults, destroying the
# resource hands the organization back to the parent. If the parent's defaults
# turn CI configuration or network access on, the destroy needs a confirmation
# a destroy cannot carry: set confirm_on_destroy, apply, then destroy.
#
#   confirm_on_destroy = ["ci_config"]
