package utils

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

func CompareJSON(t *testing.T, expectedJSON, exportedJSON []byte) {
	var expected, actual interface{}
	err := json.Unmarshal(expectedJSON, &expected)
	assert.Nil(t, err, "Failed to unmarshal expected JSON: %v", err)
	err = json.Unmarshal(exportedJSON, &actual)
	assert.Nil(t, err, "Failed to unmarshal exported JSON: %v", err)
	assert.True(t, reflect.DeepEqual(expected, actual), "Exported JSON does not match expected.\nExpected:\n%v\n\nGot:\n%v\n", expected, actual)
}

func IsJSONEqual(t *testing.T, expectedJSON, exportedJSON []byte) bool {
	var expected, actual interface{}
	err := json.Unmarshal(expectedJSON, &expected)
	assert.Nil(t, err, "Failed to unmarshal expected JSON: %v", err)
	err = json.Unmarshal(exportedJSON, &actual)
	assert.Nil(t, err, "Failed to unmarshal exported JSON: %v", err)
	return reflect.DeepEqual(expected, actual)
}
