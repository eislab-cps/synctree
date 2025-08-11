package crdt

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/eislab-cps/synctree/pkg/core"
	"github.com/eislab-cps/synctree/pkg/random"
	"github.com/eislab-cps/synctree/pkg/utils"
	"github.com/eislab-cps/synctree/pkg/vectorclock"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestTreeCRDTSetFieldArrays(t *testing.T) {
	clientID := core.ClientID(random.GenerateRandomID())

	json := []byte(`{
	  "a": [
	    {
	      "2": "3"
	    }
	  ]
	}`)

	c := NewTreeCRDT()
	_, err := c.ImportJSON(json, clientID)
	assert.NoError(t, err)
}

func TestTreeCRDTSetFieldsConflictLastWriterWins(t *testing.T) {
	logrus.SetLevel(logrus.WarnLevel)

	c1 := NewTreeCRDT()

	clientID1 := core.ClientID(random.GenerateRandomID())
	clientID2 := core.ClientID(random.GenerateRandomID())

	rootC1 := c1.Root

	mapNodeC1, err := rootC1.CreateMapNode(clientID1)
	assert.NoError(t, err, "CreateMapNode should not return an error")

	_, _, err = mapNodeC1.SetKeyValue("key", "value1", clientID1)
	assert.NoError(t, err, "SetKeyValue should not return an error")

	c2, err := c1.Clone()

	assert.NoError(t, err, "Clone should not return an error")
	mapNodeC2, ok := c2.GetNode(mapNodeC1.ID)
	assert.True(t, ok, "Node should exist in cloned graph")

	_, _, err = mapNodeC2.SetKeyValue("key", "value2", clientID2)
	assert.NoError(t, err, "SetKeyValue should not return an error")

	_, _, err = mapNodeC2.SetKeyValue("key", "value3", clientID2)
	assert.NoError(t, err, "SetKeyValue should not return an error")

	_, _, err = mapNodeC1.SetKeyValue("key", "value4", clientID1) // Will be overwritten by c2
	assert.NoError(t, err, "SetKeyValue should not return an error")

	err = c1.Merge(c2)
	assert.NoError(t, err, "Merge should not return an error")

	exportedJSON, err := c1.ExportJSON()
	assert.NoError(t, err, "ExportToJSON should not return an error")

	expectedJSON := []byte(`{"key":"value3"}`)
	utils.CompareJSON(t, expectedJSON, exportedJSON)
}

func TestTreeCRDTSetFieldsConflictNodeIDTieBraker(t *testing.T) {
	logrus.SetLevel(logrus.WarnLevel)

	c1 := NewTreeCRDT()

	clientID1 := core.ClientID(random.GenerateRandomID())
	clientID2 := core.ClientID(random.GenerateRandomID())

	rootC1 := c1.Root

	mapNodeC1, err := rootC1.CreateMapNode(clientID1)
	assert.NoError(t, err, "CreateMapNode should not return an error")

	_, _, err = mapNodeC1.SetKeyValue("key", "value1", clientID1)
	assert.NoError(t, err, "SetKeyValue should not return an error")

	c2, err := c1.Clone()

	assert.NoError(t, err, "Clone should not return an error")
	mapNodeC2, ok := c2.GetNode(mapNodeC1.ID)
	assert.True(t, ok, "Node should exist in cloned graph")

	_, _, err = mapNodeC2.SetKeyValue("key", "value2", clientID2)
	assert.NoError(t, err, "SetKeyValue should not return an error")

	// Conflict, both clients have the same vector clock version
	_, _, err = mapNodeC1.SetKeyValue("key", "value3", clientID1)
	assert.NoError(t, err, "SetKeyValue should not return an error")

	err = c1.Merge(c2) // Enable conflict resolution with tie-breaker
	assert.NoError(t, err, "Merge should not return an error")

	exportedJSON, err := c1.ExportJSON()
	assert.NoError(t, err, "ExportToJSON should not return an error")

	// This test will result in a conflict resolution where client IDs will be used as tie-breakers.
	if clientID1 < clientID2 {
		expectedJSON := []byte(`{"key":"value3"}`)
		utils.CompareJSON(t, expectedJSON, exportedJSON)
	} else {
		expectedJSON := []byte(`{"key":"value2"}`)
		utils.CompareJSON(t, expectedJSON, exportedJSON)
	}
}

func TestTreeCRDTNodeRemoveField(t *testing.T) {
	c := NewTreeCRDT()

	clientID := core.ClientID(random.GenerateRandomID())

	mapNode, err := c.Root.CreateMapNode(clientID)
	assert.NoError(t, err, "CreateMapNode should not return an error")

	_, _, err = mapNode.SetKeyValue("key1", "value1", clientID)
	assert.NoError(t, err, "SetKeyValue should not return an error")

	_, _, err = mapNode.SetKeyValue("key2", "value1", clientID)
	assert.NoError(t, err, "SetKeyValue should not return an error")

	valueNode, found, err := mapNode.GetNodeForKey("key1")
	assert.NoError(t, err, "GetValueNode should not return an error")
	assert.NotNil(t, valueNode, "Value node for key1 should exist")
	assert.True(t, found, "Value node for key1 should be found")

	valueNode, found, err = mapNode.GetNodeForKey("key2")
	assert.NoError(t, err, "GetValueNode should not return an error")
	assert.NotNil(t, valueNode, "Value node for key2 should exist")
	assert.True(t, found, "Value node for key2 should be found")

	// Remove key1
	err = mapNode.RemoveKeyValue("key1", clientID)
	assert.NoError(t, err, "RemoveKeyValue should not return an error")

	// Check if key1 is removed
	valueNode, found, err = mapNode.GetNodeForKey("key1")
	assert.NoError(t, err, "GetValueNode should not return an error")
	assert.Nil(t, valueNode, "Value node for key1 should be nil after removal")
	assert.False(t, found, "Value node for key1 should not be found after removal")

	// Check if key2 still exists
	valueNode, found, err = mapNode.GetNodeForKey("key2")
	assert.NoError(t, err, "GetValueNode should not return an error")
	assert.NotNil(t, valueNode, "Value node for key2 should still exist")
	assert.True(t, found, "Value node for key2 should still be found after removal of key1")
}

func TestTreeCRDTAddEdgeWithVersion(t *testing.T) {
	c := NewTreeCRDT()

	// To make the test deterministic, we will use fixed client IDs
	clientID := core.ClientID("bbbb")
	otherClientID := core.ClientID("aaaa")

	parent := c.CreateAttachedNode("parent", Map, c.Root.ID, clientID)
	child := c.CreateAttachedNode("child", Map, c.Root.ID, clientID)

	// 1. Add an edge with version 1
	err := c.addEdgeWithVersion(parent.ID, child.ID, "link", clientID, 1)
	assert.Nil(t, err, "AddEdgeWithVersion should not return error")

	assert.Equal(t, 1, len(parent.Edges), "Expected 1 edge")
	assert.Equal(t, child.ID, parent.Edges[0].To, "Edge should point to child")
	assert.Equal(t, "link", parent.Edges[0].Label, "Edge label mismatch")

	// 2. Add another edge with higher version (should succeed)
	anotherChild := c.CreateAttachedNode("another_child", Map, c.Root.ID, clientID)
	err = c.addEdgeWithVersion(parent.ID, anotherChild.ID, "link2", clientID, 2)
	assert.Nil(t, err, "AddEdgeWithVersion second time should not return error")

	assert.Equal(t, 2, len(parent.Edges), "Expected 2 edges now")

	// 3. Try to add conflicting edge with lower version (should be ignored)
	fakeChild := c.CreateAttachedNode("fake_child", Map, c.Root.ID, clientID)
	err = c.addEdgeWithVersion(parent.ID, fakeChild.ID, "fake_link", clientID, 1) // lower version
	assert.Nil(t, err, "AddEdgeWithVersion with lower version should not error")

	found := false
	for _, edge := range parent.Edges {
		if edge.To == fakeChild.ID {
			found = true
			break
		}
	}
	assert.False(t, found, "Edge with lower version should not overwrite or add")

	// 4. Simulate a tie with another client (new client id)
	tieChild := c.CreateAttachedNode("tie_child", Map, c.Root.ID, otherClientID)
	err = c.addEdgeWithVersion(parent.ID, tieChild.ID, "tie_link", otherClientID, 2) // same version
	assert.Nil(t, err, "AddEdgeWithVersion with same version different client should not error")

	if otherClientID < clientID {
		assert.Equal(t, 3, len(parent.Edges), "Tie-breaker: new client wins")
	} else {
		assert.Equal(t, 2, len(parent.Edges), "Tie-breaker: original client keeps ownership")
	}
}

