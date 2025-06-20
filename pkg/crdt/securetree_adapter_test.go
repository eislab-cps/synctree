package crdt

import (
	"testing"

	"github.com/eislab-cps/synctree/internal/crypto"
	"github.com/stretchr/testify/assert"
)

func TestSecureTreeAdapterBasic(t *testing.T) {
	prvKey := "d6eb959e9aec2e6fdc44b5862b269e987b8a4d6f2baca542d8acaa97ee5e74f6"
	prvKeyInvalid := "ed26531bac1838e519c2c6562ac717b22aac041730f0d753d3ad35b76b5f4924"
	initialJSON := []byte(`["A", "B", "B"]`)

	c, err := NewSecureTree(prvKey)
	assert.Nil(t, err, "NewSecureTree should not return an error")

	_, err = c.ImportJSON(initialJSON, prvKey)
	assert.Nil(t, err, "AddNodeRecursively should not return an error")

	aNode, err := c.GetNodeByPath("/0")
	assert.Nil(t, err, "GetNodeByPath should not return an error")
	err = aNode.SetLiteral("AA", prvKeyInvalid)
	assert.NotNil(t, err, "SetLiteral should return an error for invalid private key")
	err = aNode.SetLiteral("AA", prvKey)
	assert.Nil(t, err, "SetLiteral should not return an error")

	exportedJSON, err := c.ExportJSON()
	assert.Nil(t, err, "ExportToJSON should not return an error")

	// Correct expected JSON
	expectedJSON := []byte(`[
		"AA",
		"B",
		"B"
	]`)

	compareJSON(t, expectedJSON, exportedJSON)
}

func TestSecureTreeAdapterSetLiteral(t *testing.T) {
	prvKey := "d6eb959e9aec2e6fdc44b5862b269e987b8a4d6f2baca542d8acaa97ee5e74f6"
	prvKeyInvalid := "ed26531bac1838e519c2c6562ac717b22aac041730f0d753d3ad35b76b5f4924"
	initialJSON := []byte(`["A", "B", "B"]`)

	c, err := NewSecureTree(prvKey)
	assert.Nil(t, err)

	_, err = c.ImportJSON(initialJSON, prvKey)
	assert.Nil(t, err)

	t.Run("Reject SetLiteral with invalid key", func(t *testing.T) {
		aNode, err := c.GetNodeByPath("/0")
		assert.Nil(t, err)

		err = aNode.SetLiteral("AA", prvKeyInvalid)
		assert.NotNil(t, err)
		assert.Contains(t, err.Error(), "not allowed")
	})

	t.Run("Allow SetLiteral with valid key", func(t *testing.T) {
		aNode, err := c.GetNodeByPath("/0")
		assert.Nil(t, err)

		err = aNode.SetLiteral("AA", prvKey)
		assert.Nil(t, err)

		secureNode := aNode.(*AdapterSecureNodeCRDT)
		assert.NotEmpty(t, secureNode.nodeCrdt.Nounce)
		assert.NotEmpty(t, secureNode.nodeCrdt.Signature)
	})
}

func TestSecureTreeAdapterCreateMapNode(t *testing.T) {
	prvKey := "d6eb959e9aec2e6fdc44b5862b269e987b8a4d6f2baca542d8acaa97ee5e74f6"
	prvKeyInvalid := "ed26531bac1838e519c2c6562ac717b22aac041730f0d753d3ad35b76b5f4924"

	c, err := NewSecureTree(prvKey)
	assert.Nil(t, err)

	root, err := c.GetNodeByPath("/")
	assert.Nil(t, err)
	secureNode := root.(*AdapterSecureNodeCRDT)

	t.Run("Reject CreateMapNode with invalid key", func(t *testing.T) {
		_, err := secureNode.CreateMapNode(prvKeyInvalid)
		assert.NotNil(t, err)
		assert.Contains(t, err.Error(), "not allowed")
	})

	t.Run("Allow CreateMapNode with valid key", func(t *testing.T) {
		mapNode, err := secureNode.CreateMapNode(prvKey)
		assert.Nil(t, err)

		_, ok := mapNode.(*AdapterSecureNodeCRDT)
		assert.True(t, ok, "returned node should be of type *AdapterSecureNodeCRDT")
	})
}

