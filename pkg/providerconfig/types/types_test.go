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

package types

import (
	"encoding/json"
	"reflect"
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/utils/pointer"
)

func TestConfigVarStringUnmarshalling(t *testing.T) {
	jsonString := []byte(`"value"`)
	jsonMap := []byte(`{"value":"value"}`)

	expectedResult := ConfigVarString{Value: "value"}

	var jsonStringTarget ConfigVarString
	var jsonMapTarget ConfigVarString

	err := json.Unmarshal(jsonString, &jsonStringTarget)
	if err != nil || !reflect.DeepEqual(expectedResult, jsonStringTarget) {
		t.Fatalf("Decoding raw string into configVarString failed! Error: '%v'", err)
	}

	err = json.Unmarshal(jsonMap, &jsonMapTarget)
	if err != nil || !reflect.DeepEqual(expectedResult, jsonMapTarget) {
		t.Fatalf("Decoding map into configVarString failed! Error: '%v'", err)
	}
}

func TestConfigVarBoolUnmarshalling(t *testing.T) {
	testCases := []struct {
		jsonString string
		expected   ConfigVarBool
	}{
		{
			jsonString: "true",
			expected:   ConfigVarBool{Value: pointer.Bool(true)},
		},
		{
			jsonString: `{"value":true}`,
			expected:   ConfigVarBool{Value: pointer.Bool(true)},
		},
		{
			jsonString: "null",
			expected:   ConfigVarBool{},
		},
		{
			jsonString: `{"value":null}`,
			expected:   ConfigVarBool{},
		},
		{
			jsonString: `{"secretKeyRef":{"namespace":"ns","name":"name","key":"key"}}`,
			expected:   ConfigVarBool{Value: nil, SecretKeyRef: GlobalSecretKeySelector{ObjectReference: v1.ObjectReference{Namespace: "ns", Name: "name"}, Key: "key"}},
		},
		{
			jsonString: `{"value": null, "secretKeyRef":{"namespace":"ns","name":"name","key":"key"}}`,
			expected:   ConfigVarBool{Value: nil, SecretKeyRef: GlobalSecretKeySelector{ObjectReference: v1.ObjectReference{Namespace: "ns", Name: "name"}, Key: "key"}},
		},
		{
			jsonString: `{"value":false, "secretKeyRef":{"namespace":"ns","name":"name","key":"key"}}`,
			expected:   ConfigVarBool{Value: pointer.Bool(false), SecretKeyRef: GlobalSecretKeySelector{ObjectReference: v1.ObjectReference{Namespace: "ns", Name: "name"}, Key: "key"}},
		},
		{
			jsonString: `{"value":true, "secretKeyRef":{"namespace":"ns","name":"name","key":"key"}}`,
			expected:   ConfigVarBool{Value: pointer.Bool(true), SecretKeyRef: GlobalSecretKeySelector{ObjectReference: v1.ObjectReference{Namespace: "ns", Name: "name"}, Key: "key"}},
		},
	}

	for _, testCase := range testCases {
		var cvb ConfigVarBool
		err := json.Unmarshal([]byte(testCase.jsonString), &cvb)
		if err != nil || !reflect.DeepEqual(testCase.expected, cvb) {
			t.Fatalf("Decoding '%s' into configVarBool failed! Error: '%v'", testCase.jsonString, err)
		}
	}
}

func TestConfigVarStringMarshalling(t *testing.T) {
	testCases := []struct {
		cvs      ConfigVarString
		expected string
	}{
		{
			cvs:      ConfigVarString{Value: "val"},
			expected: `"val"`,
		},
		{
			cvs:      ConfigVarString{SecretKeyRef: GlobalSecretKeySelector{ObjectReference: v1.ObjectReference{Namespace: "ns", Name: "name"}, Key: "key"}},
			expected: `{"secretKeyRef":{"namespace":"ns","name":"name","key":"key"}}`,
		},
	}

	for _, testCase := range testCases {
		result, err := json.Marshal(testCase.cvs)
		if err != nil {
			t.Errorf("Failed to marshall config var string: %v", err)
		}
		if string(result) != testCase.expected {
			t.Errorf("Result '%s' of config var string marshalling does not match expected '%s'", string(result), testCase.expected)
		}
	}
}

func TestConfigVarBoolMarshalling(t *testing.T) {
	testCases := []struct {
		cvb      ConfigVarBool
		expected string
	}{
		{
			cvb:      ConfigVarBool{},
			expected: `null`,
		},
		{
			cvb:      ConfigVarBool{Value: pointer.Bool(true)},
			expected: `true`,
		},
		{
			cvb:      ConfigVarBool{SecretKeyRef: GlobalSecretKeySelector{ObjectReference: v1.ObjectReference{Namespace: "ns", Name: "name"}, Key: "key"}},
			expected: `{"secretKeyRef":{"namespace":"ns","name":"name","key":"key"}}`,
		},
		{
			cvb:      ConfigVarBool{SecretKeyRef: GlobalSecretKeySelector{ObjectReference: v1.ObjectReference{Namespace: "ns", Name: "name"}, Key: "key"}, Value: pointer.Bool(true)},
			expected: `{"secretKeyRef":{"namespace":"ns","name":"name","key":"key"},"value":true}`,
		},
		{
			cvb:      ConfigVarBool{SecretKeyRef: GlobalSecretKeySelector{ObjectReference: v1.ObjectReference{Namespace: "ns", Name: "name"}, Key: "key"}, Value: pointer.Bool(false)},
			expected: `{"secretKeyRef":{"namespace":"ns","name":"name","key":"key"},"value":false}`,
		},
	}

	for _, testCase := range testCases {
		result, err := json.Marshal(testCase.cvb)
		if err != nil {
			t.Errorf("Failed to marshall config var bool: %v", err)
		}
		if string(result) != testCase.expected {
			t.Errorf("Result '%s' of config var bool marshalling does not match expected '%s'", string(result), testCase.expected)
		}
	}
}

