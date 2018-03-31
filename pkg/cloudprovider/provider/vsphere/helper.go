package vsphere

import (
	"context"
	"fmt"
	"path"

	"github.com/golang/glog"

	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"
)

const (
	snapshotName string = "machine-controller"
	snapshotDesc string = "Snapshot created by machine-controller"
)

func CreateLinkClonedVm(vmName, vmImage, datacenter, clusterName string, client *govmomi.Client) (string, error) {
	f := find.NewFinder(client.Client, true)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dc, err := f.Datacenter(ctx, datacenter)
	if err != nil {
		return "", err
	}
	f.SetDatacenter(dc)

	templateVm, err := f.VirtualMachine(ctx, vmImage)
	if err != nil {
		return "", err
	}

	glog.V(3).Infof("Template VM ref is %+v", templateVm)
	datacenterFolders, err := dc.Folders(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get datacenter folders: %v", err)
	}

	// Create snapshot of the template VM if not already snapshotted.
	snapshot, err := createSnapshot(ctx, templateVm, snapshotName, snapshotDesc)
	if err != nil {
		return "", fmt.Errorf("failed to create snapshot: %v", err)
	}

	clsComputeRes, err := f.ClusterComputeResource(ctx, clusterName)
	if err != nil {
		return "", fmt.Errorf("failed to get cluster %s: %v", clusterName, err)
	}
	glog.V(3).Infof("Cluster is %+v", clsComputeRes)

	resPool, err := clsComputeRes.ResourcePool(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get ressource pool: %v", err)
	}
	glog.V(3).Infof("Cluster resource pool is %+v", resPool)

	if resPool == nil {
		return "", fmt.Errorf("no resource pool found for cluster %s", clusterName)
	}

	resPoolRef := resPool.Reference()
	snapshotRef := snapshot.Reference()

	diskUuidEnabled := true
	cloneSpec := &types.VirtualMachineCloneSpec{
		Config: &types.VirtualMachineConfigSpec{
			Flags: &types.VirtualMachineFlagInfo{
				DiskUuidEnabled: &diskUuidEnabled,
			},
		},
		Location: types.VirtualMachineRelocateSpec{
			Pool:         &resPoolRef,
			DiskMoveType: "createNewChildDiskBacking",
		},
		Snapshot: &snapshotRef,
	}

	// Create a link cloned VM from the template VM's snapshot
	clonedVmTask, err := templateVm.Clone(ctx, datacenterFolders.VmFolder, vmName, *cloneSpec)
	if err != nil {
		return "", err
	}

	clonedVmTaskInfo, err := clonedVmTask.WaitForResult(ctx, nil)
	if err != nil {
		return "", err
	}

	clonedVm := clonedVmTaskInfo.Result.(object.Reference)
	glog.V(2).Infof("Created VM %s successfully", clonedVm)

	return clonedVm.Reference().Value, nil
}

func createSnapshot(ctx context.Context, vm *object.VirtualMachine, snapshotName string, snapshotDesc string) (object.Reference, error) {
	//TODO: Add protection for snapshot creation
	//snapshotLock.Lock()
	//defer snapshotLock.Unlock()

	snapshotRef, err := findSnapshot(vm, ctx, snapshotName)
	if err != nil {
		return nil, err
	}
	glog.V(4).Infof("Template VM is %s and snapshot is %s", vm, snapshotRef)
	if snapshotRef != nil {
		return snapshotRef, nil
	}

	task, err := vm.CreateSnapshot(ctx, snapshotName, snapshotDesc, false, false)
	if err != nil {
		return nil, err
	}

	taskInfo, err := task.WaitForResult(ctx, nil)
	if err != nil {
		return nil, err
	}
	glog.Infof("taskInfo.Result is %s", taskInfo.Result)
	return taskInfo.Result.(object.Reference), nil
}

type snapshotMap map[string][]object.Reference