func TestSecureTreeAdapterSetKeyValue(t *testing.T) {
	prvKey := "d6eb959e9aec2e6fdc44b5862b269e987b8a4d6f2baca542d8acaa97ee5e74f6"
	prvKeyInvalid := "ed26531bac1838e519c2c6562ac717b22aac041730f0d753d3ad35b76b5f4924"

	c, err := NewSecureTree(prvKey)
	assert.Nil(t, err)

	// Get the root node and create a map node under it
	root, err := c.GetNodeByPath("/")
	assert.Nil(t, err)
	mapNode, err := root.(*AdapterSecureNodeCRDT).CreateMapNode(prvKey)
	assert.Nil(t, err)

	t.Run("Reject SetKeyValue on map node with invalid key", func(t *testing.T) {
		_, err := mapNode.SetKeyValue("someKey", "someValue", prvKeyInvalid)
		assert.NotNil(t, err)
		assert.Contains(t, err.Error(), "not allowed")
	})

	t.Run("Allow SetKeyValue on map node with valid key", func(t *testing.T) {
		nodeID, err := mapNode.SetKeyValue("someKey", "someValue", prvKey)
		assert.Nil(t, err)
		assert.NotEmpty(t, nodeID)

		// Optionally verify the key is accessible
		childNode, err := c.GetNodeByPath("/someKey")
		assert.Nil(t, err)
		assert.NotNil(t, childNode)
	})
}

func TestSecureTreeAdapterRemoveKeyValue(t *testing.T) {
	prvKey := "d6eb959e9aec2e6fdc44b5862b269e987b8a4d6f2baca542d8acaa97ee5e74f6"
	prvKeyInvalid := "ed26531bac1838e519c2c6562ac717b22aac041730f0d753d3ad35b76b5f4924"

	c, err := NewSecureTree(prvKey)
	assert.Nil(t, err)

	// Create map node under root and add a key-value
	root, err := c.GetNodeByPath("/")
	assert.Nil(t, err)
	mapNode, err := root.(*AdapterSecureNodeCRDT).CreateMapNode(prvKey)
	assert.Nil(t, err)

	_, err = mapNode.SetKeyValue("keyToRemove", "value", prvKey)
	assert.Nil(t, err)

	t.Run("Reject RemoveKeyValue with invalid key", func(t *testing.T) {
		err := mapNode.RemoveKeyValue("keyToRemove", prvKeyInvalid)
		assert.NotNil(t, err)
		assert.Contains(t, err.Error(), "not allowed")
	})

	t.Run("Allow RemoveKeyValue with valid key", func(t *testing.T) {
		err := mapNode.RemoveKeyValue("keyToRemove", prvKey)
		assert.Nil(t, err)

		// Confirm the key no longer exists
		_, err = c.GetNodeByPath("/keyToRemove")
		assert.NotNil(t, err)
	})
}

func TestSecureTreeAdapterCreateAttachedNode(t *testing.T) {
	prvKey := "d6eb959e9aec2e6fdc44b5862b269e987b8a4d6f2baca542d8acaa97ee5e74f6"
	prvKeyInvalid := "ed26531bac1838e519c2c6562ac717b22aac041730f0d753d3ad35b76b5f4924"

	c, err := NewSecureTree(prvKey)
	assert.Nil(t, err)

	// Create a parent map node under root
	root, err := c.GetNodeByPath("/")
	assert.Nil(t, err)
	parentNode, err := root.(*AdapterSecureNodeCRDT).CreateMapNode(prvKey)
	assert.Nil(t, err)
	parentID := parentNode.(*AdapterSecureNodeCRDT).nodeCrdt.ID

	t.Run("Reject CreateAttachedNode with invalid key", func(t *testing.T) {
		_, err := c.CreateAttachedNode("child", Literal, parentID, prvKeyInvalid)
		assert.NotNil(t, err)
		assert.Contains(t, err.Error(), "not allowed")
	})

	t.Run("Allow CreateAttachedNode with valid key", func(t *testing.T) {
		childNode, err := c.CreateAttachedNode("child", Map, parentID, prvKey)
		assert.Nil(t, err)
		assert.NotNil(t, childNode)
	})
}

func TestSecureTreeAdapterCreateNode(t *testing.T) {
	prvKey := "d6eb959e9aec2e6fdc44b5862b269e987b8a4d6f2baca542d8acaa97ee5e74f6"
	prvKeyInvalid := "ed26531bac1838e519c2c6562ac717b22aac041730f0d753d3ad35b76b5f4924"

	c, err := NewSecureTree(prvKey)
	assert.Nil(t, err)

	t.Run("Reject CreateNode with invalid key", func(t *testing.T) {
		_, err := c.CreateNode("myNode", Map, prvKeyInvalid)
		assert.Nil(t, err) // This is actually ok, as long as the node is not attached to the tree
	})

	t.Run("Allow CreateNode with valid key", func(t *testing.T) {
		node, err := c.CreateNode("myNode", Map, prvKey)
		assert.Nil(t, err)
		assert.NotNil(t, node)
	})
}

