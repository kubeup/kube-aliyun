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

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"kubeup.com/kube-aliyun/pkg/cloudprovider"
	_ "kubeup.com/kube-aliyun/pkg/cloudprovider/providers"
	"os"
)

// fatalf is a convenient method that outputs error in flex volume plugin style
// and quits
func fatalf(msg string, args ...interface{}) {
	err := cloudprovider.VolumeError{
		Message: fmt.Sprintf(msg, args...),
		Status:  "Failure",
	}
	fmt.Printf(err.ToJson())
	os.Exit(1)
}

// printResult is a convenient method for printing result of volume operation
func printResult(err error) {
	if err == nil {
		err = cloudprovider.VolumeSuccess
	}
	ve, ok := err.(cloudprovider.VolumeError)
	if !ok {
		fatalf("Unknown error: %v", err)
	}

	fmt.Printf(ve.ToJson())
	if ve.Status == "Success" {
		os.Exit(0)
	}
	os.Exit(1)
}

// ensureVolumeOptions decodes json or die
func ensureVolumeOptions(v string) (vo cloudprovider.VolumeOptions) {
	err := json.Unmarshal([]byte(v), &vo)
	if err != nil {
		fatalf("Invalid json options: %s", v)
	}
	return
}

func main() {
	// Used in downloader. To test if the binary is complete
	test := flag.Bool("test", false, "Dry run. To test if the binary is complete")

	// Prepare logs
	os.MkdirAll("/opt/logs/flexv", 0750)
	//log.SetOutput(os.Stderr)
	flag.Parse()
	flag.Set("logtostderr", "true")
	flag.Set("alsologtostderr", "false")
	flag.Set("log_dir", "/opt/logs/flexv")
	flag.Set("stderrThreshold", "fatal")

	if *test {
		return
	}

	pp, err := cloudprovider.GetProvider("aliyun")
	if err != nil {
		fatalf("Error getting provider")
	}

	p, _ := pp.Volume()
	if p == nil {
		fatalf("Provider doesn't support volume")
	}

	args := flag.Args()
	if len(args) == 0 {
		fatalf("Usage: %s init|attach|detach|mount|unmount", os.Args[0])
	}

	var ret error
	op := args[0]
	args = args[1:]
	switch op {
	case "init":
		ret = p.Init()
	case "attach":
		if len(args) < 1 {
			fatalf("attach requires options in json format")
		}
		ret = p.Attach(ensureVolumeOptions(args[0]))
	case "detach":
		if len(args) < 1 {
			fatalf("detach requires a device path")
		}
		ret = p.Detach(args[0])
	case "mount":
		if len(args) < 3 {
			fatalf("mount requires a mount path, a device path and mount options")
		}
		ret = p.Mount(args[0], args[1], ensureVolumeOptions(args[2]))
	case "unmount":
		if len(args) < 1 {
			fatalf("unmount requires a mount path")
		}
		ret = p.Unmount(args[0])
	default:
		fatalf("Unknown command: %s", op)
	}

	printResult(ret)
}