func TestTreeCRDTRemoveEdgeWithVersion(t *testing.T) {
	c := NewTreeCRDT()

	clientID := core.ClientID("bbbb")
	otherClientID := core.ClientID("aaaa")

	parent := c.CreateAttachedNode("parent", Map, c.Root.ID, clientID)
	child := c.CreateAttachedNode("child", Map, c.Root.ID, clientID)

	// Add an edge
	err := c.addEdgeWithVersion(parent.ID, child.ID, "link", clientID, 1)
	assert.Nil(t, err, "addEdgeWithVersion should not return error")

	assert.Equal(t, 1, len(parent.Edges), "Expected 1 edge before removal")

	// Remove the edge with higher version (should succeed)
	err = c.removeEdgeWithVersion(parent.ID, child.ID, clientID, 2, false)
	assert.Nil(t, err, "removeEdgeWithVersion should not return error")
	assert.Equal(t, 0, len(parent.Edges), "Expected 0 edges after removal")

	// Re-add it for conflict test
	_ = c.addEdgeWithVersion(parent.ID, child.ID, "link", clientID, 3)

	// Try to remove with lower version (should be ignored)
	err = c.removeEdgeWithVersion(parent.ID, child.ID, clientID, 2, false)
	assert.NotNil(t, err, "removeEdgeWithVersion with lower version should error")
	assert.Equal(t, 1, len(parent.Edges), "Edge should still exist after invalid removal")

	// Tie-break with other client (lower client ID wins)
	err = c.removeEdgeWithVersion(parent.ID, child.ID, otherClientID, 3, false)
	assert.Nil(t, err, "removeEdgeWithVersion tie-break should not error")

	if otherClientID < clientID {
		assert.Equal(t, 0, len(parent.Edges), "Tie-break: other client removed the edge")
	} else {
		assert.Equal(t, 1, len(parent.Edges), "Tie-break: original client kept the edge")
	}
}

func TestTreeCRDTRemoveIndexInArray(t *testing.T) {
	clientID := core.ClientID(random.GenerateRandomID())

	initialJSON := []byte(`["A", "B", "C"]`)

	c := NewTreeCRDT()
	_, err := c.ImportJSON(initialJSON, core.ClientID(clientID))
	assert.Nil(t, err, "AddNodeRecursively should not return an error")

	// Find the node with ID "B"
	arrNodeID := c.Root.Edges[0].To
	arrNode, ok := c.GetNode(arrNodeID)
	assert.True(t, ok, "Array node should exist")
	edges := arrNode.Edges
	for _, edge := range edges {
		node, ok := c.GetNode(edge.To)
		assert.True(t, ok, "Node should exist in the array")
		if node.LiteralValue.(string) == "B" {
			// Remove the edge with ID "B"
			err = c.RemoveEdge(arrNodeID, node.ID, clientID)
			assert.Nil(t, err, "removeEdgeWithVersion should not return an error")
			break
		}
	}

	exportedJSON, err := c.ExportJSON()
	assert.Nil(t, err, "ExportToJSON should not return an error")

	// Correct expected JSON
	expectedJSON := []byte(`[
		"A",
		"C"
	]`)

	utils.CompareJSON(t, expectedJSON, exportedJSON)
}

func TestTreeCRDTTidy(t *testing.T) {
	c := NewTreeCRDT()

	clientID := core.ClientID("client")

	c.CreateAttachedNode("parent", Map, c.Root.ID, clientID)
	c.CreateAttachedNode("child", Map, c.Root.ID, clientID)

	// Create an orphan node manually (NOT attached)
	orphanID := generateRandomNodeID("orphan")
	orphan := c.getOrCreateNode(orphanID, Map, clientID, 1)

	assert.Equal(t, 4, len(c.Nodes), "Expected 4 nodes before purge (root, parent, child, orphan)")

	c.Tidy() // Remove orphan nodes

	// Should only have root, parent, and child left
	_, orphanExists := c.Nodes[orphan.ID]
	assert.False(t, orphanExists, "Orphan should have been purged")
	assert.Equal(t, 3, len(c.Nodes), "Expected 3 nodes after purge (root, parent, child)")
}

func TestTreeCRDTNodeSetLiteral(t *testing.T) {
	c := NewTreeCRDT()

	clientID1 := core.ClientID("client1")
	clientID2 := core.ClientID("client2")

	node := c.CreateAttachedNode("literalNode", Literal, c.Root.ID, clientID1)

	// 1. Set an initial literal value
	node.setLiteralWithVersion("hello", clientID1, 1)

	assert.True(t, node.IsLiteral, "Expected node to be marked as literal")
	assert.Equal(t, "hello", node.LiteralValue, "Expected literal value to be 'hello'")

	// 2. Set a higher version value (should overwrite)
	node.setLiteralWithVersion("world", clientID1, 2)

	assert.Equal(t, "world", node.LiteralValue, "Expected literal value to be updated to 'world'")

	// 3. Attempt to set with a lower version (should be ignored)
	node.setLiteralWithVersion("ignored", clientID1, 1)

	assert.Equal(t, "world", node.LiteralValue, "Lower version should not overwrite the value")

	// 4. Simulate conflict: different client, same version
	node.setLiteralWithVersion("conflict", clientID2, 2)

	// Resolve which client should win
	expectedWinner := clientID1
	if clientID2 < clientID1 {
		expectedWinner = clientID2
	}

	expectedValue := "world"
	if expectedWinner == clientID2 {
		expectedValue = "conflict"
	}

	assert.Equal(t, expectedWinner, node.Owner, fmt.Sprintf("Expected owner %s to win tie-breaker, got %s", expectedWinner, node.Owner))
	assert.Equal(t, expectedValue, node.LiteralValue, fmt.Sprintf("Expected literal value %s after conflict resolution, got %s", expectedValue, node.LiteralValue))
}

