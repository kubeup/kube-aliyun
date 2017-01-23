package cloudprovider

import (
	"errors"
	"k8s.io/kubernetes/pkg/cloudprovider"
)

// Cloud provider interface
type Provider interface {
	cloudprovider.Interface
	Volume() (Volume, bool)
}

type ProviderFactory func() Provider

var (
	providers = make(map[string]ProviderFactory)
)

func RegisterProvider(name string, p ProviderFactory) {
	providers[name] = p
}

func GetProvider(name string) (Provider, error) {
	if p, ok := providers[name]; ok {
		return p(), nil
	}

	return nil, errors.New("No such provider: " + name)

}
