package util

import (
	"k8s.io/kubernetes/pkg/api/v1"
)

func GetNodePrivateIPs(n *v1.Node) (ips []string) {
	if n == nil {
		return
	}

	for _, addr := range n.Status.Addresses {
		if addr.Type == v1.NodeInternalIP {
			ips = append(ips, addr.Address)
		}
	}

	return
}