func TestSecureTreeAdapterAddEdge(t *testing.T) {
	prvKey := "d6eb959e9aec2e6fdc44b5862b269e987b8a4d6f2baca542d8acaa97ee5e74f6"
	prvKeyInvalid := "ed26531bac1838e519c2c6562ac717b22aac041730f0d753d3ad35b76b5f4924"

	c, err := NewSecureTree(prvKey)
	assert.Nil(t, err)

	// Create fromNode under root
	root, err := c.GetNodeByPath("/")
	assert.Nil(t, err)

	fromNode, err := root.(*AdapterSecureNodeCRDT).CreateMapNode(prvKey)
	assert.Nil(t, err)
	fromNodeID := fromNode.(*AdapterSecureNodeCRDT).nodeCrdt.ID

	// Create toNode as detached node (not attached to root)
	toNode, err := c.CreateNode("detachedNode", Map, prvKey)
	assert.Nil(t, err)
	toNodeID := toNode.(*AdapterSecureNodeCRDT).nodeCrdt.ID

	t.Run("Reject AddEdge with invalid key", func(t *testing.T) {
		err := c.AddEdge(fromNodeID, toNodeID, "edgeLabel", prvKeyInvalid)
		assert.NotNil(t, err)
		assert.Contains(t, err.Error(), "not allowed")
	})

	t.Run("Allow AddEdge with valid key", func(t *testing.T) {
		err := c.AddEdge(fromNodeID, toNodeID, "edgeLabel", prvKey)
		assert.Nil(t, err)
	})
}

func TestSecureTreeAdapterRemoveEdge(t *testing.T) {
	prvKey := "d6eb959e9aec2e6fdc44b5862b269e987b8a4d6f2baca542d8acaa97ee5e74f6"
	prvKeyInvalid := "ed26531bac1838e519c2c6562ac717b22aac041730f0d753d3ad35b76b5f4924"

	c, err := NewSecureTree(prvKey)
	assert.Nil(t, err)

	// Create fromNode under root
	root, err := c.GetNodeByPath("/")
	assert.Nil(t, err)

	fromNode, err := root.(*AdapterSecureNodeCRDT).CreateMapNode(prvKey)
	assert.Nil(t, err)
	fromNodeID := fromNode.(*AdapterSecureNodeCRDT).nodeCrdt.ID

	// Create toNode as detached node
	toNode, err := c.CreateNode("detachedNode", Map, prvKey)
	assert.Nil(t, err)
	toNodeID := toNode.(*AdapterSecureNodeCRDT).nodeCrdt.ID

	// First: Add the edge (valid)
	err = c.AddEdge(fromNodeID, toNodeID, "edgeLabel", prvKey)
	assert.Nil(t, err)

	t.Run("Reject RemoveEdge with invalid key", func(t *testing.T) {
		err := c.RemoveEdge(fromNodeID, toNodeID, prvKeyInvalid)
		assert.NotNil(t, err)
		assert.Contains(t, err.Error(), "not allowed")
	})

	t.Run("Allow RemoveEdge with valid key", func(t *testing.T) {
		err := c.RemoveEdge(fromNodeID, toNodeID, prvKey)
		assert.Nil(t, err)
	})
}

func TestSecureTreeAdapterAppendEdge(t *testing.T) {
	prvKey := "d6eb959e9aec2e6fdc44b5862b269e987b8a4d6f2baca542d8acaa97ee5e74f6"
	prvKeyInvalid := "ed26531bac1838e519c2c6562ac717b22aac041730f0d753d3ad35b76b5f4924"

	c, err := NewSecureTree(prvKey)
	assert.Nil(t, err)

	// Create fromNode under root
	root, err := c.GetNodeByPath("/")
	assert.Nil(t, err)

	fromNode, err := root.(*AdapterSecureNodeCRDT).CreateMapNode(prvKey)
	assert.Nil(t, err)
	fromNodeID := fromNode.(*AdapterSecureNodeCRDT).nodeCrdt.ID

	// Create toNode as detached node
	toNode, err := c.CreateNode("detachedNode", Map, prvKey)
	assert.Nil(t, err)
	toNodeID := toNode.(*AdapterSecureNodeCRDT).nodeCrdt.ID

	t.Run("Reject AppendEdge with invalid key", func(t *testing.T) {
		err := c.AppendEdge(fromNodeID, toNodeID, "edgeLabel", prvKeyInvalid)
		assert.NotNil(t, err)
		assert.Contains(t, err.Error(), "not allowed")
	})

	t.Run("Allow AppendEdge with valid key", func(t *testing.T) {
		err := c.AppendEdge(fromNodeID, toNodeID, "edgeLabel", prvKey)
		assert.Nil(t, err)
	})
}