func TestTreeCRDTValidation(t *testing.T) {
	client := core.ClientID("clientA")

	t.Run("Valid tree passes validation", func(t *testing.T) {
		c := NewTreeCRDT()
		nodeA := c.CreateAttachedNode("A", Map, c.Root.ID, client)
		nodeB := c.CreateAttachedNode("B", Map, nodeA.ID, client)
		c.CreateAttachedNode("C", Map, nodeB.ID, client)

		err := c.ValidateTree()
		assert.NoError(t, err, "Valid tree structure should pass validation")
	})

	t.Run("Multiple parents detected", func(t *testing.T) {
		c := NewTreeCRDT()
		nodeA := c.CreateAttachedNode("A", Map, c.Root.ID, client)
		nodeB := c.CreateAttachedNode("B", Map, nodeA.ID, client)
		nodeC := c.CreateAttachedNode("C", Map, nodeB.ID, client)

		// Add second parent (invalid) through API
		err := c.AddEdge(nodeA.ID, nodeC.ID, "", client)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "multiple parents")

		// Simulate corruption manually
		c.Nodes[nodeA.ID].Edges = append(c.Nodes[nodeA.ID].Edges, &EdgeCRDT{
			From:         nodeA.ID,
			To:           nodeC.ID,
			Label:        "",
			LSEQPosition: []int{42},
		})

		err = c.ValidateTree()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "multiple parents")
	})

	t.Run("Cycle detection", func(t *testing.T) {
		c := NewTreeCRDT()
		nodeA := c.CreateAttachedNode("A", Map, c.Root.ID, client)
		nodeB := c.CreateAttachedNode("B", Map, nodeA.ID, client)
		nodeC := c.CreateAttachedNode("C", Map, nodeB.ID, client)

		// Create a cycle: C -> A
		c.Nodes[nodeC.ID].Edges = append(c.Nodes[nodeC.ID].Edges, &EdgeCRDT{
			From:         nodeC.ID,
			To:           nodeA.ID,
			Label:        "",
			LSEQPosition: []int{99},
		})

		err := c.validAttachment(nodeC.ID, nodeA.ID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "would create a cycle")

		err = c.ValidateTree()
		assert.Error(t, err)
	})

	t.Run("Literal node with children fails validation", func(t *testing.T) {
		c := NewTreeCRDT()
		lit := c.CreateAttachedNode("Literal", Literal, c.Root.ID, client)
		c.CreateAttachedNode("Child", Map, lit.ID, client)

		err := c.ValidateTree()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "must not have children")
	})

	t.Run("Node with multiple types fails validation", func(t *testing.T) {
		c := NewTreeCRDT()
		node := c.CreateAttachedNode("BadNode", Map, c.Root.ID, client)
		node.IsArray = true // Invalid: now both map and array

		err := c.ValidateTree()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "must have exactly one type")
	})

	t.Run("Unreachable node fails validation", func(t *testing.T) {
		c := NewTreeCRDT()
		_ = c.CreateAttachedNode("A", Map, c.Root.ID, client)

		// Add isolated node
		isolated := &NodeCRDT{
			ID:        core.NodeID("isolated"),
			IsMap:     true,
			IsRoot:    false,
			Owner:     client,
			tree:      c,
			Clock:     vectorclock.VectorClock{},
			Nonce:     "iso",
			Signature: "sig",
		}
		c.Nodes[isolated.ID] = isolated

		err := c.ValidateTree()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "Unreachable node")
	})
}

// // Test case:
// // 1. Create two graphs with shared nodes
// // 2. Set different literal values on the same node in both graphs
// // 3. Merge the graphs
// // 4. The merged graph should be an array of literals since n1 + n2 → [n1, n2] sorted by node ID
func TestTreeCRDTMergeLiterals(t *testing.T) {
	c1 := NewTreeCRDT()
	c2 := NewTreeCRDT()

	clientA := core.ClientID("clientA")
	clientB := core.ClientID("clientB")

	// Create shared nodes in both graphs
	node1 := c1.CreateAttachedNode("sharedA", Literal, c1.Root.ID, clientA)
	node2 := c2.CreateAttachedNode("sharedB", Literal, c2.Root.ID, clientB)
	_, err := node1.SetLiteral("A-literal", clientA)
	assert.Nil(t, err, "SetLiteral should not return an error")
	_, err = node2.SetLiteral("B-literal", clientB)
	assert.Nil(t, err, "SetLiteral should not return an error")

	c1Copy, err := c1.Clone()
	c2Copy, err := c2.Clone()

	// Perform merge
	err = c1.Merge(c2)
	assert.Nil(t, err, "Merge should not return an error")
	err = c2Copy.Merge(c1Copy)
	assert.Nil(t, err, "Merge should not return an error")

	// Check that all nodes exist
	_, ok1 := c1.GetNode(node1.ID)
	_, ok2 := c1.GetNode(node2.ID)
	assert.True(t, ok1, "Node1 should exist after merge")
	assert.True(t, ok2, "Node2 should exist after merge")

	json, err := c1.ExportJSON()
	assert.Nil(t, err, "ExportToJSON should not return an error")

	json2, err := c2Copy.ExportJSON()
	assert.Nil(t, err, "ExportToJSON should not return an error")

	utils.CompareJSON(t, json, json2)

	if node1.ID < node2.ID {
		expectedJSON := []byte(`["A-literal", "B-literal"]`)
		utils.CompareJSON(t, expectedJSON, json)
	} else {
		expectedJSON := []byte(`["B-literal", "A-literal"]`)
		utils.CompareJSON(t, expectedJSON, json)
	}
}

func TestTreeCRDTMergeLists(t *testing.T) {
	clientA := core.ClientID("clientA")
	clientB := core.ClientID("clientB")

	initialJSON := []byte(`[1, 2, 4]`)

	c1 := NewTreeCRDT()
	_, err := c1.ImportJSON(initialJSON, core.ClientID(clientA))
	assert.Nil(t, err, "AddNodeRecursively should not return an error")

	rawJSON, err := c1.Save()
	assert.Nil(t, err, "ExportToRaw should not return an error")

	c2 := NewTreeCRDT()
	c2.Load(rawJSON)
	assert.Nil(t, err, "ImportRawJSON should not return an error")

	rawJSONBefore, err := c1.Save()
	assert.Nil(t, err, "ExportToRaw should not return an error")

	err = c1.Merge(c2)
	assert.Nil(t, err, "Merge should not return an error")

	rawJSONAfter, err := c1.Save()
	assert.Nil(t, err, "ExportToRaw should not return an error")

	// Trees should be identical before and after merge
	assert.Equal(t, rawJSONBefore, rawJSONAfter, "Trees should be identical before and after merge")
	assert.True(t, c1.Equal(c2), "Trees should be equal after merge")

	// Let's do some modifications on the graph independently
	// Original    :    [1, 2, 4]
	// G1(A):        [0, 1, 2, 4]
	// G2(B):           [1, 2, 3, 4]
	// G1 + G2:      [0, 1, 2, 3, 4] <- 4 is added to G1, owner of root is B
	// G2 + G1:      [0, 1, 2, 3, 4] <- 0 is added to G2, owner of root is A

	// 1. Create a new node in c1
	node0 := c1.CreateNode("0", Literal, clientA)
	node0.SetLiteral(0, clientA)

	// First child is the array
	assert.Len(t, c1.Root.Edges, 1, "Root should have one edge")
	c1ArrayNodeID := c1.Root.Edges[0].To

	// Find the node with id "0"
	sibling, err := c1.GetSibling(c1ArrayNodeID, 0)
	assert.Nil(t, err, "GetSiblingNode should not return an error")
	err = c1.InsertEdgeLeft(c1ArrayNodeID, node0.ID, "", sibling.ID, clientA)
	assert.Nil(t, err, "InsertEdge should not return an error")
	// G1: [0, 1, 2, 4]  <-- 0 added

	// 2. Create a new node in c2
	node3 := c2.CreateNode("3", Literal, clientA)
	node3.SetLiteral(3, clientA)
	// node3.IsLiteral = true
	// node3.LiteralValue = 3.0

	// First child is the array
	assert.Len(t, c2.Root.Edges, 1, "Root should have one edge")
	c2ArrayNodeID := c2.Root.Edges[0].To
	sibling, err = c2.GetSibling(c2ArrayNodeID, 1)
	assert.Nil(t, err, "GetSiblingNode should not return an error")
	err = c2.InsertEdgeRight(c2ArrayNodeID, node3.ID, "", sibling.ID, clientB)
	assert.Nil(t, err, "InsertEdge should not return an error")
	//  G2: [1, 2, 3, 4]   <-- 3 added

	c1Clone, err := c1.Clone()
	assert.Nil(t, err, "Clone should not return an error")

	// set debug level to see the merge process
	logrus.SetLevel(logrus.ErrorLevel)

	// 3. Merge the graphs
	err = c1.Merge(c2)
	assert.Nil(t, err, "Merge should not return an error")
	err = c2.Merge(c1Clone)
	assert.Nil(t, err, "Merge should not return an error")

	json, err := c1.ExportJSON()
	assert.Nil(t, err, "ExportToJSON should not return an error")
	expectedJSON := []byte(`[0, 1, 2, 3, 4]`)
	utils.CompareJSON(t, expectedJSON, json)

	json2, err := c2.ExportJSON()
	assert.Nil(t, err, "ExportToJSON should not return an error")
	expectedJSON2 := []byte(`[0, 1, 2, 3, 4]`)
	utils.CompareJSON(t, expectedJSON2, json2)

	// Turn on warning log
	logrus.SetLevel(logrus.WarnLevel)

	// C2 == C1
	assert.True(t, c1.Equal(c2), "Graphs should be equal after merge")
	assert.True(t, c1.Root.Owner == c2.Root.Owner, "Owners should be equal after merge")
}

