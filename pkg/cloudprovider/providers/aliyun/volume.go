package aliyun

import (
	"errors"
	"fmt"
	"github.com/denverdino/aliyungo/common"
	"github.com/denverdino/aliyungo/ecs"
	"k8s.io/kubernetes/pkg/util/exec"
	"k8s.io/kubernetes/pkg/util/mount"
	//"log"
	"os"
	"strings"
	//"syscall"
	"kubeup.com/aliyun-controller/pkg/cloudprovider"
)

var (
	letters     = "abcdefghijklmnopqrstuvwxyz"
	localPrefix = "/dev/xvd"
)

// TODO: Use nil as success result

// Aliyun api only takes /dev/xvd? even though it's actuall vd? in coreos system
func deviceApi2Local(s string) string {
	return strings.Replace(s, "/dev/xvd", localPrefix, 1)
}

func deviceLocal2Api(s string) string {
	return strings.Replace(s, localPrefix, "/dev/xvd", 1)
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
		InstanceId: p.instance,
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

	err = errors.New(fmt.Sprintf("DiskId %s is not found with this instance", diskId))
	return
}

func (p *AliyunProvider) getDiskByDevice(device string) (d ecs.DiskItemType, err error) {
	pg := &common.Pagination{
		PageNumber: 0,
		PageSize:   50,
	}
	for {
		args := &ecs.DescribeDisksArgs{
			RegionId:   common.Region(p.region),
			InstanceId: p.instance,
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

func (p *AliyunProvider) Attach(options cloudprovider.VolumeOptions) error {
	if err := probeLocalDevicePath(); err != nil {
		return err
	}

	if p.instance == "" {
		return cloudprovider.NewVolumeError("instanceId is required")
	}

	diskId, _ := options["diskId"].(string)
	if diskId == "" {
		return cloudprovider.NewVolumeError("diskid is required")
	}

	// Optional
	device, _ := options["device"].(string)
	deleteWithInstance, _ := options["deleteWithInstance"].(bool)

	if device == "" {
		device = getAvailableDevicePath()
		if device == "" {
			return cloudprovider.NewVolumeError("Unable to find an available device path")
		}
	}

	// Check if already attached
	disk, err := p.getDiskById(diskId)
	if err == nil && disk.InstanceId == p.instance && disk.DiskId == diskId && disk.Status == ecs.DiskStatusInUse {
		return cloudprovider.VolumeError{
			Status: "Success",
			Device: deviceApi2Local(disk.Device),
		}
	}

	args := &ecs.AttachDiskArgs{
		InstanceId:         p.instance,
		DiskId:             diskId,
		Device:             deviceLocal2Api(device),
		DeleteWithInstance: deleteWithInstance,
	}
	err = p.client.AttachDisk(args)
	if err != nil {
		return cloudprovider.VolumeError{
			err.Error(),
			"Failure",
			device,
		}
	}
	region := common.Region(p.region)
	p.client.WaitForDisk(region, diskId, ecs.DiskStatusInUse, 0)
	return cloudprovider.VolumeError{
		Status: "Success",
		Device: device,
	}
}

func (p *AliyunProvider) Detach(device string) error {
	if err := probeLocalDevicePath(); err != nil {
		return err
	}

	if p.instance == "" {
		return cloudprovider.NewVolumeError("instanceId is required")
	}

	disk, err := p.getDiskByDevice(device)
	if err != nil {
		return cloudprovider.NewVolumeError(err.Error())
	}

	err = p.client.DetachDisk(p.instance, disk.DiskId)
	if err != nil {
		return cloudprovider.NewVolumeError(err.Error())
	}

	return cloudprovider.VolumeSuccess
}

func (p *AliyunProvider) Mount(dir, device string, options cloudprovider.VolumeOptions) error {
	if err := probeLocalDevicePath(); err != nil {
		return err
	}

	fstype, _ := options["kubernetes.io/fsType"].(string)
	//data, _ := options["data"].(string)
	flags, _ := options["flags"].(string)

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err = os.MkdirAll(dir, 0750); err != nil {
			return cloudprovider.NewVolumeError(err.Error())
		}
	}

	mounter := &mount.SafeFormatAndMount{Interface: mount.New(""), Runner: exec.New()}
	err := mounter.FormatAndMount(device, dir, fstype, strings.Split(flags, ","))
	if err != nil {
		return cloudprovider.NewVolumeError(err.Error())
	}

	return cloudprovider.VolumeSuccess
}

func (p *AliyunProvider) Unmount(dir string) error {
	mounter := &mount.SafeFormatAndMount{Interface: mount.New(""), Runner: exec.New()}
	err := mounter.Unmount(dir)
	if err != nil {
		return cloudprovider.NewVolumeError(err.Error())
	}
	return cloudprovider.VolumeSuccess
}
