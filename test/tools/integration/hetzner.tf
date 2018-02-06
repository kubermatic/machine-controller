provider "hcloud" {
  token = "${var.hcloud_token}"
}

resource "hcloud_ssh_key" "default" {
  name = "${var.hcloud_sshkey_name}"
  public_key = "${var.hcloud_sshkey_content}"
}
