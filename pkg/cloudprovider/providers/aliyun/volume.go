/*
Copyright 2016 The Archon Authors.

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

package aliyun

import (
	"errors"
	"fmt"
	"github.com/denverdino/aliyungo/common"
	"github.com/denverdino/aliyungo/ecs"
	log "github.com/golang/glog"
	pvcontroller "github.com/kubernetes-incubator/external-storage/lib/controller"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/pkg/api/v1"
	"k8s.io/kubernetes/pkg/util/exec"
	"k8s.io/kubernetes/pkg/util/mount"
	"os"
	"path"
	"strings"
	//"syscall"
	"kubeup.com/kube-aliyun/pkg/cloudprovider"
	"kubeup.com/kube-aliyun/pkg/util"
	"time"
)

var (
	OptionFSType    = "kubernetes.io/fsType"
	OptionReadWrite = "kubernetes.io/readwrite"

	APIPrefix   = "/dev/xvd"
	LocalPrefix = "/dev/vd"

	WaitInterval         = time.Second
	WaitForAttachTimeout = 10 * time.Second

	DefaultFSType   = "ext4"
	DriverName      = "aliyun/flexv"
	ProvisionerName = "archon.kubeup.com/aliyun"

	MinSizeTable = map[ecs.DiskCategory]int{
		ecs.DiskCategoryCloud:           5,
		ecs.DiskCategoryEphemeral:       5,
		ecs.DiskCategoryEphemeralSSD:    20,
		ecs.DiskCategoryCloudEfficiency: 20,
		ecs.DiskCategoryCloudSSD:        20,
	}
)

var _ cloudprovider.Volume = &AliyunProvider{}
var _ pvcontroller.Provisioner = &AliyunProvider{}

type StorageOptions struct {
	FSType   string `k8s:"fsType"`
	Category string `k8s:"diskCategory"`
}

// TODO: Use nil as success result

// Aliyun api only takes /dev/xvd? even though it's actuall vd? in coreos system
func deviceApi2Local(s string) string {
	return strings.Replace(s, APIPrefix, LocalPrefix, 1)
}

func deviceLocal2Api(s string) string {
	return strings.Replace(s, LocalPrefix, APIPrefix, 1)
}

func sameDevice(a, b string) bool {
	return deviceApi2Local(a) == deviceApi2Local(b)
}

func (p *AliyunProvider) getDiskById(diskId string) (d ecs.DiskItemType, err error) {
	pg := &common.Pagination{
		PageNumber: 0,
		PageSize:   50,
	}

	args := &ecs.DescribeDisksArgs{
		RegionId:   common.Region(p.region),
		DiskIds:    []string{diskId},
		Pagination: *pg,
	}
	disks, _, err := p.client.DescribeDisks(args)
	if err != nil {
		return d, err
	}

	if len(disks) > 0 {
		d = disks[0]
		return
	}

	err = errors.New(fmt.Sprintf("DiskId %s is not found", diskId))
	return
}

func (p *AliyunProvider) getDiskByDevice(instanceId, device string) (d ecs.DiskItemType, err error) {
	pg := &common.Pagination{
		PageNumber: 0,
		PageSize:   50,
	}
	for {
		args := &ecs.DescribeDisksArgs{
			RegionId:   common.Region(p.region),
			InstanceId: instanceId,
			Pagination: *pg,
		}
		disks, pgr, err := p.client.DescribeDisks(args)
		if err != nil {
			return d, err
		}

		for _, disk := range disks {
			if sameDevice(disk.Device, device) {
				return disk, nil
			}
		}

		pg = pgr.NextPage()
		if pg == nil {
			break
		}
	}

	err = errors.New(fmt.Sprintf("Device %s is not found in provider inventory", device))
	return
}

func (p *AliyunProvider) Init() error {
	return cloudprovider.VolumeSuccess
}

func (p *AliyunProvider) Attach(options cloudprovider.VolumeOptions, node string) error {
	instance := p.instance
	if node != "" {
		instances, err := p.getInstanceIdsByNodeNames([]string{node})
		if len(instances) == 0 {
			return cloudprovider.NewVolumeError("Can't get instanceId for node: %v %v", node, err)
		}
		instance = instances[0]
	}

	if instance == "" {
		return cloudprovider.NewVolumeError("Failed to attach. No ALIYUN_INSTANCE is set and no node is provided.")
	}

	diskId, _ := options["diskId"].(string)
	if diskId == "" {
		return cloudprovider.NewVolumeError("diskid is required")
	}

	// Optional
	deleteWithInstance, _ := options["deleteWithInstance"].(bool)

	// Check if already attached
	disk, err := p.getDiskById(diskId)
	if err == nil && disk.InstanceId == instance && disk.DiskId == diskId && disk.Status == ecs.DiskStatusInUse {
		return cloudprovider.VolumeError{
			Status:     "Success",
			DevicePath: deviceApi2Local(disk.Device),
		}
	}

	args := &ecs.AttachDiskArgs{
		InstanceId:         instance,
		DiskId:             diskId,
		DeleteWithInstance: deleteWithInstance,
	}
	err = p.client.AttachDisk(args)
	if err != nil {
		return cloudprovider.NewVolumeError(err.Error())
	}
	region := common.Region(p.region)
	p.client.WaitForDisk(region, diskId, ecs.DiskStatusInUse, 0)

	disk, err = p.getDiskById(diskId)
	if err != nil {
		return cloudprovider.NewVolumeError(err.Error())
	}

	status := cloudprovider.NewVolumeSuccess()
	status.DevicePath = deviceApi2Local(disk.Device)
	return status
}

// Here the deviceName is actually the volumeName from GetVolumeName. see operation-generator.go
func (p *AliyunProvider) Detach(deviceName string, node string) error {
	instance := p.instance

	if node != "" {
		instances, err := p.getInstanceIdsByNodeNames([]string{node})
		if len(instances) == 0 {
			return cloudprovider.NewVolumeError("Can't not get instanceId for node: %v %v", node, err)
		}
		instance = instances[0]
	}

	if instance == "" {
		return cloudprovider.NewVolumeError("Failed to attach. No ALIYUN_INSTANCE is set and no node is provided.")
	}

	diskId := path.Base(deviceName)
	disk, err := p.getDiskById(diskId)
	if err != nil {
		return cloudprovider.NewVolumeError(err.Error())
	}

	switch disk.Status {
	case ecs.DiskStatusDetaching, ecs.DiskStatusAvailable:
		return cloudprovider.VolumeSuccess
	}

	if disk.InstanceId != instance {
		return cloudprovider.VolumeSuccess
	}

	err = p.client.DetachDisk(instance, disk.DiskId)
	if err != nil {
		return cloudprovider.NewVolumeError(err.Error())
	}

	return cloudprovider.VolumeSuccess
}

func (p *AliyunProvider) MountDevice(dir, device string, options cloudprovider.VolumeOptions) error {
	fstype, _ := options[OptionFSType].(string)
	//data, _ := options["data"].(string)
	readwrite, _ := options[OptionReadWrite].(string)
	flagstr, _ := options["flags"].(string)
	flags := []string{}
	if flagstr != "" {
		flags = strings.Split(flagstr, ",")
	}
	if readwrite != "" {
		flags = append(flags, readwrite)
	}

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err = os.MkdirAll(dir, 0750); err != nil {
			return cloudprovider.NewVolumeError(err.Error())
		}
	}

	mounter := &mount.SafeFormatAndMount{Interface: mount.New(""), Runner: exec.New()}
	err := mounter.FormatAndMount(device, dir, fstype, flags)
	if err != nil {
		return cloudprovider.NewVolumeError(err.Error())
	}

	return cloudprovider.VolumeSuccess
}

func (p *AliyunProvider) UnmountDevice(dir string) error {
	mounter := &mount.SafeFormatAndMount{Interface: mount.New(""), Runner: exec.New()}
	err := mounter.Unmount(dir)
	if err != nil {
		return cloudprovider.NewVolumeError(err.Error())
	}
	return cloudprovider.VolumeSuccess
}

// WaitForAttach checks if the device is successfully attached on a given node.
// If there's no way to check, return not supported (ex. when it's another node and access
// key is not provided
func (p *AliyunProvider) WaitForAttach(device string, options cloudprovider.VolumeOptions) error {
	if p.authorized() {
		diskId, _ := options["diskId"].(string)

		// Wait by API
		disk, err := p.getDiskById(diskId)
		if err != nil {
			return cloudprovider.NewVolumeError(err.Error())
		}

		region := common.Region(p.region)
		err = p.client.WaitForDisk(region, disk.DiskId, ecs.DiskStatusInUse, 0)
		if err != nil {
			return cloudprovider.NewVolumeError(err.Error())
		}

		return cloudprovider.VolumeSuccess
	}

	// Can't check.
	return cloudprovider.NewVolumeNotSupported("")
}

func (p *AliyunProvider) GetVolumeName(options cloudprovider.VolumeOptions) error {
	diskId, _ := options["diskId"].(string)
	if diskId == "" {
		return cloudprovider.NewVolumeError("diskId is required, options: %+v", options)
	}

	status := cloudprovider.NewVolumeSuccess()
	status.VolumeName = diskId
	return status
}

func (p *AliyunProvider) IsAttached(options cloudprovider.VolumeOptions, node string) error {
	if !p.authorized() {
		return cloudprovider.NewVolumeNotSupported("Unable to check if a spec is attached without access key")
	}

	// localhost is passed in flexvolume tests. Not sure if this will happen in real life?
	if node == "localhost" {
		node = p.hostname
	}

	// Only do this when flexv is provided with access keys
	instances, err := p.getInstanceIdsByNodeNames([]string{node})
	if len(instances) == 0 {
		return cloudprovider.NewVolumeError("Can't not get instanceId for node: %v", node, err)
	}
	instance := instances[0]

	diskId, _ := options["diskId"].(string)
	if diskId == "" {
		return cloudprovider.NewVolumeError("diskid is required")
	}

	disk, err := p.getDiskById(diskId)
	if err != nil {
		return cloudprovider.NewVolumeError(err.Error())
	}

	status := cloudprovider.NewVolumeSuccess()
	status.Attached = disk.InstanceId == instance && disk.Status == ecs.DiskStatusInUse

	return status
}

func (p *AliyunProvider) Provision(options pvcontroller.VolumeOptions) (*v1.PersistentVolume, error) {
	if options.PVC.Spec.Selector != nil {
		return nil, fmt.Errorf("claim.Spec.Selector is not supported")
	}

	// Validate access modes
	found := false
	for _, mode := range options.PVC.Spec.AccessModes {
		if mode == v1.ReadWriteOnce {
			found = true
		}
	}
	if !found {
		return nil, fmt.Errorf("Aliyun disk only supports ReadWriteOnce mounts")
	}

	// Get all options
	storageOptions := StorageOptions{
		FSType: DefaultFSType,
	}
	util.MapToStruct(options.Parameters, &storageOptions, "")

	if storageOptions.Category == "" {
		return nil, fmt.Errorf("diskCategory must be specified in storageClass")
	}
	capacity := options.PVC.Spec.Resources.Requests[v1.ResourceName(v1.ResourceStorage)]
	// Convert to G/Gi scale
	size := int(capacity.ScaledValue(resource.Giga))
	log.Infof("Provision disk %v size: %+v %d", capacity, size)

	// Min size
	minSize, ok := MinSizeTable[ecs.DiskCategory(storageOptions.Category)]
	if !ok {
		minSize = 5
	}

	if size < minSize {
		size = minSize
	}
	log.Infof("Min size %d New size: %d", minSize, size)

	// Create disk
	diskId, err := p.client.CreateDisk(&ecs.CreateDiskArgs{
		RegionId:     common.Region(p.region),
		ZoneId:       p.zone,
		DiskCategory: ecs.DiskCategory(storageOptions.Category),
		DiskName:     options.PVName,
		Size:         size,
	})
	if err != nil {
		return nil, fmt.Errorf("Failed to call Aliyun CreateDisk: %v", err)
	}

	storageClassName := ""
	if options.PVC.Spec.StorageClassName != nil {
		storageClassName = *options.PVC.Spec.StorageClassName
	}
	capacity.SetScaled(int64(size), resource.Giga)

	pv := &v1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: options.PVName,
		},
		Spec: v1.PersistentVolumeSpec{
			PersistentVolumeReclaimPolicy: options.PersistentVolumeReclaimPolicy,
			AccessModes:                   []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
			Capacity: v1.ResourceList{
				v1.ResourceName(v1.ResourceStorage): capacity,
			},
			StorageClassName: storageClassName,
			PersistentVolumeSource: v1.PersistentVolumeSource{
				FlexVolume: &v1.FlexVolumeSource{
					Driver: DriverName,
					Options: map[string]string{
						"diskId": diskId,
					},
				},
			},
		},
	}

	return pv, nil
}

func (p *AliyunProvider) Delete(pv *v1.PersistentVolume) error {
	fv := pv.Spec.PersistentVolumeSource.FlexVolume
	if fv == nil {
		return fmt.Errorf("No flexvolume is provided: %+v", pv)
	}

	diskId, _ := fv.Options["diskId"]
	if diskId == "" {
		return fmt.Errorf("No diskId in the flexvolume: %+v", fv)
	}

	err := p.client.DeleteDisk(diskId)
	if err != nil {
		return fmt.Errorf("Error deleting disk: %v", err)
	}

	return nil
}

func (p *AliyunProvider) ProvisionerName() string {
	return ProvisionerName
}
