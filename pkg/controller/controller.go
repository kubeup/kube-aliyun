package controller

import (
	log "github.com/golang/glog"
	"github.com/spf13/pflag"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/api/unversioned"
	"k8s.io/kubernetes/pkg/apis/componentconfig"
	kubernetes "k8s.io/kubernetes/pkg/client/clientset_generated/internalclientset"
	unversionedcore "k8s.io/kubernetes/pkg/client/clientset_generated/internalclientset/typed/core/internalversion"
	"k8s.io/kubernetes/pkg/client/leaderelection"
	"k8s.io/kubernetes/pkg/client/leaderelection/resourcelock"
	"k8s.io/kubernetes/pkg/client/record"
	"k8s.io/kubernetes/pkg/client/restclient"
	"k8s.io/kubernetes/pkg/client/unversioned/clientcmd"
	clientcmdapi "k8s.io/kubernetes/pkg/client/unversioned/clientcmd/api"
	"k8s.io/kubernetes/pkg/controller/route"
	"k8s.io/kubernetes/pkg/controller/service"
	"kubeup.com/aliyun-controller/pkg/cloudprovider"
	"net"
	"os"
	"time"
)

type Options struct {
	Name                     string
	ClusterCIDR              string
	InCluster                bool
	ConcurrentServiceSyncs   int
	ShutdownGracePeriod      unversioned.Duration
	RouteReconcilationPeriod unversioned.Duration

	LeaderElection componentconfig.LeaderElectionConfiguration `json:"leaderElection"`
	Overrides      clientcmd.ConfigOverrides
}

func (o *Options) AddFlags(ps *pflag.FlagSet) {
	id, _ := os.Hostname()
	ps.StringVar(&o.ClusterCIDR, "cluster-cidr", "", "Pod CIDR range")
	ps.StringVar(&o.Name, "instance-name", id, "Name of the instance")
	ps.BoolVar(&o.InCluster, "in-cluster", false, "If the controller is running in a pod")
	ps.IntVar(&o.ConcurrentServiceSyncs, "concurrent-service-syncs", 3, "Concurrent service syncs")
	ps.DurationVar(&o.ShutdownGracePeriod.Duration, "shutdown-grace-period", 3*time.Second, "Shutdown grace period")
	ps.DurationVar(&o.RouteReconcilationPeriod.Duration, "route-reconcilation-period", 30*time.Second, "Route reconcilation period")

	leaderelection.BindFlags(&o.LeaderElection, ps)
	overrideFlags := clientcmd.RecommendedConfigOverrideFlags("")
	clientcmd.BindOverrideFlags(&o.Overrides, ps, overrideFlags)
}

// Controller is the actual entry of archond. It setups leader election, watches
// for TPR changes, updates VPC routes and etcd accordingly and runs service
// controller for LB
type Controller struct {
	*Options

	provider cloudprovider.Provider
	k8s      kubernetes.Interface
	sc       *service.ServiceController
	rc       *route.RouteController

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

	// LB
	sc, err := service.New(p, k8s, options.Overrides.Context.Cluster)
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
	rc := route.New(routes, k8s, options.Overrides.Context.Cluster, clusterCIDR)

	return &Controller{
		Options:  options,
		provider: p,
		k8s:      k8s,
		sc:       sc,
		rc:       rc,
		done:     false,
	}, nil
}

// Run starts leader election, service controller and main loop
func (c *Controller) Run() error {
	eventBroadcaster := record.NewBroadcaster()
	recorder := eventBroadcaster.NewRecorder(api.EventSource{Component: c.Name})
	eventBroadcaster.StartLogging(log.Infof)
	eventBroadcaster.StartRecordingToSink(&unversionedcore.EventSinkImpl{Interface: c.k8s.Core().Events("")})

	run := func(done <-chan struct{}) {
		log.Infof("Controller is leading. Starting loop")
		go c.sc.Run(c.Options.ConcurrentServiceSyncs)
		go c.rc.Run(c.Options.RouteReconcilationPeriod.Duration)

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
		EndpointsMeta: api.ObjectMeta{
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