func TestTreeCRDTMergeListsConflicts(t *testing.T) {
	clientA := core.ClientID("A")
	clientB := core.ClientID("B")

	initialJSON := []byte(`[2, 3, 4]`)

	c1 := NewTreeCRDT()
	_, err := c1.ImportJSON(initialJSON, core.ClientID(clientA))
	assert.Nil(t, err, "AddNodeRecursively should not return an error")

	c2, err := c1.Clone()
	assert.Nil(t, err, "Clone should not return an error")

	// C1 prepares nodes
	node := c1.CreateNode("1", Literal, clientA)
	node.IsLiteral = true
	node.LiteralValue = 1
	err = c1.PrependEdge(c1.Root.ID, node.ID, "", clientA)
	assert.Nil(t, err, "PrependEdge should not return an error")

	node = c1.CreateNode("0", Literal, clientA)
	node.IsLiteral = true
	node.LiteralValue = 0
	err = c1.PrependEdge(c1.Root.ID, node.ID, "", clientA)
	assert.Nil(t, err, "PrependEdge should not return an error")

	// C2 appends nodes
	node = c2.CreateNode("5", Literal, clientB)
	node.IsLiteral = true
	node.LiteralValue = 5
	err = c2.AppendEdge(c2.Root.ID, node.ID, "", clientB)
	assert.Nil(t, err, "AppendEdge should not return an error")

	node = c2.CreateNode("6", Literal, clientB)
	node.IsLiteral = true
	node.LiteralValue = 6
	err = c2.AppendEdge(c2.Root.ID, node.ID, "", clientB)
	assert.Nil(t, err, "AppendEdge should not return an error")

	//logrus.SetLevel(logrus.DebugLevel)

	err = c2.Merge(c1)
	assert.Nil(t, err, "Merge should not return an error")
	_, err = c2.ExportJSON()
	assert.Nil(t, err, "ExportToJSON should not return an error")
}

func TestTreeCRDTMergeKVListsWithConflicts(t *testing.T) {
	clientA := core.ClientID("clientA")
	clientB := core.ClientID("clientB")

	initialJSON := []byte(`[
		{"id": "A", "value": "1"}
	]`)

	c1 := NewTreeCRDT()
	_, err := c1.ImportJSON(initialJSON, core.ClientID(clientA))
	assert.Nil(t, err, "AddNodeRecursively should not return an error")

	c2, err := c1.Clone()
	assert.Nil(t, err, "Clone should not return an error")

	arrNodeID := c1.Root.Edges[0].To
	arrNode, ok := c1.GetNode(arrNodeID)
	assert.True(t, ok, "Array node should exist in c1")
	mapNodeID := arrNode.Edges[0].To
	mapNode, ok := c1.GetNode(mapNodeID)
	assert.True(t, ok, "Map node should exist in c1")

	assert.True(t, ok, "Array node should exist in c1")

	_, _, err = mapNode.SetKeyValue("value", "11", clientA)
	_, _, err = mapNode.SetKeyValue("value", "22", clientA)

	json, err := c1.ExportJSON()
	assert.Nil(t, err, "ExportToJSON should not return an error")

	arrNodeID2 := c2.Root.Edges[0].To
	arrNode2, ok := c2.GetNode(arrNodeID2)
	assert.True(t, ok, "Array node should exist in c2")
	mapNodeID2 := arrNode2.Edges[0].To
	mapNode2, ok := c2.GetNode(mapNodeID2)
	assert.True(t, ok, "Map node should exist in c2")

	// Set a different value in c2
	_, _, err = mapNode2.SetKeyValue("value", "33", clientB) // <- Should we overwriting, according to last writer wins policy
	assert.Nil(t, err, "SetKeyValue should not return an error")

	err = c1.Merge(c2) // Enable conflict resolution with last writer wins
	assert.Nil(t, err, "Merge should not return an error")
	err = c2.Merge(c1)
	assert.Nil(t, err, "Merge should not return an error")
	json, err = c1.ExportJSON()
	assert.Nil(t, err, "ExportToJSON should not return an error")

	json2, err := c2.ExportJSON()
	assert.Nil(t, err, "ExportToJSON should not return an error")

	expectedJSON := []byte(`[
	 	{"id": "A", "value": "22"}
	]`)

	utils.CompareJSON(t, expectedJSON, json)
	utils.CompareJSON(t, expectedJSON, json2)
}

