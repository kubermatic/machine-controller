variable "hcloud_token" {}
variable "hcloud_sshkey_content" {}

variable "hcloud_sshkey_name" {
  default = "machine-controller-e2e"
}

variable "hcloud_test_server_name" {}

variable "hcloud_network_zone" {
  default     = "eu-central"
  description = "network zone to use for private network"
  type        = string
}
