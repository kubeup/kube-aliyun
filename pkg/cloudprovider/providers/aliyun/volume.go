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
	"k8s.io/kubernetes/pkg/util/exec"
	"k8s.io/kubernetes/pkg/util/mount"
	"path"
	//"log"
	"os"
	"strings"
	//"syscall"
	"kubeup.com/kube-aliyun/pkg/cloudprovider"
	"time"
)

var (
	optionFSType    = "kubernetes.io/fsType"
	optionReadWrite = "kubernetes.io/readwrite"

	letters     = "abcdefghijklmnopqrstuvwxyz"
	apiPrefix   = "/dev/xvd"
	localPrefix = "/dev/vd"

	WaitInterval         = time.Second
	WaitForAttachTimeout = 10 * time.Second
)

var _ cloudprovider.Volume = &AliyunProvider{}

// TODO: Use nil as success result

// Aliyun api only takes /dev/xvd? even though it's actuall vd? in coreos system
func deviceApi2Local(s string) string {
	return strings.Replace(s, apiPrefix, localPrefix, 1)
}

func deviceLocal2Api(s string) string {
	return strings.Replace(s, localPrefix, apiPrefix, 1)
}

func probeLocalDevicePath() error {
	prefixes := []string{"/dev/xvd", "/dev/vd", "/dev/sd"}
	for _, p := range prefixes {
		if _, err := os.Stat(p + "a"); !os.IsNotExist(err) {
			localPrefix = p
			return nil
		}
	}
	return cloudprovider.NewVolumeError("Unable to get a proper local device name")
}

func getAvailableDevicePath() string {
	for _, b := range letters {
		path := localPrefix + string(b)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return path
		}
	}

	return ""
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
	if err := probeLocalDevicePath(); err != nil {
		return err
	}

	fstype, _ := options[optionFSType].(string)
	//data, _ := options["data"].(string)
	readwrite, _ := options[optionReadWrite].(string)
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