func (m snapshotMap) add(parent string, tree []types.VirtualMachineSnapshotTree) {
	for i, st := range tree {
		sname := st.Name
		names := []string{sname, st.Snapshot.Value}

		if parent != "" {
			sname = path.Join(parent, sname)
			// Add full path as an option to resolve duplicate names
			names = append(names, sname)
		}

		for _, name := range names {
			m[name] = append(m[name], &tree[i].Snapshot)
		}

		m.add(sname, st.ChildSnapshotList)
	}
}

func findSnapshot(v *object.VirtualMachine, ctx context.Context, name string) (object.Reference, error) {
	var o mo.VirtualMachine

	err := v.Properties(ctx, v.Reference(), []string{"snapshot"}, &o)
	if err != nil {
		return nil, err
	}

	if o.Snapshot == nil || len(o.Snapshot.RootSnapshotList) == 0 {
		return nil, nil
	}

	//TODO: Rework this for readability, the only thing we want to know is if there is exactly one
	// snapshot without parent and with the correct name
	m := make(snapshotMap)
	m.add("", o.Snapshot.RootSnapshotList)

	s := m[name]
	switch len(s) {
	case 0:
		return nil, nil
	case 1:
		return s[0], nil
	default:
		glog.Warningf("VM %s seems to have more than one snapshots with name %s. Using a random snapshot.", v, name)
		return s[0], nil
	}
}

func uploadAndAttachISO(f *find.Finder, vmRef *object.VirtualMachine, localIsoFilePath string, client *govmomi.Client) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var refs []types.ManagedObjectReference
	refs = append(refs, vmRef.Reference())
	var vmResult mo.VirtualMachine

	pc := property.DefaultCollector(client.Client)
	err := pc.RetrieveOne(ctx, vmRef.Reference(), []string{"datastore"}, &vmResult)
	if err != nil {
		return err
	}
	glog.V(3).Infof("vm property collector result :%+v\n", vmResult)

	// We expect the VM to be on only 1 datastore
	dsRef := vmResult.Datastore[0].Reference()
	var dsResult mo.Datastore
	err = pc.RetrieveOne(ctx, dsRef, []string{"summary"}, &dsResult)
	if err != nil {
		return err
	}
	glog.V(3).Infof("datastore property collector result :%+v\n", dsResult)
	dsObj, err := f.Datastore(ctx, dsResult.Summary.Name)
	if err != nil {
		return err
	}
	p := soap.DefaultUpload
	remoteIsoFilePath := fmt.Sprintf("%s/%s", vmRef.Name(), "cloud-init.iso")
	glog.V(3).Infof("Uploading userdata ISO to datastore %+v, destination iso is %s\n", dsObj, remoteIsoFilePath)
	err = dsObj.UploadFile(ctx, localIsoFilePath, remoteIsoFilePath, &p)
	if err != nil {
		return err
	}
	glog.V(3).Infof("Uploaded ISO file %s", localIsoFilePath)

	// Find the cd-rom devide and insert the cloud init iso file into it.
	devices, err := vmRef.Device(ctx)
	if err != nil {
		return err
	}

	// passing empty cd-rom name so that the first one gets returned
	cdrom, err := devices.FindCdrom("")
	cdrom.Connectable.StartConnected = true
	if err != nil {
		return err
	}
	iso := dsObj.Path(remoteIsoFilePath)
	glog.V(2).Infof("Inserting ISO file %s into cd-rom", iso)
	return vmRef.EditDevice(ctx, devices.InsertIso(cdrom, iso))

}

func getVirtualMachine(name string, datacenterFinder *find.Finder) (*object.VirtualMachine, error) {
	return datacenterFinder.VirtualMachine(context.TODO(), name)
}

func getDatacenterFinder(datacenter string, client *govmomi.Client) (*find.Finder, error) {
	finder := find.NewFinder(client.Client, true)
	dc, err := finder.Datacenter(context.TODO(), datacenter)
	if err != nil {
		return nil, fmt.Errorf("failed to get vsphere datacenter: %v", err)
	}
	finder.SetDatacenter(dc)
	return finder, nil
}
