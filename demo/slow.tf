// Resources that take a few seconds to create, so the apply progress view
// (spinners, progress bar, ETA) is visible in demos. Provisioners only run
// on create, so plan/refresh stays fast.

resource "terraform_data" "slow" {
  count = 4
  input = "slow-${count.index}"

  provisioner "local-exec" {
    command = "sleep ${count.index * 2 + 1}"
  }
}
