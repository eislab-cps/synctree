package crdt

import (
	"testing"

	"github.com/eislab-cps/synctree/pkg/core"
	"github.com/eislab-cps/synctree/pkg/vectorclock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDeltaSerialization tests basic delta serialization
func TestDeltaSerialization(t *testing.T) {
	prvKey := "d6eb959e9aec2e6fdc44b5862b269e987b8a4d6f2baca542d8acaa97ee5e74f6"
	
	_, err := NewSecureTree(prvKey)
	require.NoError(t, err)

	// Create a simple delta 
	delta := Delta{
		Operations: []DeltaOperation{
			{
				Type:     OpSetLiteral,
				NodeID:   "test-node",
				Value:    "test-value",
				ClientID: core.ClientID("test-client"),
			},
		},
		SourceID: core.ClientID("test-client"),
	}

	// Test JSON serialization
	jsonData, err := delta.ToJSON()
	assert.NoError(t, err)
	assert.NotEmpty(t, jsonData)

	// Test deserialization
	delta2 := Delta{}
	err = delta2.FromJSON(jsonData)
	assert.NoError(t, err)
	assert.Len(t, delta2.Operations, 1)
	assert.Equal(t, OpSetLiteral, delta2.Operations[0].Type)
	assert.Equal(t, "test-value", delta2.Operations[0].Value)
}

// TestBasicDeltaSync tests creating and using DeltaSync
func TestBasicDeltaSync(t *testing.T) {
	prvKey := "d6eb959e9aec2e6fdc44b5862b269e987b8a4d6f2baca542d8acaa97ee5e74f6"
	
	secureTree, err := NewSecureTree(prvKey)
	require.NoError(t, err)
	
	// Access the underlying TreeCRDT
	adapter := secureTree.(*AdapterSecureTreeCRDT)
	tree := adapter.treeCrdt
	
	// Create DeltaSync
	deltaSync := NewDeltaSync(tree)
	assert.NotNil(t, deltaSync)
	assert.Equal(t, tree, deltaSync.tree)
	assert.Equal(t, 1000, deltaSync.maxHistory)
}

// TestClockOperations tests vector clock operations
func TestClockOperations(t *testing.T) {
	clientA := core.ClientID("client-a")
	clientB := core.ClientID("client-b")

	// Test clock comparison
	clock1 := vectorclock.VectorClock{clientA: 1}
	clock2 := vectorclock.VectorClock{clientA: 2}
	clock3 := vectorclock.VectorClock{clientA: 1, clientB: 1}

	assert.True(t, clockDominatesOrEqual(clock2, clock1))
	assert.False(t, clockDominatesOrEqual(clock1, clock2))
	assert.True(t, clockDominatesOrEqual(clock3, clock1))

	// Test clock merging
	merged := mergeClock(clock1, clock3)
	expected := vectorclock.VectorClock{clientA: 1, clientB: 1}
	assert.Equal(t, expected, merged)

	merged2 := mergeClock(clock2, clock3)
	expected2 := vectorclock.VectorClock{clientA: 2, clientB: 1}
	assert.Equal(t, expected2, merged2)
}

// TestDeltaOperationRecording tests recording operations
func TestDeltaOperationRecording(t *testing.T) {
	prvKey := "d6eb959e9aec2e6fdc44b5862b269e987b8a4d6f2baca542d8acaa97ee5e74f6"
	
	secureTree, err := NewSecureTree(prvKey)
	require.NoError(t, err)
	
	adapter := secureTree.(*AdapterSecureTreeCRDT)
	tree := adapter.treeCrdt
	deltaSync := NewDeltaSync(tree)

	// Record some operations
	op1 := DeltaOperation{
		Type:     OpCreateNode,
		NodeID:   "node1",
		ClientID: core.ClientID("client1"),
		Clock:    vectorclock.VectorClock{core.ClientID("client1"): 1},
	}
	
	op2 := DeltaOperation{
		Type:     OpSetLiteral,
		NodeID:   "node1",
		Value:    "value1",
		ClientID: core.ClientID("client1"),
		Clock:    vectorclock.VectorClock{core.ClientID("client1"): 2},
	}

	deltaSync.RecordOperation(op1)
	deltaSync.RecordOperation(op2)

	assert.Len(t, deltaSync.history, 2)
	assert.Equal(t, OpCreateNode, deltaSync.history[0].Type)
	assert.Equal(t, OpSetLiteral, deltaSync.history[1].Type)
}

// TestDeltaGeneration tests generating deltas from operation history
func TestDeltaGeneration(t *testing.T) {
	prvKey := "d6eb959e9aec2e6fdc44b5862b269e987b8a4d6f2baca542d8acaa97ee5e74f6"
	
	secureTree, err := NewSecureTree(prvKey)
	require.NoError(t, err)
	
	adapter := secureTree.(*AdapterSecureTreeCRDT)
	tree := adapter.treeCrdt
	deltaSync := NewDeltaSync(tree)

	clientID := core.ClientID("test-client")

	// Record operations
	op1 := DeltaOperation{
		Type:     OpCreateNode,
		NodeID:   "node1",
		ClientID: clientID,
		Clock:    vectorclock.VectorClock{clientID: 1},
	}
	
	op2 := DeltaOperation{
		Type:     OpSetLiteral,
		NodeID:   "node1",
		Value:    "value1",
		ClientID: clientID,
		Clock:    vectorclock.VectorClock{clientID: 2},
	}

	deltaSync.RecordOperation(op1)
	deltaSync.RecordOperation(op2)

	// Generate delta from empty clock (should include all operations)
	delta := deltaSync.GenerateDelta(vectorclock.VectorClock{}, clientID)
	
	assert.NotNil(t, delta)
	assert.Len(t, delta.Operations, 2)
	assert.Equal(t, clientID, delta.SourceID)
	assert.Equal(t, OpCreateNode, delta.Operations[0].Type)
	assert.Equal(t, OpSetLiteral, delta.Operations[1].Type)

	// Generate delta from partial clock (should include only newer operations)
	fromClock := vectorclock.VectorClock{clientID: 1}
	delta2 := deltaSync.GenerateDelta(fromClock, clientID)
	
	assert.NotNil(t, delta2)
	assert.Len(t, delta2.Operations, 1)
	assert.Equal(t, OpSetLiteral, delta2.Operations[0].Type)
}