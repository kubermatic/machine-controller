/*
Copyright 2019 The Machine Controller Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package vsphere

import (
	"fmt"
	"math"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/types"
)

func validateDiskResize(devices object.VirtualDeviceList, diskSize int64) error {
	disks := devices.SelectByType((*types.VirtualDisk)(nil))
	if len(disks) != 1 {
		return fmt.Errorf("invalid disk count: %d. Resizing is only allowed when using 1 disk", len(disks))
	}

	disk := disks[0].(*types.VirtualDisk)
	requestedCapacity := diskSize * int64(math.Pow(1024, 2))
	if requestedCapacity < disk.CapacityInKB {
		attachedDiskSizeInGiB := disk.CapacityInKB / int64(math.Pow(1024, 2))
		return fmt.Errorf("requested diskSizeGB %d is smaller than size of attached disk(%dGiB)", diskSize, attachedDiskSizeInGiB)
	}

	return nil
}

func getDiskSpec(devices object.VirtualDeviceList, diskSize int64) (types.BaseVirtualDeviceConfigSpec, error) {
	disks := devices.SelectByType((*types.VirtualDisk)(nil))
	if len(disks) != 1 {
		return nil, fmt.Errorf("invalid disk count: %d. Resizing is only allowed when using 1 disk", len(disks))
	}

	disk := disks[0].(*types.VirtualDisk)
	disk.CapacityInKB = diskSize * int64(math.Pow(1024, 2))

	return &types.VirtualDeviceConfigSpec{
		Operation: types.VirtualDeviceConfigSpecOperationEdit,
		Device:    disk,
	}, nil
}
