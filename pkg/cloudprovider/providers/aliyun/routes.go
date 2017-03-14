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
	log "github.com/golang/glog"
	"k8s.io/apimachinery/pkg/types"
	origcloudprovider "k8s.io/kubernetes/pkg/cloudprovider"
)

func nodeName2IP(s types.NodeName) string {
	return string(s)
}

func ip2NodeName(s string) types.NodeName {
	return types.NodeName(s)
}

func (w *AliyunProvider) ListRoutes(clusterName string) (routes []*origcloudprovider.Route, err error) {
	id2ip, err := w.getInstanceID2IP()
	if err != nil {
		return
	}

	tables, _, err := w.client.DescribeRouteTables(&ecs.DescribeRouteTablesArgs{
		VRouterId:    w.vrouterID,
		RouteTableId: w.routeTable,
		Pagination: common.Pagination{
			PageNumber: 0,
			PageSize:   10,
		},
	})

	if err != nil {
		err = errors.New("Unable list routes:" + err.Error())
		return
	}

	for _, t := range tables {
		for _, r := range t.RouteEntrys.RouteEntry {
			if r.NextHopType != string(ecs.NextHopInstance) {
				continue
			}

			ip, ok := id2ip[r.InstanceId]
			if !ok {
				log.Warningf("Unable to get ip of instance: %+v", r)
				log.Warningf("%+v", id2ip)
				continue
			}

			routes = append(routes, &origcloudprovider.Route{
				TargetNode:      ip2NodeName(ip),
				DestinationCIDR: r.DestinationCidrBlock,
			})
		}
	}

	return
}

func (w *AliyunProvider) DeleteRoute(clusterName string, route *origcloudprovider.Route) (err error) {
	ip2id, err := w.getInstanceIP2ID()
	if err != nil {
		return
	}

	var (
		instanceId string
		ok         bool
	)

	ip := nodeName2IP(route.TargetNode)
	if instanceId, ok = ip2id[ip]; !ok {
		err = errors.New("Unable to get instance id of node:" + ip)
		return
	}

	args := &ecs.DeleteRouteEntryArgs{
		RouteTableId:         w.routeTable,
		DestinationCidrBlock: route.DestinationCIDR,
		NextHopId:            instanceId,
	}

	err = w.client.DeleteRouteEntry(args)
	if err != nil {
		log.Warningf("Unable to remove vpc route for %s (subnet: %s): %+v", instanceId, route.DestinationCIDR, err)
	}

	return
}

func (w *AliyunProvider) CreateRoute(clusterName string, nameHint string, route *origcloudprovider.Route) (err error) {
	ip2id, err := w.getInstanceIP2ID()
	if err != nil {
		return
	}

	var (
		instanceId string
		ok         bool
	)

	ip := nodeName2IP(route.TargetNode)
	if instanceId, ok = ip2id[ip]; !ok {
		err = errors.New("Unable to get instance id of node:" + ip)
		return
	}

	args := &ecs.CreateRouteEntryArgs{
		RouteTableId:         w.routeTable,
		DestinationCidrBlock: route.DestinationCIDR,
		NextHopId:            instanceId,
	}

	err = w.client.CreateRouteEntry(args)
	if err != nil {
		log.Warningf("Unable to add vpc route for %s (subnet: %s): %+v", instanceId, route.DestinationCIDR, err)
	}
	return
}
