package cloudprovider

import (
	"encoding/json"
)

// VolumeError is the json response of an volume operation
type VolumeError struct {
	Message string `json:"message"`
	Status  string `json:"status"`
	Device  string `json:"device"`
}

func (v VolumeError) Error() string {
	return v.Message
}

// NewVolumeError creates failure error with given message
func NewVolumeError(msg string) VolumeError {
	return VolumeError{msg, "Failure", ""}
}

var (
	VolumeSuccess = VolumeError{"", "Success", ""}
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
	Attach(options VolumeOptions) error
	Detach(device string) error
	Mount(dir, device string, options VolumeOptions) error
	Unmount(dir string) error
}