func TestSecureTreeAdapterPrependEdge(t *testing.T) {
	prvKey := "d6eb959e9aec2e6fdc44b5862b269e987b8a4d6f2baca542d8acaa97ee5e74f6"
	prvKeyInvalid := "ed26531bac1838e519c2c6562ac717b22aac041730f0d753d3ad35b76b5f4924"

	c, err := NewSecureTree(prvKey)
	assert.Nil(t, err)

	// Create fromNode under root
	root, err := c.GetNodeByPath("/")
	assert.Nil(t, err)

	fromNode, err := root.(*AdapterSecureNodeCRDT).CreateMapNode(prvKey)
	assert.Nil(t, err)
	fromNodeID := fromNode.(*AdapterSecureNodeCRDT).nodeCrdt.ID

	// Create toNode as detached node
	toNode, err := c.CreateNode("detachedNode", Map, prvKey)
	assert.Nil(t, err)
	toNodeID := toNode.(*AdapterSecureNodeCRDT).nodeCrdt.ID

	t.Run("Reject PrependEdge with invalid key", func(t *testing.T) {
		err := c.PrependEdge(fromNodeID, toNodeID, "edgeLabel", prvKeyInvalid)
		assert.NotNil(t, err)
		assert.Contains(t, err.Error(), "not allowed")
	})

	t.Run("Allow PrependEdge with valid key", func(t *testing.T) {
		err := c.PrependEdge(fromNodeID, toNodeID, "edgeLabel", prvKey)
		assert.Nil(t, err)
	})
}

func TestSecureTreeAdapterInsertEdgeLeft(t *testing.T) {
	prvKey := "d6eb959e9aec2e6fdc44b5862b269e987b8a4d6f2baca542d8acaa97ee5e74f6"
	prvKeyInvalid := "ed26531bac1838e519c2c6562ac717b22aac041730f0d753d3ad35b76b5f4924"

	c, err := NewSecureTree(prvKey)
	assert.Nil(t, err)

	// Create fromNode under root
	root, err := c.GetNodeByPath("/")
	assert.Nil(t, err)

	fromNode, err := root.(*AdapterSecureNodeCRDT).CreateMapNode(prvKey)
	assert.Nil(t, err)
	fromNodeID := fromNode.(*AdapterSecureNodeCRDT).nodeCrdt.ID

	// Create sibling node (first edge)
	siblingNode, err := c.CreateNode("siblingNode", Map, prvKey)
	assert.Nil(t, err)
	siblingNodeID := siblingNode.(*AdapterSecureNodeCRDT).nodeCrdt.ID

	// Add sibling edge first
	err = c.AppendEdge(fromNodeID, siblingNodeID, "edgeLabel", prvKey)
	assert.Nil(t, err)

	// Create toNode (node we want to insert to the left of sibling)
	toNode, err := c.CreateNode("toNode", Map, prvKey)
	assert.Nil(t, err)
	toNodeID := toNode.(*AdapterSecureNodeCRDT).nodeCrdt.ID

	t.Run("Reject InsertEdgeLeft with invalid key", func(t *testing.T) {
		err := c.InsertEdgeLeft(fromNodeID, toNodeID, "edgeLabel", siblingNodeID, prvKeyInvalid)
		assert.NotNil(t, err)
		assert.Contains(t, err.Error(), "not allowed")
	})

	t.Run("Allow InsertEdgeLeft with valid key", func(t *testing.T) {
		err := c.InsertEdgeLeft(fromNodeID, toNodeID, "edgeLabel", siblingNodeID, prvKey)
		assert.Nil(t, err)
	})
}

