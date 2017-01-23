package aliyun

import (
	"os"

	"github.com/denverdino/aliyungo/common"
	"github.com/denverdino/aliyungo/ecs"
	"github.com/denverdino/aliyungo/slb"
	origcloudprovider "k8s.io/kubernetes/pkg/cloudprovider"
	"kubeup.com/aliyun-controller/pkg/cloudprovider"
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

	client    *ecs.Client
	slbClient *slb.Client

	OverwriteMismatch bool
}

var _ origcloudprovider.Interface = &AliyunProvider{}

func init() {
	cloudprovider.RegisterProvider("aliyun", NewProvider)
}

func NewProvider() cloudprovider.Provider {
	accessKey := os.Getenv("ALIYUN_ACCESS_KEY")
	accessKeySecret := os.Getenv("ALIYUN_ACCESS_KEY_SECRET")

	p := &AliyunProvider{
		client:       ecs.NewClient(accessKey, accessKeySecret),
		slbClient:    slb.NewClient(accessKey, accessKeySecret),
		region:       os.Getenv("ALIYUN_REGION"),
		vpcID:        os.Getenv("ALIYUN_VPC"),
		vrouterID:    os.Getenv("ALIYUN_ROUTER"),
		vswitch:      os.Getenv("ALIYUN_VSWITCH"),
		routeTable:   os.Getenv("ALIYUN_ROUTE_TABLE"),
		loadbalancer: os.Getenv("ALIYUN_LOADBALANCER"),
		instance:     os.Getenv("VPSID"),

		OverwriteMismatch: true,
	}
	return p
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

func (p *AliyunProvider) Clusters() (origcloudprovider.Clusters, bool) {
	return nil, false
}

func (p *AliyunProvider) Zones() (origcloudprovider.Zones, bool) {
	return p, false
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
