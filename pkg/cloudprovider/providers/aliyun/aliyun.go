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
	"net/http"
	"os"
	"time"

	"github.com/denverdino/aliyungo/ecs"
	"github.com/denverdino/aliyungo/metadata"
	"github.com/denverdino/aliyungo/slb"
	log "github.com/golang/glog"
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
