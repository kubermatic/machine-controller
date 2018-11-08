package cache

import (
	"errors"
	"testing"

	corev1 "k8s.io/api/core/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"

	clusterv1alpha1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
)

func TestCloudproviderCache(t *testing.T) {
	cache := New()

	m1 := clusterv1alpha1.MachineSpec{}
	m1.ProviderConfig.Value = &runtime.RawExtension{Raw: []byte(`{"key":"m1"}`)}
	m1.Name = "hans"

	if err := cache.Set(m1, nil); err != nil {
		t.Fatalf("Error setting cache value for m1: %v", err)
	}
	val, exists, err := cache.Get(m1)
	if err != nil {
		t.Fatalf("Error when getting m1 from cache: %v", err)
	}
	if !exists {
		t.Error("Expected val to exist when getting m1 from cache")
	}
	if val != nil {
		t.Errorf("Expected m1 val to be nil but was %v", val)
	}

	m1.Name = "wurst"
	val, exists, err = cache.Get(m1)
	if err != nil {
		t.Fatalf("Error getting m1 from cache after changing name: %v", err)
	}
	if !exists {
		t.Error("Expected val to exist when getting m1 from cache after chaning name")
	}
	if val != nil {
		t.Errorf("Expected m1 val to be nil after changing name but was %v", val)
	}

	m1.Taints = []corev1.Taint{{Key: "hello", Value: "world"}}
	val, exists, err = cache.Get(m1)
	if err != nil {
		t.Fatalf("Error getting m1 from cache after adding taint: %v", err)
	}
	if !exists {
		t.Error("Expected val to exist when getting m1 from cache after adding taints")
	}
	if val != nil {
		t.Errorf("Expected m1 val to be nil after adding taint but was %s", val)
	}

	m2 := clusterv1alpha1.MachineSpec{}
	m2.ProviderConfig.Value = &runtime.RawExtension{Raw: []byte(`{"key":"m2"}`)}
	val, exists, err = cache.Get(m2)
	if err != nil {
		t.Fatalf("Error getting m2 from cache: %v", err)
	}
	if exists {
		t.Error("Expected val to not exist when getting m2 from cache")
	}
	if val != nil {
		t.Errorf("Expected val for m2 to be nil but was %v", val)
	}

	m3 := clusterv1alpha1.MachineSpec{}
	m3.ProviderConfig.Value = &runtime.RawExtension{Raw: []byte(`{"key":"m3"}`)}
	errMsg := "Thou shall not pass"
	if err := cache.Set(m3, errors.New(errMsg)); err != nil {
		t.Fatalf("Error setting cache value for m3: %v", err)
	}
	val, exists, err = cache.Get(m3)
	if err != nil {
		t.Fatalf("Error getting m3 from cache: %v", err)
	}
	if !exists {
		t.Error("Expected val to exist when getting m3 from cache")
	}
	if val.Error() != errMsg {
		t.Errorf("Expected val for m3 to be %s but was %v", errMsg, val)
	}
}
