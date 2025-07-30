package crdt

import (
	"testing"

	"github.com/eislab-cps/synctree/pkg/core"
	"github.com/eislab-cps/synctree/pkg/random"
	"github.com/eislab-cps/synctree/pkg/utils"
	"github.com/stretchr/testify/assert"
)

func TestTreeCRDTImport(t *testing.T) {
	clientID := core.ClientID(random.GenerateRandomID())

	originalJSON := []byte(`{
		"uid": "user_1",
		"name": "Alice",
		"friends": [
			{
				"uid": "user_2",
				"name": "Bob"
			},
			{
				"uid": "user_3",
				"name": "Charlie",
				"friends": [
					{
						"uid": "user_4",
						"name": "Dana"
					}
				]
			}
		]
	}`)

	c := NewTreeCRDT()
	_, err := c.ImportJSON(originalJSON, clientID)
	if err != nil {
		t.Fatalf("Failed to add node recursively: %v", err)
	}

	exportedJSON, err := c.ExportJSON()
	assert.Nil(t, err, "ExportToJSON should not return an error")
	utils.CompareJSON(t, originalJSON, exportedJSON)
}

func TestTreeCRDTImportToArray(t *testing.T) {
	clientID := core.ClientID(random.GenerateRandomID())

	originalJSON := []byte(`{
		"uid": "user_1",
		"name": "Alice",
		"friends": [
			{
				"uid": "user_2",
				"name": "Bob"
			},
			{
				"uid": "user_3",
				"name": "Charlie",
				"friends": [
					{
						"uid": "user_4",
						"name": "Dana"
					}
				]
			}
		]
	}`)

	childJSON := []byte(`{
		"uid": "user_4",
		"name": "Bob2"
	}`)

	c := NewTreeCRDT()
	_, err := c.ImportJSON(originalJSON, clientID)
	if err != nil {
		t.Fatalf("Failed to add node recursively: %v", err)
	}

	friendsNode, err := c.GetNodeByPath("/friends")
	assert.Nil(t, err, "GetNodeByPath should not return an error")

	_, err = c.ImportJSONToArray(childJSON, friendsNode.ID, clientID)
	assert.Nil(t, err, "ImportJSON should not return an error")

	str, err := c.GetStringValueByPath("/friends/0/uid")
	assert.Nil(t, err, "GetNodeByPath should not return an error")
	assert.Equal(t, "user_4", str, "Expected uid to be 'user_4'")
}

func TestTreeCRDTImportToMap(t *testing.T) {
	clientID := core.ClientID(random.GenerateRandomID())

	originalJSON := []byte(`{
		"uid": "user_1",
		"name": "Alice",
		"friends": [
			{
				"uid": "user_2",
				"name": "Bob"
			},
			{
				"uid": "user_3",
				"name": "Charlie",
				"friends": [
					{
						"uid": "user_4",
						"name": "Dana"
					}
				]
			}
		]
	}`)

	childJSON := []byte(`{
		"uid": "user_4",
		"name": "Bob"
	}`)

	c := NewTreeCRDT()
	_, err := c.ImportJSON(originalJSON, clientID)
	if err != nil {
		t.Fatalf("Failed to add node recursively: %v", err)
	}

	mapNode, err := c.GetNodeByPath("/friends/0")
	assert.Nil(t, err, "GetNodeByPath should not return an error")

	_, err = c.ImportJSONToMap(childJSON, mapNode.ID, "testkey", clientID)
	assert.Nil(t, err, "ImportJSON should not return an error")

	str, err := c.GetStringValueByPath("/friends/0/testkey/name")
	assert.Nil(t, err, "GetNodeByPath should not return an error")
	assert.Equal(t, "Bob", str, "Expected name to be 'Bob'")
}

func TestTreeCRDTSetKeyValueAfterImport(t *testing.T) {
	clientID := core.ClientID(random.GenerateRandomID())

	initialJSON := []byte(`{
        "A": "1",
        "B": "2",
        "D": "2"
    }`)

	c := NewTreeCRDT()

	_, err := c.ImportJSON(initialJSON, core.ClientID(clientID))
	assert.Nil(t, err, "AddNodeRecursively should not return an error")

	// Find the root child
	firstObjID := c.Root.Edges[0].To
	childNode, ok := c.GetNode(firstObjID)
	if !ok {
		t.Fatalf("Child node not found")
	}
	assert.NotNil(t, childNode, "Child node should not be nil")
	childNode.SetKeyValue("C", "3", core.ClientID(clientID))

	exportedJSON, err := c.ExportJSON()
	assert.Nil(t, err, "ExportToJSON should not return an error")

	expectedJSON := []byte(`{
        "A": "1",
        "B": "2",
        "C": "3",
        "D": "2"
    }`)

	utils.CompareJSON(t, expectedJSON, exportedJSON)
}

