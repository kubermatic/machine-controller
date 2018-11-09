package cache

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	gocache "github.com/patrickmn/go-cache"

	clusterv1alpha1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
)

type CloudproviderCache struct {
	cache *gocache.Cache
}

// New returns a new cloudproviderCache
func New() *CloudproviderCache {
	return &CloudproviderCache{cache: gocache.New(5*time.Minute, 5*time.Minute)}
}

// Get returns an error indicating the result of the validation and a boolean indicating if
// it got a cache hit or miss
func (c *CloudproviderCache) Get(machineSpec clusterv1alpha1.MachineSpec) (error, bool, error) {
	id, err := getID(machineSpec)
	if err != nil {
		return nil, false, err
	}

	val, found := c.cache.Get(id)
	if !found {
		return nil, false, nil
	}

	if val == nil {
		return nil, true, nil
	}

	errVal, castable := val.(error)
	if !castable {
		return nil, false, fmt.Errorf("failed to cast val to err: %v", err)
	}
	return errVal, true, nil
}

// Set sets the passed value for the given machineSpec
func (c *CloudproviderCache) Set(machineSpec clusterv1alpha1.MachineSpec, val error) error {
	id, err := getID(machineSpec)
	if err != nil {
		return err
	}

	c.cache.Set(id, val, gocache.DefaultExpiration)
	return nil
}

func getID(machineSpec clusterv1alpha1.MachineSpec) (string, error) {
	b, err := json.Marshal(machineSpec.ProviderConfig)
	if err != nil {
		return "", fmt.Errorf("failed to marshal MachineSpec: %v", err)
	}

	sum := sha256.Sum256(b)
	var sumSlice []byte
	for _, b := range sum {
		sumSlice = append(sumSlice, b)
	}
	return string(sumSlice), nil
}
