provider "hcloud" {
  token = "${var.hcloud_token}"
}

resource "hcloud_ssh_key" "default" {
  name = "${var.hcloud_sshkey_name}"
  public_key = "${var.hcloud_sshkey_content}"
}

resource "hcloud_server" "machine-controller-test" {
  name = "machine-controller-test"
  image = "ubuntu-16.04"
  server_type = "cx11"
  ssh_keys = ["${hcloud_ssh_key.default.id}"]
}
