# Windows Images

## Dependency versions

* [Packer](https://github.com/hashicorp/packer) `1.6.5` or greater is required.
* [Packer-Provisioner-Windows-Update](https://github.com/rgl/packer-provisioner-windows-update) `0.11.0` or greater is required.
* envsubs
* 7z
* wget
* bash
* qemu-img

## Customizations

* [Enable-MicrosoftUpdates](./scripts/Enable-MicrosoftUpdates.ps1): Enables Updates for other Microsoft Products to be downloaded together with Windows Updates
* [Install-GuestTools](./scripts/Install-GuestTools.ps1): Installs guest tools for the used hypervisor. Currently only VirtIO (QEMU/KVM)
* [Install-PowerShellCore](./scripts/Install-PowerShellCore.ps1): Installs the most recent version of PowerShell. So that it can be utilized for administrative tasks and for remoting over SSH (instead of only WinRM).
* [Install-OpenSSH](./scripts/Install-OpenSSH.ps1): Installs the most recent version of OpenSSH so that we can connect using ssh.
* [Set-WindowsTimeZone](./scripts/Set-WindowsTimeZone.ps1): This script sets the time zone to UTC. The time zone is also set within the Autounattend.xml, but that has profen to be unreliable in recent builds of windows 10 and windows server.
* [Set-ProductKey](./scripts/Set-ProductKey.ps1): This script injects the product key and activates windows. In former versions of windows the product key was usualy provided via the Autounattend.xml, but that fails in some cases with recent builds of windows 10 and windows server.
* [Configure-WindowsSecurity](./scripts/Configure-WindowsSecurity.ps1): Configures windows to a base level of security so that it isn't an immediate target when deployed with public ip and no security group (firewall) restrictions in front of it.
  * Hardening Windows Defender settings (To change use PowerShell commands).
  * Enabling SmartScreen to protect against accidental execution of malicious files.
  * Disable IPv6 Privacy Extension and Interface Identifier Randomization. These are unsuitable for servers anyway, but cause problems within OpenStack environments esp. in combination with PortSecurity.
  * Enforce NLA (Network Level Authentication) for RDP
  * Enable RDP (it's recommended to tunnel it through SSH though)
  * Enable ICMP Echo Requests (allow to "ping" the server)
  * Disable Auto Logon that we used while installing windows until we were able to connect via SSH.
  * Enable CPU vulnerability Mitigations for Spectre, Meltdown, ...
  * Enforce strong security for TLS to prevent attacks and comply with many certifications of enterprises.
  * Enable SCHANNEL eventLogging, log failed connection attempts to the EventLog for later analysis or usage in a SIEM environment by the user.
* [Enable-InsecureWinRM](./scripts/Enable-InsecureWinRM.ps1): Sadly this is required because the windows update provider that we use to install windows updates while building does not support SSH and needs to connect over insecure WinRM.
* [Enable-SecureWinRM](./scripts/Enable-SecureWinRM.ps1): Secures WinRM again after installing windows updates using the packer provider.
* [Start-CleanUp](./scripts/Start-CleanUp.ps1): Clean up after installing windows updates. Delete Windows Update Server Id. So that if a wsus is used not all VMs deployed from this template "share the same name" and to reduce storage requirements of the final image.
* [Install-CloudbaseInit](./scripts/Install-CloudbaseInit.ps1): Installs Cloudbase-Init. [Cloudbase-Init](https://cloudbase.it/cloudbase-init/) is a version of cloud-init for windows provided by Cloudbase.
