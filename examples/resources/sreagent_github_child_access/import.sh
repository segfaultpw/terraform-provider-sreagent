# A row imports by the child and the account: CHILD?account=ACCOUNT, where the child
# is its id or its name, written the way the block writes child_organization.
# While the organization has one connected account, the child alone is enough.
terraform import sreagent_github_child_access.acme_labs_acme acme-labs?account=acme
