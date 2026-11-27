# A row imports by the child and the account: CHILD?account=ACCOUNT, where the child
# is its id (preferred) or its name, written exactly the way the block writes
# child_organization. A block that names the child another way (its name where the
# import used its id, or the name in another case) plans a replacement that changes
# nothing on the platform: the old row is left in place and the same row is adopted again.
# While the organization has one connected account, the child alone is enough.
terraform import sreagent_github_child_access.acme_labs_acme acme-labs?account=acme
