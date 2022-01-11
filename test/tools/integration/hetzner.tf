provider "hcloud" {
  token = var.hcloud_token
}

resource "hcloud_ssh_key" "default" {
  name       = var.hcloud_sshkey_name
  public_key = var.hcloud_sshkey_content
}

resource "hcloud_network" "net" {
  name     = var.hcloud_test_server_name
  ip_range = "192.168.0.0/16"
}

resource "hcloud_server" "machine-controller-test" {
  name        = var.hcloud_test_server_name
  image       = "ubuntu-20.04"
  server_type = "cx21"
  ssh_keys    = [hcloud_ssh_key.default.id]
  location    = "nbg1"
}

resource "hcloud_network_subnet" "machine_controller" {
  network_id   = hcloud_network.net.id
  type         = "server"
  network_zone = var.hcloud_network_zone
  ip_range     = "192.168.0.0/16"
}

resource "hcloud_server_network" "machine_controller" {
  server_id = hcloud_server.machine-controller-test.id
  subnet_id = hcloud_network_subnet.machine_controller.id
}
