output "ip" {
  value = hcloud_server.machine-controller-test.ipv4_address
}

output "private_ip" {
  value = hcloud_server_network.machine_controller.ip
}
