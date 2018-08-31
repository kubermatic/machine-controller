package providerconfig

import (
	"encoding/json"
	"reflect"
	"testing"

	"k8s.io/api/core/v1"
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
	jsonBool := []byte("true")
	jsonMapBool := []byte(`{"value":true}`)

	expectedResult := ConfigVarBool{Value: true}

	var jsonBoolTarget ConfigVarBool
	var jsonMapBoolTarget ConfigVarBool

	err := json.Unmarshal(jsonBool, &jsonBoolTarget)
	if err != nil || !reflect.DeepEqual(expectedResult, jsonBoolTarget) {
		t.Fatalf("Decoding raw bool into configVarBool failed! Error: '%v'", err)
	}
	err = json.Unmarshal(jsonMapBool, &jsonMapBoolTarget)
	if err != nil || !reflect.DeepEqual(expectedResult, jsonMapBoolTarget) {
		t.Fatalf("Decoding map bool into configVarBool failed! Error: '%v'", err)
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
			cvb:      ConfigVarBool{Value: true},
			expected: `true`,
		},
		{
			cvb:      ConfigVarBool{SecretKeyRef: GlobalSecretKeySelector{ObjectReference: v1.ObjectReference{Namespace: "ns", Name: "name"}, Key: "key"}},
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
		ConfigVarString{Value: "val"},
		ConfigVarString{SecretKeyRef: GlobalSecretKeySelector{ObjectReference: v1.ObjectReference{Namespace: "ns", Name: "name"}, Key: "key"}},
		ConfigVarString{Value: "val", SecretKeyRef: GlobalSecretKeySelector{ObjectReference: v1.ObjectReference{Namespace: "ns", Name: "name"}, Key: "key"}},
		ConfigVarString{ConfigMapKeyRef: GlobalConfigMapKeySelector{ObjectReference: v1.ObjectReference{Namespace: "ns", Name: "name"}, Key: "key"}},
		ConfigVarString{
			ConfigMapKeyRef: GlobalConfigMapKeySelector{ObjectReference: v1.ObjectReference{Namespace: "ns", Name: "name"}, Key: "key"},
			SecretKeyRef:    GlobalSecretKeySelector{ObjectReference: v1.ObjectReference{Namespace: "ns", Name: "name"}, Key: "key"},
		},
		ConfigVarString{
			Value:           "val",
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
		ConfigVarBool{Value: true},
		ConfigVarBool{SecretKeyRef: GlobalSecretKeySelector{ObjectReference: v1.ObjectReference{Namespace: "ns", Name: "name"}, Key: "key"}},
		ConfigVarBool{Value: true, SecretKeyRef: GlobalSecretKeySelector{ObjectReference: v1.ObjectReference{Namespace: "ns", Name: "name"}, Key: "key"}},
		ConfigVarBool{ConfigMapKeyRef: GlobalConfigMapKeySelector{ObjectReference: v1.ObjectReference{Namespace: "ns", Name: "name"}, Key: "key"}},
		ConfigVarBool{Value: true, ConfigMapKeyRef: GlobalConfigMapKeySelector{ObjectReference: v1.ObjectReference{Namespace: "ns", Name: "name"}, Key: "key"}},
		ConfigVarBool{
			ConfigMapKeyRef: GlobalConfigMapKeySelector{ObjectReference: v1.ObjectReference{Namespace: "ns", Name: "name"}, Key: "key"},
			SecretKeyRef:    GlobalSecretKeySelector{ObjectReference: v1.ObjectReference{Namespace: "ns", Name: "name"}, Key: "key"},
		},
		ConfigVarBool{
			Value:           true,
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