func TestSecureTreeAdapterInsertEdgeRight(t *testing.T) {
	prvKey := "d6eb959e9aec2e6fdc44b5862b269e987b8a4d6f2baca542d8acaa97ee5e74f6"
	prvKeyInvalid := "ed26531bac1838e519c2c6562ac717b22aac041730f0d753d3ad35b76b5f4924"

	c, err := NewSecureTree(prvKey)
	assert.Nil(t, err)

	// Create fromNode under root
	root, err := c.GetNodeByPath("/")
	assert.Nil(t, err)

	fromNode, err := root.(*AdapterSecureNodeCRDT).CreateMapNode(prvKey)
	assert.Nil(t, err)
	fromNodeID := fromNode.(*AdapterSecureNodeCRDT).nodeCrdt.ID

	// Create sibling node (first edge)
	siblingNode, err := c.CreateNode("siblingNode", Map, prvKey)
	assert.Nil(t, err)
	siblingNodeID := siblingNode.(*AdapterSecureNodeCRDT).nodeCrdt.ID

	// Add sibling edge first
	err = c.AppendEdge(fromNodeID, siblingNodeID, "edgeLabel", prvKey)
	assert.Nil(t, err)

	// Create toNode (node we want to insert to the right of sibling)
	toNode, err := c.CreateNode("toNode", Map, prvKey)
	assert.Nil(t, err)
	toNodeID := toNode.(*AdapterSecureNodeCRDT).nodeCrdt.ID

	t.Run("Reject InsertEdgeRight with invalid key", func(t *testing.T) {
		err := c.InsertEdgeRight(fromNodeID, toNodeID, "edgeLabel", siblingNodeID, prvKeyInvalid)
		assert.NotNil(t, err)
		assert.Contains(t, err.Error(), "not allowed")
	})

	t.Run("Allow InsertEdgeRight with valid key", func(t *testing.T) {
		err := c.InsertEdgeRight(fromNodeID, toNodeID, "edgeLabel", siblingNodeID, prvKey)
		assert.Nil(t, err)
	})
}

func TestSecureTreeAdapterImportJSON(t *testing.T) {
	prvKey := "d6eb959e9aec2e6fdc44b5862b269e987b8a4d6f2baca542d8acaa97ee5e74f6"
	prvKeyInvalid := "ed26531bac1838e519c2c6562ac717b22aac041730f0d753d3ad35b76b5f4924"

	c, err := NewSecureTree(prvKey)
	assert.Nil(t, err)

	// Example JSON structure
	jsonData := []byte(`{
		"foo": "bar",
		"baz": 123
	}`)

	t.Run("Reject ImportJSON with invalid key", func(t *testing.T) {
		_, err := c.ImportJSON(jsonData, prvKeyInvalid)
		assert.NotNil(t, err)
		assert.Contains(t, err.Error(), "not allowed")
	})

	t.Run("Allow ImportJSON with valid key", func(t *testing.T) {
		nodeID, err := c.ImportJSON(jsonData, prvKey)
		assert.Nil(t, err)
		assert.NotEmpty(t, nodeID)

		// OPTIONAL: Verify that keys are accessible
		nodeFoo, err := c.GetNodeByPath("/foo")
		assert.Nil(t, err)
		assert.NotNil(t, nodeFoo)

		nodeBaz, err := c.GetNodeByPath("/baz")
		assert.Nil(t, err)
		assert.NotNil(t, nodeBaz)
	})
}

func TestSecureTreeAdapterImportJSONToMap(t *testing.T) {
	prvKey := "d6eb959e9aec2e6fdc44b5862b269e987b8a4d6f2baca542d8acaa97ee5e74f6"
	prvKeyInvalid := "ed26531bac1838e519c2c6562ac717b22aac041730f0d753d3ad35b76b5f4924"

	c, err := NewSecureTree(prvKey)
	assert.Nil(t, err)

	// Create parent map node under root
	root, err := c.GetNodeByPath("/")
	assert.Nil(t, err)

	parentMapNode, err := root.(*AdapterSecureNodeCRDT).CreateMapNode(prvKey)
	assert.Nil(t, err)
	parentID := parentMapNode.(*AdapterSecureNodeCRDT).nodeCrdt.ID

	// Example JSON to import
	jsonData := []byte(`{
		"nestedFoo": "value1",
		"nestedBar": 42
	}`)

	t.Run("Reject ImportJSONToMap with invalid key", func(t *testing.T) {
		_, err := c.ImportJSONToMap(jsonData, parentID, "childKey", prvKeyInvalid)
		assert.NotNil(t, err)
		assert.Contains(t, err.Error(), "not allowed")
	})

	t.Run("Allow ImportJSONToMap with valid key", func(t *testing.T) {
		nodeID, err := c.ImportJSONToMap(jsonData, parentID, "childKey", prvKey)
		assert.Nil(t, err)
		assert.NotEmpty(t, nodeID)
	})
}

