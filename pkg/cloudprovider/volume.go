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

package cloudprovider

import (
	"encoding/json"
	"fmt"
	"k8s.io/kubernetes/pkg/volume/flexvolume"
)

// VolumeError is the json response of an volume operation
type VolumeError flexvolume.DriverStatus

func (v VolumeError) Error() string {
	return v.Message
}

// NewVolumeError creates failure error with given message
func NewVolumeError(msg string, args ...interface{}) VolumeError {
	return VolumeError{Message: fmt.Sprintf(msg, args...), Status: flexvolume.StatusFailure}
}

func NewVolumeNotSupported(msg string) VolumeError {
	return VolumeError{Message: msg, Status: flexvolume.StatusNotSupported}
}

func NewVolumeSuccess() VolumeError {
	return VolumeError{Status: flexvolume.StatusSuccess}
}

var (
	VolumeSuccess = NewVolumeSuccess()
)

func (v VolumeError) ToJson() string {
	ret, _ := json.Marshal(&v)
	return string(ret)
}

// Json data
type VolumeOptions map[string]interface{}

// Volume interface of flex volume plugin
type Volume interface {
	Init() error
	Attach(options VolumeOptions, node string) error
	Detach(device string, node string) error
	MountDevice(dir, device string, options VolumeOptions) error
	UnmountDevice(dir string) error
	WaitForAttach(device string, options VolumeOptions) error
	GetVolumeName(options VolumeOptions) error
	IsAttached(options VolumeOptions, node string) error
}
