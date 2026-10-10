# By the service, plus ?environment=<name> when the environment is not the
# default (for example 'checkout?environment=prod'). Both parts are escaped the
# way a URL escapes them.
terraform import 'sreagent_service_binding.checkout' 'checkout'