func TestSecureTreeAdapterImportJSONToArray(t *testing.T) {
	prvKey := "d6eb959e9aec2e6fdc44b5862b269e987b8a4d6f2baca542d8acaa97ee5e74f6"
	prvKeyInvalid := "ed26531bac1838e519c2c6562ac717b22aac041730f0d753d3ad35b76b5f4924"

	c, err := NewSecureTree(prvKey)
	assert.Nil(t, err)

	// Create parent array node under root
	root, err := c.GetNodeByPath("/")
	assert.Nil(t, err)

	parentArrayNode, err := root.(*AdapterSecureNodeCRDT).CreateMapNode(prvKey)
	assert.Nil(t, err)

	// Now under parentArrayNode, add an array key
	parentID := parentArrayNode.(*AdapterSecureNodeCRDT).nodeCrdt.ID

	arrayNode, err := c.CreateNode("arrayKey", Array, prvKey)
	assert.Nil(t, err)
	arrayNodeID := arrayNode.(*AdapterSecureNodeCRDT).nodeCrdt.ID

	// Link the array node under parent map node
	err = c.AppendEdge(parentID, arrayNodeID, "arrayKey", prvKey)
	assert.Nil(t, err)

	// Example array JSON
	jsonData := []byte(`[
		"elem1",
		"elem2",
		"elem3"
	]`)

	t.Run("Reject ImportJSONToArray with invalid key", func(t *testing.T) {
		_, err := c.ImportJSONToArray(jsonData, arrayNodeID, prvKeyInvalid)
		assert.NotNil(t, err)
		assert.Contains(t, err.Error(), "not allowed")
	})

	t.Run("Allow ImportJSONToArray with valid key", func(t *testing.T) {
		nodeID, err := c.ImportJSONToArray(jsonData, arrayNodeID, prvKey)
		assert.Nil(t, err)
		assert.NotEmpty(t, nodeID)
	})
}

func TestSecureTreeAdapterMerge(t *testing.T) {
	prvKey := "d6eb959e9aec2e6fdc44b5862b269e987b8a4d6f2baca542d8acaa97ee5e74f6"

	c1, err := NewSecureTree(prvKey)
	assert.Nil(t, err)

	jsonData := []byte(`{
		"foo": "bar",
		"baz": 123
	}`)

	c1.ImportJSON(jsonData, prvKey)

	c2, err := c1.Clone()

	mapNode, err := c2.GetNodeByPath("/")
	assert.Nil(t, err)

	valueNodeID, err := mapNode.SetKeyValue("newKey", "newValue", prvKey)
	valueNode, ok := c2.GetNode(valueNodeID)
	assert.True(t, ok, "GetNode should return the node")
	assert.NotNil(t, valueNode, "valueNode should not be nil")
	oldSignature := valueNode.(*AdapterSecureNodeCRDT).nodeCrdt.Signature
	valueNode.(*AdapterSecureNodeCRDT).nodeCrdt.Signature = "e713a1bb015fecabb5a084b0fe6d6e7271fca6f79525a634183cfdb175fe69241f4da161779d8e6b761200e1cf93766010a19072fa778f9643363e2cfadd640900" // Invalid signature for testing
	assert.Nil(t, err, "SetKeyValue should return an error for invalid private key")

	err = c1.Merge(c2, prvKey)
	assert.NotNil(t, err, "Merge should return an error since c2 has a node with an invalid signature")

	// Restore the original signature for a valid merge
	valueNode.(*AdapterSecureNodeCRDT).nodeCrdt.Signature = oldSignature

	err = c1.Merge(c2, prvKey)
	assert.Nil(t, err, "Merge should not return an error after restoring the signature")
}