func TestConfigVarStringMarshallingAndUnmarshalling(t *testing.T) {
	testCases := []ConfigVarString{
		{Value: "val"},
		{Value: "spe<ialv&lue"},
		{SecretKeyRef: GlobalSecretKeySelector{ObjectReference: v1.ObjectReference{Namespace: "ns", Name: "name"}, Key: "key"}},
		{Value: "val", SecretKeyRef: GlobalSecretKeySelector{ObjectReference: v1.ObjectReference{Namespace: "ns", Name: "name"}, Key: "key"}},
		{ConfigMapKeyRef: GlobalConfigMapKeySelector{ObjectReference: v1.ObjectReference{Namespace: "ns", Name: "name"}, Key: "key"}},
		{
			ConfigMapKeyRef: GlobalConfigMapKeySelector{ObjectReference: v1.ObjectReference{Namespace: "ns", Name: "name"}, Key: "key"},
			SecretKeyRef:    GlobalSecretKeySelector{ObjectReference: v1.ObjectReference{Namespace: "ns", Name: "name"}, Key: "key"},
		},
		{
			Value:           "val",
			ConfigMapKeyRef: GlobalConfigMapKeySelector{ObjectReference: v1.ObjectReference{Namespace: "ns", Name: "name"}, Key: "key"},
			SecretKeyRef:    GlobalSecretKeySelector{ObjectReference: v1.ObjectReference{Namespace: "ns", Name: "name"}, Key: "key"},
		},
		{
			Value:           "spe<ialv&lue",
			ConfigMapKeyRef: GlobalConfigMapKeySelector{ObjectReference: v1.ObjectReference{Namespace: "ns", Name: "name"}, Key: "key"},
			SecretKeyRef:    GlobalSecretKeySelector{ObjectReference: v1.ObjectReference{Namespace: "ns", Name: "name"}, Key: "key"},
		},
	}

	for _, cvs := range testCases {
		marshalled, err := json.Marshal(cvs)
		if err != nil {
			t.Errorf("Failed to marshall config var string: %v", err)
		}

		var unmarshalled ConfigVarString
		err = json.Unmarshal(marshalled, &unmarshalled)
		if err != nil {
			t.Errorf("Failed to unmarshall config var string after marshalling: %v", err)
		}
		if !reflect.DeepEqual(cvs, unmarshalled) {
			t.Errorf("ConfigVarString object is not equal after marshalling and unmarshalling, old: '%+v', new: '%+v'", cvs, unmarshalled)
		}
	}
}

func TestConfigVarBoolMarshallingAndUnmarshalling(t *testing.T) {
	testCases := []ConfigVarBool{
		{},
		{Value: pointer.Bool(false)},
		{Value: pointer.Bool(true)},
		{SecretKeyRef: GlobalSecretKeySelector{ObjectReference: v1.ObjectReference{Namespace: "ns", Name: "name"}, Key: "key"}},
		{Value: pointer.Bool(true), SecretKeyRef: GlobalSecretKeySelector{ObjectReference: v1.ObjectReference{Namespace: "ns", Name: "name"}, Key: "key"}},
		{ConfigMapKeyRef: GlobalConfigMapKeySelector{ObjectReference: v1.ObjectReference{Namespace: "ns", Name: "name"}, Key: "key"}},
		{Value: pointer.Bool(true), ConfigMapKeyRef: GlobalConfigMapKeySelector{ObjectReference: v1.ObjectReference{Namespace: "ns", Name: "name"}, Key: "key"}},
		{
			ConfigMapKeyRef: GlobalConfigMapKeySelector{ObjectReference: v1.ObjectReference{Namespace: "ns", Name: "name"}, Key: "key"},
			SecretKeyRef:    GlobalSecretKeySelector{ObjectReference: v1.ObjectReference{Namespace: "ns", Name: "name"}, Key: "key"},
		},
		{
			Value:           pointer.Bool(true),
			ConfigMapKeyRef: GlobalConfigMapKeySelector{ObjectReference: v1.ObjectReference{Namespace: "ns", Name: "name"}, Key: "key"},
			SecretKeyRef:    GlobalSecretKeySelector{ObjectReference: v1.ObjectReference{Namespace: "ns", Name: "name"}, Key: "key"},
		},
	}

	for _, cvs := range testCases {
		marshalled, err := json.Marshal(cvs)
		if err != nil {
			t.Errorf("Failed to marshall config var string: %v", err)
		}

		var unmarshalled ConfigVarBool
		err = json.Unmarshal(marshalled, &unmarshalled)
		if err != nil {
			t.Errorf("Failed to unmarshall config var bool after marshalling: %v, '%v'", err, string(marshalled))
		}
		if !reflect.DeepEqual(cvs, unmarshalled) {
			t.Errorf("ConfigVarBool object is not equal after marshalling and unmarshalling, old: '%+v', new: '%+v'", cvs, unmarshalled)
		}
	}
}
