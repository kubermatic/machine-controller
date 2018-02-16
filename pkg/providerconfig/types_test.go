package providerconfig

import (
	"encoding/json"
	"reflect"
	"testing"
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
