# The complete list of other:<kind> classes counted as security findings, in any order.
# Unmarking a class closes its open findings.
resource "sreagent_security_code_classes" "this" {
  classes = ["other:ssrf", "other:xxe"]
}
