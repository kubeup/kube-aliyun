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

	"github.com/denverdino/aliyungo/common"
	"github.com/denverdino/aliyungo/ecs"
	api "k8s.io/kubernetes/pkg/api/v1"
)

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
