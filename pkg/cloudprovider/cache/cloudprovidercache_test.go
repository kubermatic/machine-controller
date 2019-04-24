/*
Copyright 2019 The Machine Controller Authors.

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
	m1.ProviderSpec.Value = &runtime.RawExtension{Raw: []byte(`{"key":"m1"}`)}
	m1.Name = "hans"

	// Test SET and GET
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

	// Test metadata gets ignored by cache
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

	// Test taints get ignored by cache
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

	// Test versions field gets ignored by cache
	m1.Versions.Kubelet = "1.13.0"
	val, exists, err = cache.Get(m1)
	if err != nil {
		t.Fatalf("Error getting m1 from cache after adding kubelet version: %v", err)
	}
	if !exists {
		t.Error("Expected val to exist when getting m1 from cache after adding kubelet version")
	}
	if val != nil {
		t.Errorf("Expected m1 val to be nil after adding kubelet version but was %s", val)
	}

	// Test ProviderSpec does not get ignored by cache
	m2 := clusterv1alpha1.MachineSpec{}
	m2.ProviderSpec.Value = &runtime.RawExtension{Raw: []byte(`{"key":"m2"}`)}
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

	// Test error gets properly cached
	m3 := clusterv1alpha1.MachineSpec{}
	m3.ProviderSpec.Value = &runtime.RawExtension{Raw: []byte(`{"key":"m3"}`)}
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
