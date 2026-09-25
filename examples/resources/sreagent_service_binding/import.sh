# By the service, plus ?environment=<name> when the environment is not the
# default. Both parts are escaped the way a URL escapes them.
terraform import 'sreagent_service_binding.checkout_prod' 'checkout?environment=prod'
