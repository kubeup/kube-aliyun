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
	"fmt"

	"github.com/denverdino/aliyungo/common"
	"github.com/denverdino/aliyungo/ecs"
	"k8s.io/kubernetes/pkg/cloudprovider"
)

func (p *AliyunProvider) GetZone() (zone cloudprovider.Zone, err error) {
	vswitches, _, err := p.client.DescribeVSwitches(&ecs.DescribeVSwitchesArgs{
		VpcId:     p.vpcID,
		VSwitchId: p.vswitch,
		Pagination: common.Pagination{
			PageNumber: 0,
			PageSize:   10,
		},
	})

	if err != nil || len(vswitches) == 0 {
		err = fmt.Errorf("Unable to get zone: %v", err)
		return
	}

	vs := vswitches[0]
	zone = cloudprovider.Zone{
		FailureDomain: vs.ZoneId,
		Region:        p.region,
	}
	return
}
