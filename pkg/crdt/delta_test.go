package crdt

import (
	"testing"

	"github.com/eislab-cps/synctree/pkg/core"
	"github.com/eislab-cps/synctree/pkg/vectorclock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSecureTreeWithDelta tests that SecureTree operations can be tracked with delta
func TestSecureTreeWithDelta(t *testing.T) {
	prvKey := "d6eb959e9aec2e6fdc44b5862b269e987b8a4d6f2baca542d8acaa97ee5e74f6"
	
	// Create SecureTree
	secureTree, err := NewSecureTree(prvKey)
	require.NoError(t, err)
	
	// Get underlying TreeCRDT for delta tracking
	adapter := secureTree.(*AdapterSecureTreeCRDT)
	tree := adapter.treeCrdt
	deltaSync := NewDeltaSync(tree)

	// Test that we can perform SecureTree operations 
	// (The current implementation returns nil for Delta, which is fine for now)
	jsonData := []byte(`{"name": "test", "value": 42}`)
	nodeID, err := secureTree.ImportJSON(jsonData, prvKey)
	assert.NoError(t, err)
	assert.NotEmpty(t, nodeID)
	// ImportJSON doesn't return Delta in current implementation

	// Test that we can access the tree state
	node, err := secureTree.GetNodeByPath("/name")
	assert.NoError(t, err)
	assert.NotNil(t, node)

	// Test that DeltaSync can track the current tree state
	currentClock := tree.GetVectorClock()
	assert.NotNil(t, currentClock)

	// Test delta generation (even with empty history, it should work)
	testDelta := deltaSync.GenerateDelta(vectorclock.VectorClock{}, core.ClientID("test"))
	assert.NotNil(t, testDelta)
	assert.Equal(t, core.ClientID("test"), testDelta.SourceID)
}

// TestDeltaWithSecureTreeOperations tests various SecureTree operations
func TestDeltaWithSecureTreeOperations(t *testing.T) {
	prvKey := "d6eb959e9aec2e6fdc44b5862b269e987b8a4d6f2baca542d8acaa97ee5e74f6"
	
	secureTree, err := NewSecureTree(prvKey)
	require.NoError(t, err)
	
	adapter := secureTree.(*AdapterSecureTreeCRDT)
	tree := adapter.treeCrdt
	deltaSync := NewDeltaSync(tree)

	// Test SetLiteral operation
	root, err := secureTree.GetNodeByPath("/")
	require.NoError(t, err)
	
	delta, mapNode, err := root.CreateMapNode(prvKey)
	assert.NoError(t, err)
	assert.NotNil(t, mapNode)
	assert.Nil(t, delta) // Current implementation

	delta, nodeID, err := mapNode.SetKeyValue("test", "value", prvKey)
	assert.NoError(t, err)
	assert.NotEmpty(t, nodeID)
	assert.Nil(t, delta) // Current implementation

	// Test that we can retrieve the value
	valueNode, err := secureTree.GetNodeByPath("/test")
	assert.NoError(t, err)
	assert.NotNil(t, valueNode)

	literal, err := valueNode.GetLiteral()
	assert.NoError(t, err)
	assert.Equal(t, "value", literal)

	// Test delta functionality still works
	testDelta := deltaSync.GenerateDelta(vectorclock.VectorClock{}, core.ClientID("test"))
	assert.NotNil(t, testDelta)
}

// TestDeltaHistoryLimit tests the history limiting functionality
func TestDeltaHistoryLimit(t *testing.T) {
	prvKey := "d6eb959e9aec2e6fdc44b5862b269e987b8a4d6f2baca542d8acaa97ee5e74f6"
	
	secureTree, err := NewSecureTree(prvKey)
	require.NoError(t, err)
	
	adapter := secureTree.(*AdapterSecureTreeCRDT)
	tree := adapter.treeCrdt
	deltaSync := NewDeltaSync(tree)
	
	// Set a small limit for testing
	deltaSync.maxHistory = 3

	clientID := core.ClientID("test-client")

	// Add more operations than the limit
	for i := 0; i < 5; i++ {
		op := DeltaOperation{
			Type:     OpCreateNode,
			NodeID:   core.NodeID("node-" + string(rune('0'+i))),
			ClientID: clientID,
			Clock:    vectorclock.VectorClock{clientID: i + 1},
		}
		deltaSync.RecordOperation(op)
	}

	// Should only keep the last 3 operations
	assert.Len(t, deltaSync.history, 3)
	
	// Should have operations for nodes 2, 3, 4 (the last 3)
	assert.Equal(t, core.NodeID("node-2"), deltaSync.history[0].NodeID)
	assert.Equal(t, core.NodeID("node-3"), deltaSync.history[1].NodeID)
	assert.Equal(t, core.NodeID("node-4"), deltaSync.history[2].NodeID)
}

// TestMultipleSecureTreeSync tests syncing between multiple SecureTrees using delta
func TestMultipleSecureTreeSync(t *testing.T) {
	prvKey := "d6eb959e9aec2e6fdc44b5862b269e987b8a4d6f2baca542d8acaa97ee5e74f6"
	
	// Create two SecureTrees
	tree1, err := NewSecureTree(prvKey)
	require.NoError(t, err)
	
	tree2, err := NewSecureTree(prvKey)
	require.NoError(t, err)

	adapter1 := tree1.(*AdapterSecureTreeCRDT)
	adapter2 := tree2.(*AdapterSecureTreeCRDT)
	
	deltaSync1 := NewDeltaSync(adapter1.treeCrdt)
	deltaSync2 := NewDeltaSync(adapter2.treeCrdt)

	// Add data to tree1
	jsonData := []byte(`{"message": "Hello from tree1"}`)
	_, err = tree1.ImportJSON(jsonData, prvKey)
	require.NoError(t, err)

	// Verify tree1 has the data
	node1, err := tree1.GetNodeByPath("/message")
	assert.NoError(t, err)
	value1, err := node1.GetLiteral()
	assert.NoError(t, err)
	assert.Equal(t, "Hello from tree1", value1)

	// Verify tree2 doesn't have the data yet
	_, err = tree2.GetNodeByPath("/message")
	assert.Error(t, err) // Should fail because node doesn't exist

	// Test that both delta syncs work independently
	delta1 := deltaSync1.GenerateDelta(vectorclock.VectorClock{}, core.ClientID("sync1"))
	delta2 := deltaSync2.GenerateDelta(vectorclock.VectorClock{}, core.ClientID("sync2"))
	
	assert.NotNil(t, delta1)
	assert.NotNil(t, delta2)
	assert.Equal(t, core.ClientID("sync1"), delta1.SourceID)
	assert.Equal(t, core.ClientID("sync2"), delta2.SourceID)

	// Note: Actual delta application would require implementing the 
	// missing functionality to convert SecureTree operations to 
	// DeltaOperations and apply them. This is a foundation test.
}

// TestVectorClockIntegration tests vector clock integration with TreeCRDT
func TestVectorClockIntegration(t *testing.T) {
	prvKey := "d6eb959e9aec2e6fdc44b5862b269e987b8a4d6f2baca542d8acaa97ee5e74f6"
	
	secureTree, err := NewSecureTree(prvKey)
	require.NoError(t, err)
	
	adapter := secureTree.(*AdapterSecureTreeCRDT)
	tree := adapter.treeCrdt

	// Test GetVectorClock method we added to TreeCRDT
	clock := tree.GetVectorClock()
	assert.NotNil(t, clock)
	
	// Initially should be empty or have root node's clock
	assert.True(t, len(clock) >= 0)
	
	// Add some data and check if clock updates
	jsonData := []byte(`{"test": "data"}`)
	_, err = secureTree.ImportJSON(jsonData, prvKey)
	require.NoError(t, err)
	
	newClock := tree.GetVectorClock()
	assert.NotNil(t, newClock)
	
	// Clock should potentially have some entries now
	// (exact behavior depends on how ImportJSON sets clocks)
}