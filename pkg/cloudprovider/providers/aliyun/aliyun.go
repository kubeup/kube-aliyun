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
	"github.com/denverdino/aliyungo/common"
	"github.com/denverdino/aliyungo/ecs"
	"github.com/denverdino/aliyungo/metadata"
	"github.com/denverdino/aliyungo/slb"
	log "github.com/golang/glog"
	api "k8s.io/kubernetes/pkg/api/v1"
	origcloudprovider "k8s.io/kubernetes/pkg/cloudprovider"
	"kubeup.com/kube-aliyun/pkg/cloudprovider"
	"net/http"
	"os"
	"time"
)

const (
	AliyunAnnotationPrefix = "aliyun.archon.kubeup.com/"
	ProviderName           = "aliyun"
)

type AliyunProvider struct {
	accessKey       string
	accessKeySecret string
	region          string
	vpcID           string
	vrouterID       string
	vswitch         string
	routeTable      string
	loadbalancer    string
	instance        string
	hostname        string

	client    *ecs.Client
	slbClient *slb.Client
	meta      *metadata.MetaData
}

var _ origcloudprovider.Interface = &AliyunProvider{}

func init() {
	cloudprovider.RegisterProvider("aliyun", NewProvider)
}

func NewProvider() cloudprovider.Provider {
	accessKey := os.Getenv("ALIYUN_ACCESS_KEY")
	accessKeySecret := os.Getenv("ALIYUN_ACCESS_KEY_SECRET")
	hostname, _ := os.Hostname()
	httpClient := &http.Client{
		Timeout: time.Duration(3) * time.Second,
	}

	p := &AliyunProvider{
		client:          ecs.NewClient(accessKey, accessKeySecret),
		slbClient:       slb.NewClient(accessKey, accessKeySecret),
		meta:            metadata.NewMetaData(httpClient),
		region:          os.Getenv("ALIYUN_REGION"),
		vpcID:           os.Getenv("ALIYUN_VPC"),
		vrouterID:       os.Getenv("ALIYUN_ROUTER"),
		vswitch:         os.Getenv("ALIYUN_VSWITCH"),
		routeTable:      os.Getenv("ALIYUN_ROUTE_TABLE"),
		instance:        os.Getenv("ALIYUN_INSTANCE"),
		accessKey:       accessKey,
		accessKeySecret: accessKeySecret,
		hostname:        hostname,
	}

	metaFailed := false
	if p.region == "" {
		var err error
		p.region, err = p.meta.Region()
		if err != nil {
			metaFailed = true
		}
	}

	if p.instance == "" && !metaFailed {
		p.instance, _ = p.meta.InstanceID()
	}

	if p.vpcID == "" && !metaFailed {
		p.vpcID, _ = p.meta.VpcID()
	}

	if p.vswitch == "" && !metaFailed {
		p.vswitch, _ = p.meta.VswitchID()
	}

	if accessKey == "" || accessKeySecret == "" {
		log.V(2).Infof("ALIYUN_ACCESS_KEY && ALIYUN_ACCESS_KEY_SECRET are required")
	}

	if p.region == "" || p.vpcID == "" || p.vrouterID == "" || p.routeTable == "" {
		log.V(2).Infof(`ALIYUN_REGION, ALIYUN_VPC, ALIYUN_ROUTER, ALIYUN_VSWITCH, ALIYUN_ROUTE_TABLE
		are required for service and route controllers`)
	}

	if p.instance == "" {
		log.V(2).Infof("ALIYUN_REGION, ALIYUN_INSTANCE are required for flexv")
	}

	return p
}

func (w *AliyunProvider) authorized() bool {
	return w.accessKey != "" && w.accessKeySecret != ""
}

func (w *AliyunProvider) isLocal(node string) bool {
	return node == "localhost" || node == w.hostname
}

func (w *AliyunProvider) getInstanceIP2ID() (ip2id map[string]string, err error) {
	args2 := &ecs.DescribeInstancesArgs{
		RegionId: common.Region(w.region),
		VpcId:    w.vpcID,
	}
	results2, _, err := w.client.DescribeInstances(args2)
	if err != nil {
		return
	}

	ip2id = make(map[string]string)
	for _, instance := range results2 {
		if len(instance.VpcAttributes.PrivateIpAddress.IpAddress) > 0 {
			ip2id[instance.VpcAttributes.PrivateIpAddress.IpAddress[0]] = instance.InstanceId
		}
	}
	return
}

func (w *AliyunProvider) getInstanceID2IP() (id2ip map[string]string, err error) {
	args2 := &ecs.DescribeInstancesArgs{
		RegionId: common.Region(w.region),
		VpcId:    w.vpcID,
	}
	results2, _, err := w.client.DescribeInstances(args2)
	if err != nil {
		return
	}

	id2ip = make(map[string]string)
	for _, instance := range results2 {
		if len(instance.VpcAttributes.PrivateIpAddress.IpAddress) > 0 {
			id2ip[instance.InstanceId] = instance.VpcAttributes.PrivateIpAddress.IpAddress[0]
		}
	}
	return
}

func (p *AliyunProvider) getInstanceIdsByNodeNames(nodes []string) (instances []string, err error) {
	args := &ecs.DescribeInstancesArgs{
		RegionId: common.Region(p.region),
		VpcId:    p.vpcID,
	}
	results, _, err := p.client.DescribeInstances(args)
	if err != nil {
		return
	}

	// Match hostnames and all ip addresses
	instanceMap := make(map[string]string)
	for _, i := range results {
		id := i.InstanceId
		instanceMap[i.HostName] = id

		for _, ip := range i.VpcAttributes.PrivateIpAddress.IpAddress {
			instanceMap[ip] = id
		}

		for _, ip := range i.PublicIpAddress.IpAddress {
			instanceMap[ip] = id
		}

		if i.EipAddress.IpAddress != "" {
			instanceMap[i.EipAddress.IpAddress] = id
		}
	}

	for _, node := range nodes {
		if id, ok := instanceMap[node]; ok {
			instances = append(instances, id)
		}
	}

	return
}

func (p *AliyunProvider) getInstanceIdsByNodes(nodes []*api.Node) (instances []string, err error) {
	names := []string{}
	for _, node := range nodes {
		names = append(names, node.Name)
	}

	return p.getInstanceIdsByNodeNames(names)
}

func (p *AliyunProvider) Clusters() (origcloudprovider.Clusters, bool) {
	return nil, false
}

func (p *AliyunProvider) Zones() (origcloudprovider.Zones, bool) {
	return p, true
}

func (p *AliyunProvider) Instances() (origcloudprovider.Instances, bool) {
	return nil, false
}

func (p *AliyunProvider) ProviderName() string {
	return ProviderName
}

func (p *AliyunProvider) Routes() (origcloudprovider.Routes, bool) {
	return p, true
}

func (p *AliyunProvider) LoadBalancer() (origcloudprovider.LoadBalancer, bool) {
	return p, true
}

func (p *AliyunProvider) Volume() (cloudprovider.Volume, bool) {
	return p, true
}

func (p *AliyunProvider) ScrubDNS(nameservers, searches []string) (nsOut, srchOut []string) {
	return
}
