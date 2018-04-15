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
	jsonString := []byte(`"true"`)
	jsonMapBool := []byte(`{"value":true}`)
	jsonMapString := []byte(`{"value":"true"}`)

	expectedResult := ConfigVarBool{Value: true}

	var jsonBoolTarget ConfigVarBool
	var jsonStringTarget ConfigVarBool
	var jsonMapBoolTarget ConfigVarBool
	var jsonMapStringTarget ConfigVarBool

	err := json.Unmarshal(jsonBool, &jsonBoolTarget)
	if err != nil || !reflect.DeepEqual(expectedResult, jsonBoolTarget) {
		t.Fatalf("Decoding raw bool into configVarBool failed! Error: '%v'", err)
	}
	err = json.Unmarshal(jsonString, &jsonStringTarget)
	if err != nil || !reflect.DeepEqual(expectedResult, jsonStringTarget) {
		t.Fatalf("Decoding raw string bool into configVarBool failed! Error: '%v'", err)
	}
	err = json.Unmarshal(jsonMapBool, &jsonMapBoolTarget)
	if err != nil || !reflect.DeepEqual(expectedResult, jsonMapBoolTarget) {
		t.Fatalf("Decoding map bool into configVarBool failed! Error: '%v'", err)
	}
	err = json.Unmarshal(jsonMapString, &jsonMapStringTarget)
	if err != nil || !reflect.DeepEqual(expectedResult, jsonMapStringTarget) {
		t.Fatalf("Decoding map string bool into configVarBool failed! Error: '%v'", err)
	}

}

func TestConfigVarStringMarshalling(t *testing.T) {
	t.Parallel()

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
	t.Parallel()

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
			expected: `{"secretKeyRef":{"namespace":"ns","name":"name","key":"key"}}`,
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
