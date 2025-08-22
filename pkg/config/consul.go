package config

import (
	"fmt"
	"github.com/hashicorp/consul/api"
)

type Loader struct {
	Client *api.Client
	Prefix string
}

func NewLoader(addr, prefix string) (*Loader, error) {
	cfg := api.DefaultConfig()
	if addr != "" {
		cfg.Address = addr
	}
	cli, err := api.NewClient(cfg)
	if err != nil {
		return nil, err
	}
	return &Loader{Client: cli, Prefix: prefix}, nil
}

func (l *Loader) Get(key string) (string, error) {
	kv := l.Client.KV()
	p := fmt.Sprintf("%s/%s", l.Prefix, key)
	pair, _, err := kv.Get(p, nil)
	if err != nil {
		return "", err
	}
	if pair == nil {
		return "", nil
	}
	return string(pair.Value), nil
}

func (l *Loader) Put(key, val string) error {
	kv := l.Client.KV()
	p := fmt.Sprintf("%s/%s", l.Prefix, key)
	_, err := kv.Put(&api.KVPair{Key: p, Value: []byte(val)}, nil)
	return err
}
