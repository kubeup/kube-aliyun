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

func (w *AliyunProvider) ListRoutes(clusterName string) (routes []*origcloudprovider.Route, err error) {
	instances, err := w.getInstances()
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
			if r.NextHopType != string(ecs.NextHopIntance) {
				continue
			}

			f := filter(instances, byInstanceId(r.InstanceId))

			if len(f) == 0 {
				log.Warningf("Unable to get ip of instance: %+v", f)
				continue
			}

			if len(f) > 1 {
				log.Warningf("Multiple instance with the same id: %+v", f)
				continue
			}

			routes = append(routes, &origcloudprovider.Route{
				TargetNode:      types.NodeName(w.mapInstanceToNodeName(f[0])),
				DestinationCIDR: r.DestinationCidrBlock,
			})
		}
	}

	return
}

func (w *AliyunProvider) DeleteRoute(clusterName string, route *origcloudprovider.Route) (err error) {
	i, err := w.getInstanceByNodeName(string(route.TargetNode))
	if err != nil {
		return
	}

	args := &ecs.DeleteRouteEntryArgs{
		RouteTableId:         w.routeTable,
		DestinationCidrBlock: route.DestinationCIDR,
		NextHopId:            i.InstanceId,
	}

	err = w.client.DeleteRouteEntry(args)
	if err != nil {
		log.Warningf("Unable to remove vpc route for %s (subnet: %s): %+v", i.InstanceId, route.DestinationCIDR, err)
	}

	return
}

func (w *AliyunProvider) CreateRoute(clusterName string, nameHint string, route *origcloudprovider.Route) (err error) {
	i, err := w.getInstanceByNodeName(string(route.TargetNode))
	if err != nil {
		return
	}

	args := &ecs.CreateRouteEntryArgs{
		RouteTableId:         w.routeTable,
		DestinationCidrBlock: route.DestinationCIDR,
		NextHopId:            i.InstanceId,
	}

	err = w.client.CreateRouteEntry(args)
	if err != nil {
		log.Warningf("Unable to add vpc route for %s (subnet: %s): %+v", i.InstanceId, route.DestinationCIDR, err)
	}
	return
}
