resource "sreagent_alert_route" "checkout" {
  service = "checkout"
  target  = "#checkout-alerts"
}
