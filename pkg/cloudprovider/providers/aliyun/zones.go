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
