package config

import (
	"github.com/hashicorp/consul/api"
	"log"
	"sync"
	"time"
)

var (
	consulConfigProviders []*ConsulConfigProvider
	configLock            sync.RWMutex
)

type ConsulConfigProvider struct {
	consulClient    *api.Client
	currentIndex    uint64
	key             string
	Data            []byte
	reloadOnChanges bool
}

func InitConsulConfigProviders(client *api.Client, keys []string) {
	for _, key := range keys {
		consulConfigProvider := &ConsulConfigProvider{
			consulClient:    client,
			key:             key,
			reloadOnChanges: true,
		}

		consulConfigProvider.LoadData()
		consulConfigProviders = append(consulConfigProviders, consulConfigProvider)
		watchConfig()
	}
}

func GetConsulConfigProvider() []*ConsulConfigProvider {
	return consulConfigProviders
}

func (cp *ConsulConfigProvider) LoadData() {
	db, index, err := cp.getData()
	if err != nil {
		log.Fatalf("[ERROR] Failed to get data from Consul: %v", err)
	}
	cp.UpdateConsulConfigProvider(index, db)
}

func (cp *ConsulConfigProvider) UpdateConsulConfigProvider(index uint64, db []byte) {
	configLock.Lock()
	cp.currentIndex = index
	if db != nil {
		cp.Data = db
	}
	configLock.Unlock()
}

func watchConfig() {
	for _, cp := range consulConfigProviders {
		if !cp.reloadOnChanges {
			return
		}

		go func() {
			for {
				db, index, err := cp.getData()
				if err != nil {
					log.Printf("[ERROR] Failed to get data from Consul: %v", err)
				}
				if index != cp.currentIndex {
					cp.UpdateConsulConfigProvider(index, db)
					ReInitAppConfig()
				}

				time.Sleep(10 * time.Second)
			}
		}()
	}
}

func (cp *ConsulConfigProvider) getData() ([]byte, uint64, error) {
	pair, q, err := cp.consulClient.KV().Get(cp.key, nil)
	if err != nil {
		return nil, 0, err
	}
	return pair.Value, q.LastIndex, nil
}
