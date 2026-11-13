# Keep the weekly scan from filing credential matches in one repository's test fixtures,
# until the end of the year. A rule cannot be edited: any change replaces it.
resource "sreagent_secret_allow_rule" "fixtures" {
  path_glob  = "**/testdata/**"
  reason     = "Vendored test fixtures, not real keys"
  repository = "acme/web"
  expires_at = "2027-12-31T23:59:59Z"
}