func TestTreeCRDTMergeJSON1(t *testing.T) {
	clientID := core.ClientID(random.GenerateRandomID())

	json1 := []byte(`{
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

	expectedJSON := []byte(`[
	  {
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
	  },
	  {
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
	  }
	]`)

	// The order depends on how Node IDs are generated
	expectedJSONAlt := []byte(`[
	  {
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
	  },
	  {
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
	  }
	]`)

	// Build and merge CRDTs
	c1 := NewTreeCRDT()
	_, err := c1.ImportJSON(json1, clientID)
	assert.NoError(t, err)

	c2 := NewTreeCRDT()
	_, err = c2.ImportJSON(json1, clientID)
	assert.NoError(t, err)

	// Since, the node IDs are generated randomly, it imported json will duplicated in an array
	err = c1.Merge(c2) // Enable conflict resolution with last writer wins
	assert.NoError(t, err, "Merge should not return an error")

	exportedJSON, err := c1.ExportJSON()
	assert.NoError(t, err)

	exportedEqualsExpected := utils.IsJSONEqual(t, exportedJSON, expectedJSON) || utils.IsJSONEqual(t, exportedJSON, expectedJSONAlt)
	assert.True(t, exportedEqualsExpected, "Exported JSON should match expected JSON")

	utils.CompareJSON(t, expectedJSON, exportedJSON)
}

func TestTreeCRDTMergeHelloWorld(t *testing.T) {
	clientA := core.ClientID("clientA")
	clientB := core.ClientID("clientB")

	// Step 1: Start from empty CRDT
	c1 := NewTreeCRDT()
	_, err := c1.ImportJSON([]byte(`[]`), clientA)
	assert.Nil(t, err)

	// Step 2: Insert "Hello" in c1
	charsA := []string{"H", "e", "l", "l", "o"}
	parentNode := c1.Root.Edges[0].To
	var leftID core.NodeID
	for _, ch := range charsA {
		n := c1.CreateNode(ch, Literal, clientA)
		n.SetLiteral(ch, clientA)
		err := c1.InsertEdgeRight(parentNode, n.ID, "", leftID, clientA)
		assert.Nil(t, err)
		leftID = n.ID
	}
	lastAID := leftID

	// Step 3: Clone c1 into c2 (simulate clientB syncing)
	raw, err := c1.Save()
	assert.Nil(t, err)

	c2 := NewTreeCRDT()
	err = c2.Load(raw)
	assert.Nil(t, err)

	// Step 4: Insert " world!" in c2 after last "o"
	charsB := []string{" ", "w", "o", "r", "l", "d", "!"}
	parentNode = c2.Root.Edges[0].To
	leftID = lastAID
	for _, ch := range charsB {
		n := c2.CreateNode(ch, Literal, clientB)
		n.SetLiteral(ch, clientB)
		err := c2.InsertEdgeRight(parentNode, n.ID, "", leftID, clientB)
		assert.Nil(t, err)
		leftID = n.ID
	}

	// Step 5: Merge back both ways
	err = c1.Merge(c2)
	assert.Nil(t, err, "Merge c1 with c2 should not return an error")
	err = c2.Merge(c1)
	assert.Nil(t, err, "Merge c2 with c1 should not return an error")

	// Step 6: Export and verify
	json1, err := c1.ExportJSON()
	assert.Nil(t, err)
	json2, err := c2.ExportJSON()
	assert.Nil(t, err)

	expected := []byte(`["H","e","l","l","o"," ","w","o","r","l","d","!"]`)
	utils.CompareJSON(t, expected, json1)
	utils.CompareJSON(t, expected, json2)

	assert.True(t, c1.Equal(c2), "Graphs should be equal after merge")
	assert.Equal(t, c1.Root.Owner, c2.Root.Owner, "Root owners should match after merge")
}

func TestTreeCRDTSingleTreeTwoClientsHelloWorld(t *testing.T) {
	clientA := core.ClientID("clientA")
	clientB := core.ClientID("clientB")

	// Step 1: Initialize TreeCRDT with an empty array
	tree := NewTreeCRDT()
	_, err := tree.ImportJSON([]byte(`[]`), clientA)
	assert.Nil(t, err, "ImportJSON should not return an error")

	parentNode := tree.Root.Edges[0].To
	var leftID core.NodeID

	// Step 2: Client A inserts "Hello"
	charsA := []string{"H", "e", "l", "l", "o"}
	for _, ch := range charsA {
		n := tree.CreateNode(ch, Literal, clientA)
		n.SetLiteral(ch, clientA)
		err := tree.InsertEdgeRight(parentNode, n.ID, "", leftID, clientA)
		assert.Nil(t, err, "InsertEdgeRight (clientA) should not return an error")
		leftID = n.ID
	}

	// Step 3: Client B inserts " world!"
	charsB := []string{" ", "w", "o", "r", "l", "d", "!"}
	for _, ch := range charsB {
		n := tree.CreateNode(ch, Literal, clientB)
		n.SetLiteral(ch, clientB)
		err := tree.InsertEdgeRight(parentNode, n.ID, "", leftID, clientB)
		assert.Nil(t, err, "InsertEdgeRight (clientB) should not return an error")
		leftID = n.ID
	}

	// Step 4: Export final tree and validate JSON
	json, err := tree.ExportJSON()
	assert.Nil(t, err, "ExportJSON should not return an error")

	expected := []byte(`["H","e","l","l","o"," ","w","o","r","l","d","!"]`)
	utils.CompareJSON(t, expected, json)
}

func TestTreeCRDTSingleTreeInterleavedClientsHelloWorld(t *testing.T) {
	clientA := core.ClientID("clientA")
	clientB := core.ClientID("clientB")

	// Step 1: Initialize shared TreeCRDT with an empty array
	tree := NewTreeCRDT()
	_, err := tree.ImportJSON([]byte(`[]`), clientA)
	assert.Nil(t, err, "ImportJSON should not return an error")

	parentNode := tree.Root.Edges[0].To
	var leftID core.NodeID

	// Step 2: Interleave clients while inserting "Hello world!"
	chars := []string{"H", "e", "l", "l", "o", " ", "w", "o", "r", "l", "d", "!"}
	clients := []core.ClientID{clientA, clientB} // Alternating clients

	for i, ch := range chars {
		client := clients[i%2]
		n := tree.CreateNode(ch, Literal, client)
		n.SetLiteral(ch, client)

		err := tree.InsertEdgeRight(parentNode, n.ID, "", leftID, client)
		assert.Nil(t, err, "InsertEdgeRight (interleaved) should not return an error")
		leftID = n.ID
	}

	// Step 3: Export final document and validate JSON structure
	json, err := tree.ExportJSON()
	assert.Nil(t, err, "ExportJSON should not return an error")

	expected := []byte(`["H","e","l","l","o"," ","w","o","r","l","d","!"]`)
	utils.CompareJSON(t, expected, json)
}

func TestTreeCRDTMarkDeletedArray(t *testing.T) {
	clientID := core.ClientID("clientA")

	initialJSON := []byte(`[2, 3, 4]`)

	c := NewTreeCRDT()
	_, err := c.ImportJSON(initialJSON, clientID)
	assert.Nil(t, err, "AddNodeRecursively should not return an error")

	node3, err := c.GetNodeByPath("/1")
	assert.NoError(t, err, "GetNodeByPath should not return an error")

	// Mark deleted
	err = node3.MarkDeleted(clientID)
	assert.NoError(t, err, "SetDeleted should not return an error")

	// Check if the node is marked as deleted
	assert.True(t, node3.IsDeleted, "Node should be marked as deleted")

	exportedJSON, err := c.ExportJSON()
	assert.NoError(t, err, "ExportJSON should not return an error")

	expectedJSON := []byte(`[2, 4]`)
	utils.CompareJSON(t, expectedJSON, exportedJSON)

	arrayNodeID := c.Root.Edges[0].To
	arrayNode, ok := c.GetNode(arrayNodeID)
	assert.True(t, ok, "Array node should exist in the tree")

	// List number of edges and nodes
	assert.Equal(t, 3, len(arrayNode.Edges), "Deleted node should have no edges")
	assert.Equal(t, 5, len(c.Nodes), "Tree should still have the root node after deletion")

	// Tidy the tree
	c.Tidy()

	assert.Equal(t, 2, len(arrayNode.Edges), "Deleted node should have no edges")
	assert.Equal(t, 4, len(c.Nodes), "Tree should still have the root node after deletion")

}

func TestTreeCRDTMarkDeletedMap(t *testing.T) {
	clientID := core.ClientID("clientA")

	initialJSON := []byte(`{"A": 1, "B": 2, "C": 3}`)

	c := NewTreeCRDT()
	_, err := c.ImportJSON(initialJSON, clientID)
	assert.Nil(t, err, "AddNodeRecursively should not return an error")

	nodeB, err := c.GetNodeByPath("/B")
	assert.NoError(t, err, "GetNodeByPath should not return an error")

	// Mark deleted
	err = nodeB.MarkDeleted(clientID)
	assert.NoError(t, err, "SetDeleted should not return an error")

	// Check if the node is marked as deleted
	assert.True(t, nodeB.IsDeleted, "Node should be marked as deleted")

	exportedJSON, err := c.ExportJSON()
	assert.NoError(t, err, "ExportJSON should not return an error")

	expectedJSON := []byte(`{"A": 1, "C": 3}`)
	utils.CompareJSON(t, expectedJSON, exportedJSON)

	arrayNodeID := c.Root.Edges[0].To
	arrayNode, ok := c.GetNode(arrayNodeID)
	assert.True(t, ok, "Array node should exist in the tree")

	// List number of edges and nodes
	assert.Equal(t, 3, len(arrayNode.Edges), "Deleted node should have no edges")
	assert.Equal(t, 5, len(c.Nodes), "Tree should still have the root node after deletion")

	// Tidy the tree
	c.Tidy()

	assert.Equal(t, 2, len(arrayNode.Edges), "Deleted node should have no edges")
	assert.Equal(t, 4, len(c.Nodes), "Tree should still have the root node after deletion")
}

func TestTreeCRDTMarkDeletedLiteral(t *testing.T) {
	clientID := core.ClientID("clientA")

	initialJSON := []byte(`"A"`)

	c := NewTreeCRDT()
	_, err := c.ImportJSON(initialJSON, clientID)
	assert.Nil(t, err, "AddNodeRecursively should not return an error")

	nodeAID := c.Root.Edges[0].To
	assert.NoError(t, err, "GetNodeByPath should not return an error")
	nodeA, ok := c.GetNode(nodeAID)
	assert.True(t, ok, "Node A should exist in the tree")

	// Mark deleted
	err = nodeA.MarkDeleted(clientID)
	assert.NoError(t, err, "SetDeleted should not return an error")

	// Check if the node is marked as deleted
	assert.True(t, nodeA.IsDeleted, "Node should be marked as deleted")

	exportedJSON, err := c.ExportJSON()
	assert.NoError(t, err, "ExportJSON should not return an error")

	expectedJSON := []byte(`null`)
	utils.CompareJSON(t, expectedJSON, exportedJSON)
}

// TestTreeCRDTNetworkPartitionArrayPromotion tests the behavior of TreeCRDT during a network partition
// where concurrent changes on multiple nodes should trigger array promotion.
//
// Scenario:
// 1. Three clients (A, B, C) start with identical state: root -> sharedNode
// 2. Network partition occurs, splitting the clients
// 3. During partition:
//    - Client A: adds child1 to sharedNode
//    - Client B: adds child2 to sharedNode
//    - Client C: adds child3 to sharedNode
// 4. Network heals and clients converge
// 5. Expected: sharedNode should be promoted to an array containing [child1, child2, child3]
//    (sorted by NodeID for deterministic ordering)
func TestTreeCRDTNetworkPartitionArrayPromotion(t *testing.T) {
	// Initialize three clients
	clientA := core.ClientID("clientA")
	clientB := core.ClientID("clientB")
	clientC := core.ClientID("clientC")

	// Step 1: Create initial shared state
	// All clients start with: root -> sharedNode (literal)
	// This sets up a scenario where root has a single child
	initialJSON := []byte(`"shared-value"`)

	// Client A's CRDT
	crdtA := NewTreeCRDT()
	_, err := crdtA.ImportJSON(initialJSON, clientA)
	assert.NoError(t, err, "Client A: ImportJSON should not return an error")

	// Clone to create Client B's CRDT (simulating initial sync)
	crdtB, err := crdtA.Clone()
	assert.NoError(t, err, "Clone for Client B should not return an error")

	// Clone to create Client C's CRDT (simulating initial sync)
	crdtC, err := crdtA.Clone()
	assert.NoError(t, err, "Clone for Client C should not return an error")

	// Document initial state
	rootNodeA := crdtA.Root
	assert.Equal(t, 1, len(rootNodeA.Edges), "Root should have one edge")
	sharedNodeID := rootNodeA.Edges[0].To
	sharedNode, ok := crdtA.GetNode(sharedNodeID)
	assert.True(t, ok, "Shared node should exist")
	assert.True(t, sharedNode.IsLiteral, "Shared node should be a literal initially")
	t.Logf("Initial state (all clients): root -> sharedNode[%s] (literal: %v)", sharedNodeID, sharedNode.LiteralValue)

	// Step 2: Simulate network partition - each client adds a new child to root
	// This will create a conflict at the root level
	// Client A adds child1 to root
	child1 := crdtA.CreateNode("child1", Literal, clientA)
	child1.SetLiteral("value1", clientA)
	err = crdtA.AddEdge(crdtA.Root.ID, child1.ID, "", clientA)
	assert.NoError(t, err, "Client A: AddEdge should not return an error")

	// Document Client A's local state
	t.Logf("Client A after partition: root -> sharedNode + child1[%s]", child1.ID)

	// Client B adds child2 to root
	child2 := crdtB.CreateNode("child2", Literal, clientB)
	child2.SetLiteral("value2", clientB)
	err = crdtB.AddEdge(crdtB.Root.ID, child2.ID, "", clientB)
	assert.NoError(t, err, "Client B: AddEdge should not return an error")

	// Document Client B's local state
	t.Logf("Client B after partition: root -> sharedNode + child2[%s]", child2.ID)

	// Client C adds child3 to root
	child3 := crdtC.CreateNode("child3", Literal, clientC)
	child3.SetLiteral("value3", clientC)
	err = crdtC.AddEdge(crdtC.Root.ID, child3.ID, "", clientC)
	assert.NoError(t, err, "Client C: AddEdge should not return an error")

	// Document Client C's local state
	t.Logf("Client C after partition: root -> sharedNode + child3[%s]", child3.ID)

	// Verify each client's local state before convergence
	assert.Equal(t, 2, len(crdtA.Root.Edges), "Client A: root should have 2 edges")
	assert.Equal(t, 2, len(crdtB.Root.Edges), "Client B: root should have 2 edges")
	assert.Equal(t, 2, len(crdtC.Root.Edges), "Client C: root should have 2 edges")

	// Step 3: Network heals - simulate convergence through merges
	// First, A and B converge
	t.Log("\nPhase 1: Merging A and B")
	err = crdtA.Merge(crdtB)
	assert.NoError(t, err, "Merge A<-B should not return an error")
	err = crdtB.Merge(crdtA)
	assert.NoError(t, err, "Merge B<-A should not return an error")

	// Check the state after A-B merge
	rootAfterAB := crdtA.Root
	t.Logf("After A-B merge: root has %d edges", len(rootAfterAB.Edges))
	
	// Check if array promotion is starting to happen
	if len(rootAfterAB.Edges) == 1 {
		// Check if it's an array
		possibleArray, _ := crdtA.GetNode(rootAfterAB.Edges[0].To)
		t.Logf("Single child after A-B merge: IsArray=%v, IsPromoted=%v, edges=%d",
			possibleArray.IsArray, possibleArray.IsPromoted, len(possibleArray.Edges))
	}

	// Then C converges with the A-B group
	t.Log("\nPhase 2: Merging C with A-B group")
	err = crdtA.Merge(crdtC)
	assert.NoError(t, err, "Merge A<-C should not return an error")
	err = crdtC.Merge(crdtA)
	assert.NoError(t, err, "Merge C<-A should not return an error")
	err = crdtB.Merge(crdtC)
	assert.NoError(t, err, "Merge B<-C should not return an error")

	// Step 4: Verify final state
	// Root should have been promoted to contain an array
	rootFinal := crdtA.Root
	t.Logf("\nFinal structure: root has %d edges", len(rootFinal.Edges))
	
	if len(rootFinal.Edges) == 1 {
		// Check if the single child is a promoted array
		arrayNodeID := rootFinal.Edges[0].To
		arrayNode, ok := crdtA.GetNode(arrayNodeID)
		assert.True(t, ok, "Array node should exist")
		
		if arrayNode.IsArray && arrayNode.IsPromoted {
			t.Logf("Array promotion successful: root -> array[%s] (IsPromoted=%v)", arrayNodeID, arrayNode.IsPromoted)
			
			// The array should contain all four nodes (original shared + 3 new)
			t.Logf("Array contains %d children", len(arrayNode.Edges))
			
			// Collect child values
			childValues := make([]string, 0)
			for i, edge := range arrayNode.Edges {
				child, ok := crdtA.GetNode(edge.To)
				assert.True(t, ok, "Child node should exist")
				if child.IsLiteral {
					childValues = append(childValues, fmt.Sprintf("%v", child.LiteralValue))
					t.Logf("  [%d] -> %s (value: %v)", i, edge.To, child.LiteralValue)
				}
			}
			
			// Verify expected values are present
			assert.Contains(t, childValues, "shared-value", "Original shared value should be in array")
			assert.Contains(t, childValues, "value1", "value1 should be in the array")
			assert.Contains(t, childValues, "value2", "value2 should be in the array")
			assert.Contains(t, childValues, "value3", "value3 should be in the array")
		}
	} else {
		// Direct children without promotion
		t.Logf("No array promotion: root has %d direct children", len(rootFinal.Edges))
		for _, edge := range rootFinal.Edges {
			child, _ := crdtA.GetNode(edge.To)
			t.Logf("  -> %s (IsLiteral: %v, value: %v)", edge.To, child.IsLiteral, child.LiteralValue)
		}
	}

	// Step 5: Verify convergence - all CRDTs should have identical state
	jsonA, err := crdtA.ExportJSON()
	assert.NoError(t, err, "Export JSON from A should not error")
	jsonB, err := crdtB.ExportJSON()
	assert.NoError(t, err, "Export JSON from B should not error")
	jsonC, err := crdtC.ExportJSON()
	assert.NoError(t, err, "Export JSON from C should not error")

	// All clients should have converged to the same state
	utils.CompareJSON(t, jsonA, jsonB)
	utils.CompareJSON(t, jsonA, jsonC)

	// Verify the trees are equal
	assert.True(t, crdtA.Equal(crdtB), "CRDT A and B should be equal after convergence")
	assert.True(t, crdtA.Equal(crdtC), "CRDT A and C should be equal after convergence")
	assert.True(t, crdtB.Equal(crdtC), "CRDT B and C should be equal after convergence")

	t.Log("\nAll clients have successfully converged to the same state with array promotion")
}

// TestTreeCRDTBasicArrayPromotion tests the basic array promotion behavior
// when a node with one child receives a second child during merge
func TestTreeCRDTBasicArrayPromotion(t *testing.T) {
	clientA := core.ClientID("clientA")
	clientB := core.ClientID("clientB")

	// Step 1: Client A creates initial structure: root node with single literal child
	// Array promotion only triggers for nodes that are NOT already Map or Array
	crdtA := NewTreeCRDT()
	// Directly attach a literal to root (root itself will get the second child)
	child1 := crdtA.CreateAttachedNode("child1", Literal, crdtA.Root.ID, clientA)
	child1.SetLiteral("value1", clientA)

	// Document initial state
	t.Logf("Client A initial: root -> child1[%s]", child1.ID)
	assert.Equal(t, 1, len(crdtA.Root.Edges), "Root should have 1 edge")

	// Step 2: Clone to simulate Client B before divergence
	crdtB, err := crdtA.Clone()
	assert.NoError(t, err, "Clone should not return an error")

	// Step 3: Client B adds a second child to root
	child2 := crdtB.CreateAttachedNode("child2", Literal, crdtB.Root.ID, clientB)
	child2.SetLiteral("value2", clientB)

	// Document Client B's state
	t.Logf("Client B after change: root -> child1[%s], child2[%s]", child1.ID, child2.ID)

	// Before merge, verify states
	assert.Equal(t, 1, len(crdtA.Root.Edges), "Client A: root should have 1 edge before merge")
	assert.Equal(t, 2, len(crdtB.Root.Edges), "Client B: root should have 2 edges before merge")

	// Step 4: Merge B into A - this should trigger array promotion
	t.Log("\nMerging B into A...")
	err = crdtA.Merge(crdtB)
	assert.NoError(t, err, "Merge should not return an error")

	// Step 5: Check the result
	rootAfterMerge := crdtA.Root
	t.Logf("\nAfter merge: root has %d edges, IsRoot=%v, IsArray=%v, IsMap=%v", 
		len(rootAfterMerge.Edges), rootAfterMerge.IsRoot, rootAfterMerge.IsArray, rootAfterMerge.IsMap)

	// Check if array promotion occurred
	if len(rootAfterMerge.Edges) == 1 {
		// Promotion happened - root now points to an array
		arrayNodeID := rootAfterMerge.Edges[0].To
		arrayNode, ok := crdtA.GetNode(arrayNodeID)
		assert.True(t, ok, "Array node should exist")
		assert.True(t, arrayNode.IsArray, "Promoted node should be an array")
		assert.True(t, arrayNode.IsPromoted, "Array should be marked as promoted")
		assert.Equal(t, 2, len(arrayNode.Edges), "Array should have 2 children")

		t.Logf("Array promotion occurred: root -> array[%s] (IsPromoted=%v) -> [child1, child2]", 
			arrayNodeID, arrayNode.IsPromoted)
		
		// Log children of the array
		for i, edge := range arrayNode.Edges {
			child, _ := crdtA.GetNode(edge.To)
			t.Logf("  [%d] -> %s (value: %v)", i, edge.To, child.LiteralValue)
		}
	} else {
		// No promotion - children attached directly
		t.Logf("No array promotion: root has %d direct children", len(rootAfterMerge.Edges))
		for _, edge := range rootAfterMerge.Edges {
			child, _ := crdtA.GetNode(edge.To)
			t.Logf("  -> %s (value: %v)", edge.To, child.LiteralValue)
		}
	}

	// Export JSON to see final structure
	jsonBytes, err := crdtA.ExportJSON()
	assert.NoError(t, err, "ExportJSON should not return an error")
	t.Logf("\nFinal JSON: %s", string(jsonBytes))
}

// TestTreeCRDTArrayPromotionNonMapParent tests array promotion specifically
// for nodes that are neither Map nor Array (the promotion condition)
func TestTreeCRDTArrayPromotionNonMapParent(t *testing.T) {
	clientA := core.ClientID("clientA")
	clientB := core.ClientID("clientB")

	// Create a more complex initial structure to test promotion
	// Structure: {"wrapper": "single-value"}
	initialJSON := []byte(`{"wrapper": "single-value"}`)

	crdtA := NewTreeCRDT()
	_, err := crdtA.ImportJSON(initialJSON, clientA)
	assert.NoError(t, err)

	// Clone for Client B
	crdtB, err := crdtA.Clone()
	assert.NoError(t, err)

	// The structure has root -> map -> literal("single-value")
	// Let's find the map node
	rootNode := crdtA.Root
	assert.Equal(t, 1, len(rootNode.Edges), "Root should have 1 edge")
	mapNodeID := rootNode.Edges[0].To
	mapNodeA, ok := crdtA.GetNode(mapNodeID)
	assert.True(t, ok, "Map node should exist")
	assert.True(t, mapNodeA.IsMap, "First child of root should be a map")
	
	// Find the corresponding node in B
	mapNodeB, ok := crdtB.GetNode(mapNodeID)
	assert.True(t, ok, "Map node should exist in B")
	
	// Add a new key-value to the map in client B
	// This should NOT trigger promotion because the node is already a Map
	_, _, err = mapNodeB.SetKeyValue("newKey", "new-value", clientB)
	assert.NoError(t, err)

	// Merge
	err = crdtA.Merge(crdtB)
	assert.NoError(t, err)

	// Check - no promotion should occur because the node is already a Map
	mapNodeAfterMerge, _ := crdtA.GetNode(mapNodeID)
	assert.Equal(t, 2, len(mapNodeAfterMerge.Edges), "Map should have 2 edges after merge")
	assert.True(t, mapNodeAfterMerge.IsMap, "Node should still be a map")

	jsonBytes, err := crdtA.ExportJSON()
	assert.NoError(t, err)
	t.Logf("Final JSON (no promotion expected): %s", string(jsonBytes))
}

// TestTreeCRDTArrayPromotionWithNestedObjects tests array promotion
// when the conflicting children are complex objects rather than literals
func TestTreeCRDTArrayPromotionWithNestedObjects(t *testing.T) {
	clientA := core.ClientID("clientA")
	clientB := core.ClientID("clientB")

	// Initial structure: {"data": {"item": {"name": "original"}}}
	initialJSON := []byte(`{"data": {"item": {"name": "original"}}}`)

	// Client A's CRDT
	crdtA := NewTreeCRDT()
	_, err := crdtA.ImportJSON(initialJSON, clientA)
	assert.NoError(t, err, "Client A: ImportJSON should not return an error")

	// Clone for Client B
	crdtB, err := crdtA.Clone()
	assert.NoError(t, err, "Clone should not return an error")

	// Find the "data" node in both CRDTs
	dataNodeA, err := crdtA.GetNodeByPath("/data")
	assert.NoError(t, err, "Should find /data in CRDT A")
	dataNodeB, err := crdtB.GetNodeByPath("/data")
	assert.NoError(t, err, "Should find /data in CRDT B")

	// Document initial state
	t.Log("Initial state: data -> item -> {name: 'original'}")
	assert.Equal(t, 1, len(dataNodeA.Edges), "data node should have 1 child initially")

	// Client A adds a new key-value to the "data" map
	// Use SetKeyValue which handles the map properly
	_, _, err = dataNodeA.SetKeyValue("newItemA", "valueA", clientA)
	assert.NoError(t, err)

	// Client B adds a different key-value to the "data" map
	_, _, err = dataNodeB.SetKeyValue("newItemB", "valueB", clientB)
	assert.NoError(t, err)

	// Document states before merge
	t.Log("\nBefore merge:")
	t.Log("Client A: data -> {item: {...}, newItemA: 'valueA'}")
	t.Log("Client B: data -> {item: {...}, newItemB: 'valueB'}")

	// Merge
	err = crdtA.Merge(crdtB)
	assert.NoError(t, err, "Merge A<-B should not return an error")

	// Check the result
	dataNodeAfterMerge, err := crdtA.GetNodeByPath("/data")
	assert.NoError(t, err, "Should find /data after merge")

	t.Logf("\nAfter merge: data node has %d edges", len(dataNodeAfterMerge.Edges))

	// Export and log final structure
	jsonBytes, err := crdtA.ExportJSON()
	assert.NoError(t, err)
	t.Logf("Final JSON: %s", string(jsonBytes))

	// Verify all items are present by parsing the JSON
	var result map[string]interface{}
	err = json.Unmarshal(jsonBytes, &result)
	assert.NoError(t, err)
	dataMap := result["data"].(map[string]interface{})
	assert.Contains(t, dataMap, "item", "Original item should still exist")
	assert.Contains(t, dataMap, "newItemA", "newItemA should exist")
	assert.Contains(t, dataMap, "newItemB", "newItemB should exist")
}

// TestTreeCRDTArrayPromotionThenChildEdit tests what happens when
// children of a promoted array are edited after promotion
func TestTreeCRDTArrayPromotionThenChildEdit(t *testing.T) {
	clientA := core.ClientID("clientA")
	clientB := core.ClientID("clientB")
	clientC := core.ClientID("clientC")

	// Step 1: Set up initial state that will lead to array promotion
	crdtA := NewTreeCRDT()
	parent := crdtA.CreateAttachedNode("parent", Map, crdtA.Root.ID, clientA)
	child1 := crdtA.CreateAttachedNode("child1", Map, parent.ID, clientA)
	_, _, err := child1.SetKeyValue("data", "initial", clientA)
	assert.NoError(t, err)

	// Clone for clients B and C
	crdtB, err := crdtA.Clone()
	assert.NoError(t, err)
	crdtC, err := crdtA.Clone()
	assert.NoError(t, err)

	// Step 2: Client B adds a second child (triggers promotion)
	parentB, _ := crdtB.GetNode(parent.ID)
	child2 := crdtB.CreateAttachedNode("child2", Map, parentB.ID, clientB)
	_, _, err = child2.SetKeyValue("data", "initial", clientB)
	assert.NoError(t, err)

	// Merge to trigger promotion
	err = crdtA.Merge(crdtB)
	assert.NoError(t, err)
	err = crdtB.Merge(crdtA)
	assert.NoError(t, err)

	// Document state after promotion
	parentAfterPromotion, _ := crdtA.GetNode(parent.ID)
	t.Logf("After promotion: parent has %d edges", len(parentAfterPromotion.Edges))

	// Step 3: Client C (who hasn't seen the promotion yet) edits child1
	child1C, ok := crdtC.GetNode(child1.ID)
	assert.True(t, ok, "child1 should exist in CRDT C")
	_, _, err = child1C.SetKeyValue("data", "edited by C", clientC)
	assert.NoError(t, err)

	// Step 4: Merge C's edit into the promoted structure
	t.Log("\nMerging C's edits into promoted structure...")
	err = crdtA.Merge(crdtC)
	assert.NoError(t, err)

	// Step 5: Verify the edit propagated correctly
	child1AfterMerge, ok := crdtA.GetNode(child1.ID)
	assert.True(t, ok, "child1 should still exist after merge")

	valueNode, found, err := child1AfterMerge.GetNodeForKey("data")
	assert.NoError(t, err)
	assert.True(t, found, "data key should exist")
	value, err := valueNode.GetLiteral()
	assert.NoError(t, err)
	assert.Equal(t, "edited by C", value, "Edit from client C should be applied")

	// Export final state
	json, err := crdtA.ExportJSON()
	assert.NoError(t, err)
	t.Logf("\nFinal JSON after child edit: %s", string(json))
}

// TestTreeCRDTMultipleLevelArrayPromotion tests array promotion
// at multiple levels of nesting
func TestTreeCRDTMultipleLevelArrayPromotion(t *testing.T) {
	clientA := core.ClientID("clientA")
	clientB := core.ClientID("clientB")

	// Initial structure: {"level1": {"level2": {"item": "value"}}}
	initialJSON := []byte(`{"level1": {"level2": {"item": "value"}}}`)

	crdtA := NewTreeCRDT()
	_, err := crdtA.ImportJSON(initialJSON, clientA)
	assert.NoError(t, err)

	crdtB, err := crdtA.Clone()
	assert.NoError(t, err)

	// Client A adds a sibling to level2
	level1A, err := crdtA.GetNodeByPath("/level1")
	assert.NoError(t, err)
	newLevel2A := crdtA.CreateAttachedNode("newLevel2A", Map, level1A.ID, clientA)
	_, _, err = newLevel2A.SetKeyValue("data", "from A", clientA)
	assert.NoError(t, err)

	// Client B adds a different sibling to level2
	level1B, err := crdtB.GetNodeByPath("/level1")
	assert.NoError(t, err)
	newLevel2B := crdtB.CreateAttachedNode("newLevel2B", Map, level1B.ID, clientB)
	_, _, err = newLevel2B.SetKeyValue("data", "from B", clientB)
	assert.NoError(t, err)

	// Also, Client B adds something to the original level2
	level2B, err := crdtB.GetNodeByPath("/level1/level2")
	assert.NoError(t, err)
	_, _, err = level2B.SetKeyValue("newItem", "added by B", clientB)
	assert.NoError(t, err)

	// Document states before merge
	t.Log("Before merge:")
	t.Log("Client A: level1 -> {level2: {...}, newLevel2A: {data: 'from A'}}")
	t.Log("Client B: level1 -> {level2: {item: 'value', newItem: 'added by B'}, newLevel2B: {data: 'from B'}}")

	// Merge
	err = crdtA.Merge(crdtB)
	assert.NoError(t, err)

	// Check results
	level1AfterMerge, err := crdtA.GetNodeByPath("/level1")
	assert.NoError(t, err)
	t.Logf("\nAfter merge: level1 has %d children", len(level1AfterMerge.Edges))

	// Verify the nested edit also merged correctly
	level2AfterMerge, err := crdtA.GetNodeByPath("/level1/level2")
	if err == nil {
		_, found, _ := level2AfterMerge.GetNodeForKey("newItem")
		assert.True(t, found, "Nested edit from B should be preserved")
	}

	// Export final structure
	json, err := crdtA.ExportJSON()
	assert.NoError(t, err)
	t.Logf("\nFinal JSON: %s", string(json))
}
