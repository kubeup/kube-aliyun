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
	"errors"
	"net/http"
	"os"
	"time"

	"github.com/denverdino/aliyungo/common"
	"github.com/denverdino/aliyungo/ecs"
	"github.com/denverdino/aliyungo/metadata"
	"github.com/denverdino/aliyungo/slb"
	log "github.com/golang/glog"
	api "k8s.io/kubernetes/pkg/api/v1"
	origcloudprovider "k8s.io/kubernetes/pkg/cloudprovider"
	"kubeup.com/kube-aliyun/pkg/cloudprovider"
)

const (
	AliyunAnnotationPrefix = "aliyun.archon.kubeup.com/"
	ProviderName           = "aliyun"
)

type AliyunProvider struct {
	accessKey       string
	accessKeySecret string
	region          string
	zone            string
	vpcID           string
	vrouterID       string
	vswitch         string
	routeTable      string
	loadbalancer    string
	instance        string
	hostname        string
	nodeNameType    NodeNameType

	client    *ecs.Client
	slbClient *slb.Client
	meta      *metadata.MetaData
}

// NodeName describes the format of name for nodes in the cluster
type NodeNameType string

const (
	// Use private ip address as the node name
	NodeNameTypePrivateIP NodeNameType = "private-ip"
	// Use hostname as the node name. This is the default behavior for k8s
	NodeNameTypeHostname NodeNameType = "hostname"
)

var _ origcloudprovider.Interface = &AliyunProvider{}

func init() {
	cloudprovider.RegisterProvider("aliyun", NewProvider)
}

func NewProvider() cloudprovider.Provider {
	accessKey := os.Getenv("ALIYUN_ACCESS_KEY")
	accessKeySecret := os.Getenv("ALIYUN_ACCESS_KEY_SECRET")
	debug := os.Getenv("ALIYUN_DEBUG")
	var nodeNameType NodeNameType
	switch NodeNameType(os.Getenv("ALIYUN_NODE_NAME_TYPE")) {
	case NodeNameTypePrivateIP:
		nodeNameType = NodeNameTypePrivateIP
	case NodeNameTypeHostname:
		nodeNameType = NodeNameTypeHostname
	default:
		// Default to NodeNameTypePrivateIP for backward compatibility
		nodeNameType = NodeNameTypePrivateIP
	}
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
		zone:            os.Getenv("ALIYUN_ZONE"),
		vrouterID:       os.Getenv("ALIYUN_ROUTER"),
		vswitch:         os.Getenv("ALIYUN_VSWITCH"),
		routeTable:      os.Getenv("ALIYUN_ROUTE_TABLE"),
		instance:        os.Getenv("ALIYUN_INSTANCE"),
		accessKey:       accessKey,
		accessKeySecret: accessKeySecret,
		hostname:        hostname,
		nodeNameType:    nodeNameType,
	}

	if debug == "true" {
		p.client.SetDebug(true)
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

func (w *AliyunProvider) getInstances() (instances []ecs.InstanceAttributesType, err error) {
	args := &ecs.DescribeInstancesArgs{
		RegionId: common.Region(w.region),
		VpcId:    w.vpcID,
	}
	instances, _, err = w.client.DescribeInstances(args)
	return
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

func (p *AliyunProvider) mapInstanceToNodeName(instance ecs.InstanceAttributesType) string {
	switch p.nodeNameType {
	case NodeNameTypePrivateIP:
		return instance.VpcAttributes.PrivateIpAddress.IpAddress[0]
	case NodeNameTypeHostname:
		return instance.HostName
	}
	return instance.HostName
}

func (p *AliyunProvider) getInstanceByNodeName(nodeName string) (instance ecs.InstanceAttributesType, err error) {
	instances, err := p.getInstances()
	if err != nil {
		return
	}

	switch p.nodeNameType {
	case NodeNameTypePrivateIP:
		instances = filter(instances, byPrivateIP(nodeName))
	case NodeNameTypeHostname:
		instances = filter(instances, byHostname(nodeName))
	default:
		instances = []ecs.InstanceAttributesType{}
	}

	if len(instances) == 0 {
		err = errors.New("Unable to get instance of node:" + nodeName)
		return
	}

	if len(instances) > 1 {
		err = errors.New("Multiple instance with the same node name:" + nodeName)
		return
	}

	return instances[0], nil
}

func (p *AliyunProvider) getInstancesByNodeNames(nodeNames []string) (results []ecs.InstanceAttributesType, err error) {
	instances, err := p.getInstances()
	if err != nil {
		return
	}

	var by func(string) func(ecs.InstanceAttributesType) bool
	switch p.nodeNameType {
	case NodeNameTypePrivateIP:
		by = byPrivateIP
	case NodeNameTypeHostname:
		by = byHostname
	default:
		by = byEmpty
	}

	for _, nodeName := range nodeNames {
		instances = filter(instances, by(nodeName))

		if len(instances) == 0 {
			return nil, errors.New("Unable to get instance of node:" + nodeName)
		}

		if len(instances) > 1 {
			return nil, errors.New("Multiple instance with the same node name:" + nodeName)
		}
		results = append(results, instances[0])
	}

	return
}

func (p *AliyunProvider) getInstancesByNodes(nodes []*api.Node) (results []ecs.InstanceAttributesType, err error) {
	var nodeNames []string

	for _, n := range nodes {
		nodeNames = append(nodeNames, n.Name)
	}
	return p.getInstancesByNodeNames(nodeNames)
}

func filter(instances []ecs.InstanceAttributesType, f func(ecs.InstanceAttributesType) bool) (ret []ecs.InstanceAttributesType) {
	for _, i := range instances {
		if f(i) {
			ret = append(ret, i)
		}
	}
	return
}

func byHostname(hostname string) func(ecs.InstanceAttributesType) bool {
	return func(i ecs.InstanceAttributesType) bool {
		return i.HostName == hostname
	}
}

func byPrivateIP(ip string) func(ecs.InstanceAttributesType) bool {
	return func(i ecs.InstanceAttributesType) bool {
		for _, s := range i.VpcAttributes.PrivateIpAddress.IpAddress {
			if s == ip {
				return true
			}
		}
		return false
	}
}

func byInstanceId(id string) func(ecs.InstanceAttributesType) bool {
	return func(i ecs.InstanceAttributesType) bool {
		return i.InstanceId == id
	}
}

func byEmpty(string) func(ecs.InstanceAttributesType) bool {
	return func(ecs.InstanceAttributesType) bool {
		return false
	}
}