func TestSecureTreeAdapterMergeABAC(t *testing.T) {
	prvKey1 := "d6eb959e9aec2e6fdc44b5862b269e987b8a4d6f2baca542d8acaa97ee5e74f6"
	prvKey2 := "ed26531bac1838e519c2c6562ac717b22aac041730f0d753d3ad35b76b5f4924"

	identity2, err := crypto.CreateIdendityFromString(prvKey2)
	assert.Nil(t, err)

	c1, err := NewSecureTree(prvKey1)
	assert.Nil(t, err)

	jsonData := []byte(`{
		"foo": "bar",
		"baz": 123
	}`)

	c1.ImportJSON(jsonData, prvKey1)

	c2, err := c1.Clone()

	mapNode, err := c2.GetNodeByPath("/")
	assert.Nil(t, err)

	valueNodeID, err := mapNode.SetKeyValue("newKey", "newValue", prvKey2)
	assert.Error(t, err, "SetKeyValue should return an error for prvKey2 since identity2 is not allowed to modify the root node")

	c2.ABAC().Allow(identity2.ID(), ActionModify, "root", true)

	valueNodeID, err = mapNode.SetKeyValue("newKey", "newValue", prvKey2)
	assert.NoError(t, err, "SetKeyValue should not return an error for prvKey2")

	valueNode, ok := c2.GetNode(valueNodeID)
	assert.True(t, ok, "GetNode should return the node")
	assert.NotNil(t, valueNode, "valueNode should not be nil")

	err = c1.Merge(c2, prvKey1)
	assert.NotNil(t, err, "Merge should return an error since identity2 is not allowed to modify the root node")

	c1.ABAC().Allow(identity2.ID(), ActionModify, "root", true)

	err = c1.Merge(c2, prvKey1)
	assert.Nil(t, err, "Merge should not return an error after restoring the signature")
}

func TestSecureTreeAdapterMergeComplexJSONABAC(t *testing.T) {
	prvKey1 := "d6eb959e9aec2e6fdc44b5862b269e987b8a4d6f2baca542d8acaa97ee5e74f6"
	prvKey2 := "ed26531bac1838e519c2c6562ac717b22aac041730f0d753d3ad35b76b5f4924"

	json := []byte(`{
	  "1": [
	    {
	      "2": "3"
	    },
	    {
	      "4": [
	        {
	          "5": "6"
	        }
	      ]
	    }
	  ]
	}`)

	identity2, err := crypto.CreateIdendityFromString(prvKey2)
	assert.Nil(t, err)

	c1, err := NewSecureTree(prvKey1)
	assert.Nil(t, err)

	_, err = c1.ImportJSON(json, prvKey1)
	assert.Nil(t, err, "ImportJSON should not return an error")

	c2, err := NewSecureTree(prvKey2)
	assert.Nil(t, err)
	_, err = c2.ImportJSON(json, prvKey2)
	assert.Nil(t, err, "ImportJSON should not return an error")

	err = c1.Merge(c2, prvKey1)
	assert.NotNil(t, err, "Merge should return an error since identity2 is not allowed to modify the root node")

	c1.ABAC().Allow(identity2.ID(), ActionModify, "root", true)

	err = c1.Merge(c2, prvKey1)
	assert.Nil(t, err, "Merge should not return an error after restoring the signature")
}

func TestSecureTreeAdapterSave(t *testing.T) {
	prvKey := "d6eb959e9aec2e6fdc44b5862b269e987b8a4d6f2baca542d8acaa97ee5e74f6"

	json := []byte(`{
	  "1": [
	    {
	      "2": "3"
	    },
	    {
	      "4": [
	        {
	          "5": "6"
	        }
	      ]
	    }
	  ]
	}`)

	c1, err := NewSecureTree(prvKey)
	assert.Nil(t, err)

	_, err = c1.ImportJSON(json, prvKey)
	assert.Nil(t, err, "ImportJSON should not return an error")

	savedData, err := c1.Save()
	assert.Nil(t, err, "Save should not return an error")

	c2, err := NewSecureTree(prvKey)
	assert.Nil(t, err)

	err = c2.Load(savedData)
	assert.Nil(t, err, "Load should not return an error")

	adapterC1 := c1.(*AdapterSecureTreeCRDT)
	adapterC2 := c2.(*AdapterSecureTreeCRDT)
	assert.Equal(t, adapterC1.treeCrdt.ABACPolicy.identity.ID(), adapterC2.treeCrdt.ABACPolicy.identity.ID(), "ABAC identity should match after save and load")

	// Try to modify the ABAC owner
	hackedC2, err := c2.Clone()
	assert.Nil(t, err, "Clone should not return an error")
	hackedC2.(*AdapterSecureTreeCRDT).treeCrdt.ABACPolicy.OwnerID = "ff4d4028f7a41edca91c01d17da4c4c3edb18950ac98b465cb918ad5362c5bdc"
	savedData, err = hackedC2.Save()
	assert.Nil(t, err, "Save should return an error when trying to modify the ABAC owner")
	c3, err := NewSecureTree(prvKey)
	assert.Nil(t, err, "NewSecureTree should not return an error")
	err = c3.Load(savedData)
	assert.NotNil(t, err, "Load should return an error when trying to load a tree with a modified ABAC owner")

	// Try to modify the ABAC rules
	savedData, err = c2.Save()
	assert.Nil(t, err, "Save should not return an error")
	hackedC3, err := NewSecureTree("ff4d4028f7a41edca91c01d17da4c4c3edb18950ac98b465cb918ad5362c5bdc")
	assert.Nil(t, err, "NewSecureTree should not return an error")
	err = hackedC3.Load(savedData)
	assert.Nil(t, err, "Load should not return an error when loading a tree with a different ABAC owner")
	err = hackedC3.ABAC().Allow("ff4d4028f7a41edca91c01d17da4c4c3edb18950ac98b465cb918ad5362c5bdc", ActionModify, "root", true)
	assert.Nil(t, err, "Allow should not return an error when modifying ABAC rules")

	// Try to add map key value
	mapNode, err := hackedC3.GetNodeByPath("/1")
	assert.Nil(t, err, "GetNodeByPath should not return an error")
	_, err = mapNode.SetKeyValue("newKey", "newValue", "ff4d4028f7a41edca91c01d17da4c4c3edb18950ac98b465cb918ad5362c5bdc")
	assert.NotNil(t, err, "SetKeyValue should not return an error when modifying ABAC rules")
}

