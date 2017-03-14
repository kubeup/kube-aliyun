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
	//goflag "flag"
	log "github.com/golang/glog"
	pflag "github.com/spf13/pflag"
	"k8s.io/apiserver/pkg/util/flag"
	"k8s.io/apiserver/pkg/util/logs"
	_ "kubeup.com/kube-aliyun/pkg/cloudprovider/providers"
	"kubeup.com/kube-aliyun/pkg/controller"
	"net"
	"net/http"
	"net/http/pprof"
	"strconv"
)

func main() {
	options := controller.NewOptions()
	flags := pflag.NewFlagSet("", pflag.ExitOnError)

	profile := flags.Bool("profiling", false, "Enable profiling")
	address := flags.String("profiling_host", "localhost", "Profiling server host")
	port := flags.Int("profiling_port", 9801, "Profiling server port")
	test := flags.Bool("test", false, "Dry-run. To test if the binary is complete")

	//pflag.AddGoFlagSet(goflag.CommandLine)
	options.AddFlags(pflag.CommandLine)
	flag.InitFlags()
	logs.InitLogs()
	defer logs.FlushLogs()

	if *test {
		return
	}

	c, err := controller.NewController(options)
	if err != nil {
		panic(err.Error())
	}

	if *profile {
		go func() {
			mux := http.NewServeMux()
			mux.HandleFunc("/debug/pprof/", pprof.Index)
			mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
			mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)

			server := &http.Server{
				Addr:    net.JoinHostPort(*address, strconv.Itoa(*port)),
				Handler: mux,
			}
			log.Fatalf("%+v", server.ListenAndServe())
		}()
	}
	c.Run()
}