func TestTreeCRDTAddToArrayAfterImport(t *testing.T) {
	clientID := core.ClientID(random.GenerateRandomID())

	initialJSON := []byte(`[
		{"id": "A", "value": "1"},
		{"id": "B", "value": "2"},
		{"id": "D", "value": "2"}
	]`)

	c := NewTreeCRDT()
	_, err := c.ImportJSON(initialJSON, core.ClientID(clientID))
	if err != nil {
		t.Fatalf("Failed to add node recursively: %v", err)
	}

	nodeC := c.CreateNode("C", Map, clientID)
	//nodeC.IsMap = true
	nodeC.SetKeyValue("id", "C", core.ClientID(clientID))
	nodeC.SetKeyValue("value", "3", core.ClientID(clientID))

	arrayNodeID := c.Root.Edges[0].To
	leftTo, err := c.GetSibling(arrayNodeID, 1)
	assert.Nil(t, err, "GetSibling should not return an error")
	c.InsertEdgeRight(arrayNodeID, nodeC.ID, "", leftTo.ID, clientID) // Insert C

	exportedJSON, err := c.ExportJSON()
	assert.Nil(t, err, "ExportToJSON should not return an error")

	expectedJSON := []byte(`[
		{"id": "A", "value": "1"},
		{"id": "B", "value": "2"},
		{"id": "C", "value": "3"},
		{"id": "D", "value": "2"}
	]`)

	utils.CompareJSON(t, expectedJSON, exportedJSON)
}

func TestTreeCRDTInsertStringAfterImport(t *testing.T) {
	clientID := core.ClientID(random.GenerateRandomID())

	initialJSON := []byte(`["A", "B", "D"]`)

	c := NewTreeCRDT()
	_, err := c.ImportJSON(initialJSON, core.ClientID(clientID))
	assert.Nil(t, err, "AddNodeRecursively should not return an error")

	nodeIDC := generateRandomNodeID(string("C"))
	nodeC := c.getOrCreateNode(nodeIDC, Literal, clientID, 1)
	//nodeC.IsLiteral = true
	nodeC.LiteralValue = "C"

	arrayNodeID := c.Root.Edges[0].To
	sibling, err := c.GetSibling(arrayNodeID, 2) // Index 1 is D
	err = c.InsertEdgeLeft(arrayNodeID, nodeIDC, "", sibling.ID, clientID)
	assert.Nil(t, err, "InsertEdge should not return an error")

	exportedJSON, err := c.ExportJSON()
	assert.Nil(t, err, "ExportToJSON should not return an error")

	expectedJSON := []byte(`[
		"A",
		"B",
		"C",
		"D"
	]`)

	utils.CompareJSON(t, expectedJSON, exportedJSON)
}

func TestTreeCRDTInsertIntAfterImport(t *testing.T) {
	clientID := core.ClientID(random.GenerateRandomID())

	initialJSON := []byte(`[1, 2, 4]`)

	c := NewTreeCRDT()
	_, err := c.ImportJSON(initialJSON, core.ClientID(clientID))
	assert.Nil(t, err, "AddNodeRecursively should not return an error")

	nodeIDC := generateRandomNodeID(string("C"))
	nodeC := c.getOrCreateNode(nodeIDC, Literal, clientID, 1)
	//nodeC.IsLiteral = true
	nodeC.LiteralValue = 3

	sibling, err := c.GetSibling(c.Root.ID, 20)
	assert.NotNil(t, err, "Invalid index should not return an error")

	sibling, err = c.GetSibling(c.Root.ID, -1)
	assert.NotNil(t, err, "Invalid index should not return an error")

	arrayNodeID := c.Root.Edges[0].To

	sibling, err = c.GetSibling(arrayNodeID, 1) // Index 1 is B

	err = c.InsertEdgeRight(arrayNodeID, nodeIDC, "", sibling.ID, clientID)
	assert.Nil(t, err, "InsertEdge should not return an error")

	exportedJSON, err := c.ExportJSON()
	assert.Nil(t, err, "ExportToJSON should not return an error")

	expectedJSON := []byte(`[
		1,
		2,
		3,
		4
	]`)

	utils.CompareJSON(t, expectedJSON, exportedJSON)
}

func TestTreeCRDTSaveAndLoad(t *testing.T) {
	clientID := core.ClientID(random.GenerateRandomID())

	initialJSON := []byte(`[
		{"id": "A", "value": "1"},
		{"id": "B", "value": "2"},
		{"id": "C", "value": "3"},
		{"id": "D", "value": "2"}
	]`)

	c := NewTreeCRDT()
	_, err := c.ImportJSON(initialJSON, core.ClientID(clientID))
	assert.Nil(t, err, "AddNodeRecursively should not return an error")

	rawJSON, err := c.Save()
	assert.Nil(t, err, "ExportToRaw should not return an error")

	c2 := NewTreeCRDT()
	err = c2.Load(rawJSON)
	assert.Nil(t, err, "ImportRawJSON should not return an error")
	assert.True(t, c.Equal(c2), "Trees should be equal after import/export")
}

// The prupose of this test is to check that the graph ID is correctly set
func TestTreeCRDTNodeIDAfterImport(t *testing.T) {
	clientID := core.ClientID(random.GenerateRandomID())

	initialJSON := []byte(`["A", "B", "B"]`)

	// We will create graph that looks like this:
	// Root
	// ├── A
	// ├── B
	// └── B <- Duplicate

	c := NewTreeCRDT()
	_, err := c.ImportJSON(initialJSON, core.ClientID(clientID))
	assert.Nil(t, err, "AddNodeRecursively should not return an error")

	exportedJSON, err := c.ExportJSON()
	assert.Nil(t, err, "ExportToJSON should not return an error")

	// Correct expected JSON
	expectedJSON := []byte(`[
		"A",
		"B",
		"B"
	]`)

	utils.CompareJSON(t, expectedJSON, exportedJSON)
}
