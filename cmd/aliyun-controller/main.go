package main

import (
	//goflag "flag"
	log "github.com/golang/glog"
	pflag "github.com/spf13/pflag"
	"k8s.io/kubernetes/pkg/util/flag"
	_ "kubeup.com/aliyun-controller/pkg/cloudprovider/providers"
	"kubeup.com/aliyun-controller/pkg/controller"
	"net"
	"net/http"
	"net/http/pprof"
	"strconv"
)

func main() {
	options := &controller.Options{}
	flags := pflag.NewFlagSet("", pflag.ExitOnError)

	profile := flags.Bool("profiling", false, "Enable profiling")
	address := flags.String("profiling_host", "localhost", "Profiling server host")
	port := flags.Int("profiling_port", 9801, "Profiling server port")
	test := flags.Bool("test", false, "Dry-run. To test if the binary is complete")

	//pflag.AddGoFlagSet(goflag.CommandLine)
	options.AddFlags(pflag.CommandLine)
	flag.InitFlags()

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
