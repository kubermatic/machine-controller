# VMWware vsphere

To use the machine-controller to create machines on VMWare vsphere, you must first
create a template:

1. Go into the VSphere WebUI, select your datacenter, right click onto it and choose "Deploy OVF Template"
2. Enter `https://cloud-images.ubuntu.com/releases/16.04/release/ubuntu-16.04-server-cloudimg-amd64.ova` into the
   "URL" field
3. Click through the dialog until "Select storage"
4. Select the same storage you want to use for your machines
5. Select the same network you want to use for your machines
6. Leave everyhting in the "Customize Template" and "Ready to complete" dialog as it is
7. Wait until the VM got fully imported and the "Snapshots" => "Create Snapshot" button is not grayed out anymore
