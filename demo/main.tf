// Tiny demo stack for exercising the tf wrapper. Uses only the builtin
// terraform_data resource, so `terraform init` needs no network access.

variable "flavor" {
  type    = string
  default = "vanilla"
}

resource "terraform_data" "alpha" {
  input = var.flavor
}

resource "terraform_data" "beta" {
  count = 3
  input = "${var.flavor}-${count.index}"
}

resource "terraform_data" "gamma" {
  input            = terraform_data.alpha.output
  triggers_replace = [var.flavor]
}

output "alpha_id" {
  value = terraform_data.alpha.id
}

output "flavor_used" {
  value = var.flavor
}
