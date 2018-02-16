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
	err := json.Unmarshal(jsonString, &jsonStringTarget)
	if err != nil || !reflect.DeepEqual(jsonStringTarget, expectedResult) {
		t.Fatalf("Decoding raw string into configVarString failed! Error: '%v'", err)
	}

	var jsonMapTarget ConfigVarString
	err = json.Unmarshal(jsonMap, &jsonMapTarget)
	if err != nil || !reflect.DeepEqual(jsonMapTarget, expectedResult) {
		t.Fatalf("Decoding map into configVarString failed! Error: '%v'", err)
	}
}
