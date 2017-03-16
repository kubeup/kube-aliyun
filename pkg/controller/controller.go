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

package controller

import (
	log "github.com/golang/glog"
	pvcontroller "github.com/kubernetes-incubator/external-storage/lib/controller"
	pvleaderelection "github.com/kubernetes-incubator/external-storage/lib/leaderelection"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientkubernetes "k8s.io/client-go/kubernetes"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	clientv1 "k8s.io/client-go/pkg/api/v1"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/client-go/tools/record"
	"k8s.io/kubernetes/pkg/api"
	kubernetes "k8s.io/kubernetes/pkg/client/clientset_generated/clientset"
	informers "k8s.io/kubernetes/pkg/client/informers/informers_generated/externalversions"
	"k8s.io/kubernetes/pkg/client/leaderelection"
	"k8s.io/kubernetes/pkg/client/leaderelection/resourcelock"
	"k8s.io/kubernetes/pkg/controller/route"
	"k8s.io/kubernetes/pkg/controller/service"
	"kubeup.com/kube-aliyun/pkg/cloudprovider"
	"net"
	"time"
)

var (
	ResyncPeriod = 30 * time.Second
)

// Controller is the actual entry of archond. It setups leader election, watches
// for TPR changes, updates VPC routes and etcd accordingly and runs service
// controller for LB
type Controller struct {
	*Options

	provider cloudprovider.Provider
	k8s      kubernetes.Interface
	sc       *service.ServiceController
	rc       *route.RouteController
	pvc      *pvcontroller.ProvisionController

	done bool
}

// NewController creates the Controller instance and does necessary initializaiton
func NewController(options *Options) (*Controller, error) {
	var (
		clientConfig *restclient.Config
	)

	p, err := cloudprovider.GetProvider("aliyun")
	if err != nil {
		return nil, err
	}

	// Create kubeconfig
	if options.InCluster {
		clientConfig, err = restclient.InClusterConfig()
	} else if options.Kubeconfig != "" {
		clientConfig, err = clientcmd.BuildConfigFromFlags(options.Overrides.ClusterInfo.Server, options.Kubeconfig)
	} else {
		kubeconfig := clientcmd.NewDefaultClientConfig(*clientcmdapi.NewConfig(), &options.Overrides)
		clientConfig, err = kubeconfig.ClientConfig()
	}

	if err != nil {
		log.Fatalf("Unable to create config: %+v", err)
		return nil, err
	}

	// Create kubeclient
	k8s, err := kubernetes.NewForConfig(clientConfig)
	if err != nil {
		log.Fatalf("Invalid api configuration: %+v", err)
	}

	sharedInformers := informers.NewSharedInformerFactory(k8s, ResyncPeriod)

	// LB
	sc, err := service.New(
		p,
		k8s,
		sharedInformers.Core().V1().Services(),
		sharedInformers.Core().V1().Nodes(),
		options.Overrides.Context.Cluster)
	if err != nil {
		log.Fatalf("Can't initialize service controller: %v", err)
	}

	// Routes
	routes, _ := p.Routes()
	if routes == nil {
		log.Fatalf("Provider doesn't support routes")
	}
	_, clusterCIDR, err := net.ParseCIDR(options.ClusterCIDR)
	if err != nil {
		log.Fatalf("Invalid cidr")
	}
	rc := route.New(
		routes,
		k8s,
		sharedInformers.Core().V1().Nodes(),
		options.Overrides.Context.Cluster,
		clusterCIDR)

	// PVC controller
	clientk8s, err := clientkubernetes.NewForConfig(clientConfig)
	if err != nil {
		log.Fatalf("Invalid api configuration: %+v", err)
	}

	var pvc *pvcontroller.ProvisionController
	volume, ok := p.Volume()
	if !ok {
		log.Warningf("Provider doesn't support volume. Provision controller will not work")
	} else {
		serverVersion, err := clientk8s.Discovery().ServerVersion()
		if err != nil {
			log.Fatalf("Error getting server version: %v", err)
		}

		ldConfig := options.LeaderElection
		pvc = pvcontroller.NewProvisionController(clientk8s, ResyncPeriod, volume.ProvisionerName(), volume, serverVersion.GitVersion, false,
			options.ProvisionRetry,
			ldConfig.LeaseDuration.Duration,
			ldConfig.RenewDeadline.Duration,
			ldConfig.RetryPeriod.Duration,
			pvleaderelection.DefaultTermLimit)
	}

	return &Controller{
		Options:  options,
		provider: p,
		k8s:      k8s,
		sc:       sc,
		rc:       rc,
		pvc:      pvc,
		done:     false,
	}, nil
}

// Run starts leader election, service controller and main loop
func (c *Controller) Run() error {
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(log.Infof)
	eventBroadcaster.StartRecordingToSink(&v1core.EventSinkImpl{Interface: v1core.New(c.k8s.Core().RESTClient()).Events("")})
	recorder := eventBroadcaster.NewRecorder(api.Scheme, clientv1.EventSource{Component: c.Name})

	run := func(done <-chan struct{}) {
		log.Infof("Controller is leading. Starting loop")
		go c.sc.Run(done, c.Options.ConcurrentServiceSyncs)
		go c.rc.Run(done, c.Options.RouteReconcilationPeriod.Duration)
		go c.pvc.Run(done)

		select {
		case <-done:
			break
		}
		log.Fatal("Lost lead. Shutting down")
	}

	if !c.LeaderElection.LeaderElect {
		run(nil)
		panic("unreachable")
	}

	log.Infof("Leader election initiated. Waiting to take the lead...")
	rl := resourcelock.EndpointsLock{
		EndpointsMeta: metav1.ObjectMeta{
			Namespace: "kube-system",
			Name:      "aliyun-controller",
		},
		Client: c.k8s,
		LockConfig: resourcelock.ResourceLockConfig{
			Identity:      c.Name,
			EventRecorder: recorder,
		},
	}

	lconfig := leaderelection.LeaderElectionConfig{
		Lock:          &rl,
		LeaseDuration: c.Options.LeaderElection.LeaseDuration.Duration,
		RenewDeadline: c.Options.LeaderElection.RenewDeadline.Duration,
		RetryPeriod:   c.Options.LeaderElection.RetryPeriod.Duration,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: run,
			OnStoppedLeading: func() {
				c.done = true
				time.Sleep(c.Options.ShutdownGracePeriod.Duration)
				log.Fatalf("Lost lead")
			},
		},
	}

	leaderelection.RunOrDie(lconfig)

	log.Fatal("Unreachable")
	return nil
}