type DummyTree struct{}

func (t *DummyTree) isDescendant(root NodeID, target NodeID) bool {
	// Simple implementation → no hierarchy for this test
	return false
}

func TestABACPolicyMerge_LWW(t *testing.T) {
	// Setup identities
	identityA, err := crypto.CreateIdendity()
	assert.NoError(t, err)

	identityB, err := crypto.CreateIdendity()
	assert.NoError(t, err)

	ownerA := identityA.ID()
	ownerB := identityB.ID()

	// Setup trees
	tree := &DummyTree{}

	// Create ABACPolicy A
	policyA := NewABACPolicy(tree, ownerA, identityA)
	err = policyA.Allow("client1", ActionModify, "node1", false)
	assert.NoError(t, err)

	// Simulate more updates → bump clock for A
	for i := 0; i < 3; i++ {
		err = policyA.Allow("client1", ActionRead, NodeID("nodeX"), false)
		assert.NoError(t, err)
	}

	// Create ABACPolicy B
	policyB := NewABACPolicy(tree, ownerB, identityB)
	err = policyB.Allow("client2", ActionModify, "node2", true)
	assert.NoError(t, err)

	// B will be "newer" → simulate a higher clock
	for i := 0; i < 5; i++ {
		err = policyB.Allow("client2", ActionRead, NodeID("nodeY"), true)
		assert.NoError(t, err)
	}

	// Sanity: verify signatures
	_, err = policyA.Verify()
	assert.NoError(t, err)

	_, err = policyB.Verify()
	assert.NoError(t, err)

	// Now: Merge B into A
	err = policyA.Merge(policyB)
	assert.NoError(t, err)

	// After merge: policyA should now equal policyB
	assert.Equal(t, policyA.Clock, policyB.Clock)
	assert.Equal(t, policyA.OwnerID, policyB.OwnerID)
	assert.Equal(t, policyA.Rules, policyB.Rules)

	// Verify the merged policy signature
	_, err = policyA.Verify()
	assert.NoError(t, err)

	// Check that an allowed rule from B is now present
	allowed := policyA.IsAllowed("client2", ActionModify, "node2")
	assert.True(t, allowed, "Expected client2 to be allowed after merge")

	// Check that old client1 rule may have been replaced
	allowedClient1 := policyA.IsAllowed("client1", ActionModify, "node1")
	// Depending on clock dominance, this may be true or false:
	// In strict LWW → B wins → client1's rules gone
	assert.False(t, allowedClient1, "Expected client1 to lose after merge")
}

func TestSecureTreeSetLiteralt(t *testing.T) {
	// Setup identities
	prvKey := "b24b6cf725a6d0e12955ff35a470c823eaac6dbbe0feb5503a097ed5baca5328"

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

	c, err := NewSecureTree(prvKey)
	assert.Nil(t, err, "Failed to create new secure tree")

	_, err = c.ImportJSON(originalJSON, prvKey)
	assert.Nil(t, err, "Failed to import JSON into secure tree")

	c2, err := c.Clone()
	assert.Nil(t, err, "Failed to clone secure tree")

	node, err := c2.GetNodeByPath("/friends/0/name")
	assert.Nil(t, err, "Failed to get node by path")

	err = node.SetLiteral("Johan2", prvKey)
	assert.Nil(t, err, "Failed to set literal on node")

	err = c.Merge(c2, prvKey)
	assert.Nil(t, err, "Failed to merge secure trees")
}
