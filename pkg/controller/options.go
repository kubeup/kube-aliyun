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
	"github.com/spf13/pflag"
	"k8s.io/kubernetes/pkg/api/unversioned"
	"k8s.io/kubernetes/pkg/apis/componentconfig"
	"k8s.io/kubernetes/pkg/client/leaderelection"
	"k8s.io/kubernetes/pkg/client/unversioned/clientcmd"
	"os"
	"time"
)

type Options struct {
	Name                     string
	Kubeconfig               string
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
	ps.StringVar(&o.Kubeconfig, "kubeconfig", o.Kubeconfig, "Path to kubeconfig file with authorization and master location information.")

	leaderelection.BindFlags(&o.LeaderElection, ps)
	overrideFlags := clientcmd.RecommendedConfigOverrideFlags("")
	clientcmd.BindOverrideFlags(&o.Overrides, ps, overrideFlags)
}

func NewOptions() *Options {
	return &Options{
		LeaderElection: leaderelection.DefaultLeaderElectionConfiguration(),
	}
}
