# Deploying onto Amazon Web Services

With a pair of AWS keys
[accessible to Terraform](https://www.terraform.io/docs/providers/aws/), this
template deploys a simple Trillian setup in AWS using EC2 and RDS MySQL.

```shell
cd examples/deployment/aws/

# Set a random password
export TF_VAR_DB_PASSWORD="$(openssl rand -hex 16)"
# Substitute this variable with a block you'll be accessing from
export TF_VAR_WHITELIST_CIDR="0.0.0.0/0"

# Review and Create Resources
terraform plan
terraform apply
```
