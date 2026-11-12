# The other:<kind> code aliases counted as security findings, beyond the built-in classes.
data "sreagent_security_code_classes" "this" {}

output "marked_code_classes" {
  value = data.sreagent_security_code_classes.this.classes
}
