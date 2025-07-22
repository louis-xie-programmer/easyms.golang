package config

import (
	"fmt"
	"github.com/hashicorp/consul/api"
)

// CreateConsulClient creates a new Consul client
func CreateConsulClient() (*api.Client, error) {
	appStore := GetAppConfigStore()
	config := api.DefaultConfig()
	config.Address = appStore.Consul.Host

	client, err := api.NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create consul client: %w", err)
	}

	return client, nil
}
