package crdt

import (
	"testing"

	"github.com/eislab-cps/synctree/internal/crypto"
	"github.com/eislab-cps/synctree/pkg/core"
	"github.com/eislab-cps/synctree/pkg/vectorclock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCausalDependencyValidation(t *testing.T) {
	tree := NewTreeCRDT()
	identity, err := crypto.CreateIdendity()
	require.NoError(t, err)
	clientID := core.ClientID(identity.ID())

	// Test case 1: Valid sequence (version 1)
	mut1 := DeltaMutation{
		NodeID:   tree.Root.ID,
		Op:       OPSetLiteral,
		Value:    "value1",
		ClientID: clientID,
		Version:  1,
		Clock:    vectorclock.VectorClock{clientID: 1},
	}

	err = tree.applyDeltaMutation(mut1, false)
	assert.NoError(t, err, "First mutation should succeed")

	// Test case 2: Valid sequence (version 2)
	mut2 := DeltaMutation{
		NodeID:   tree.Root.ID,
		Op:       OPSetLiteral,
		Value:    "value2",
		ClientID: clientID,
		Version:  2,
		Clock:    vectorclock.VectorClock{clientID: 2},
	}

	err = tree.applyDeltaMutation(mut2, false)
	assert.NoError(t, err, "Sequential mutation should succeed")

	// Test case 3: Invalid sequence (gap in versions)
	mut4 := DeltaMutation{
		NodeID:   tree.Root.ID,
		Op:       OPSetLiteral,
		Value:    "value4",
		ClientID: clientID,
		Version:  4,
		Clock:    vectorclock.VectorClock{clientID: 4}, // Missing version 3
	}

	err = tree.applyDeltaMutation(mut4, false)
	assert.Error(t, err, "Mutation with gap should fail")
	assert.Contains(t, err.Error(), "missing causal dependency")
}

func TestNodeDependencyValidation(t *testing.T) {
	tree := NewTreeCRDT()
	identity, err := crypto.CreateIdendity()
	require.NoError(t, err)
	clientID := core.ClientID(identity.ID())

	// Test case 1: Try to add edge to non-existent node
	mut1 := DeltaMutation{
		NodeID:     "new-node-1",
		Op:         OPAddEdge,
		FromNodeID: tree.Root.ID,
		ToNodeID:   "non-existent-node",
		Label:      "testlabel",
		ClientID:   clientID,
		Version:    1,
		Clock:      vectorclock.VectorClock{clientID: 1},
	}

	err = tree.applyDeltaMutation(mut1, false)
	assert.Error(t, err, "Adding edge to non-existent node should fail")
	assert.Contains(t, err.Error(), "to node non-existent-node does not exist")

	// Test case 2: Try to set literal on non-existent node
	mut2 := DeltaMutation{
		NodeID:   "non-existent-node",
		Op:       OPSetLiteral,
		Value:    "test-value",
		ClientID: clientID,
		Version:  1,
		Clock:    vectorclock.VectorClock{clientID: 1},
	}

	err = tree.applyDeltaMutation(mut2, false)
	assert.Error(t, err, "Setting literal on non-existent node should fail")
	assert.Contains(t, err.Error(), "cannot set literal: node non-existent-node does not exist")
}

func TestNodeWillExistValidation(t *testing.T) {
	tree := NewTreeCRDT()
	identity, err := crypto.CreateIdendity()
	require.NoError(t, err)
	clientID := core.ClientID(identity.ID())

	// Create a node creation mutation and add it to mutation log
	createMut := DeltaMutation{
		NodeID:   "future-node",
		Op:       OPCreateNode,
		NodeType: Literal,
		ClientID: clientID,
		Version:  1,
		Clock:    vectorclock.VectorClock{clientID: 1},
	}

	// Add to mutation log directly (simulating it was received but not yet applied)
	if tree.mutationLog != nil {
		tree.mutationLog.AddMutation(createMut)
	}

	// Now try to add an edge to this future node - should succeed
	edgeMut := DeltaMutation{
		NodeID:     tree.Root.ID,
		Op:         OPAddEdge,
		FromNodeID: tree.Root.ID,
		ToNodeID:   "future-node",
		Label:      "testlabel",
		ClientID:   clientID,
		Version:    2,
		Clock:      vectorclock.VectorClock{clientID: 2},
	}

	// The dependency validation should pass because the node will exist
	err = tree.validateCausalDependencies(edgeMut)
	assert.NoError(t, err, "Edge to future node should pass dependency validation")
}

func TestMultiClientDependencyValidation(t *testing.T) {
	tree := NewTreeCRDT()
	
	// Create two different client identities
	identity1, err := crypto.CreateIdendity()
	require.NoError(t, err)
	client1 := core.ClientID(identity1.ID())
	
	identity2, err := crypto.CreateIdendity()
	require.NoError(t, err)
	client2 := core.ClientID(identity2.ID())

	// Apply mutation from client1
	mut1 := DeltaMutation{
		NodeID:   tree.Root.ID,
		Op:       OPSetLiteral,
		Value:    "client1-value1",
		ClientID: client1,
		Version:  1,
		Clock:    vectorclock.VectorClock{client1: 1},
	}
	
	err = tree.applyDeltaMutation(mut1, false)
	require.NoError(t, err)

	// Apply mutation from client2
	mut2 := DeltaMutation{
		NodeID:   tree.Root.ID,
		Op:       OPSetLiteral,
		Value:    "client2-value1",
		ClientID: client2,
		Version:  1,
		Clock:    vectorclock.VectorClock{client2: 1},
	}
	
	err = tree.applyDeltaMutation(mut2, false)
	assert.NoError(t, err, "Mutation from different client should succeed")

	// Apply concurrent mutation that references both clients
	concurrentMut := DeltaMutation{
		NodeID:   tree.Root.ID,
		Op:       OPSetLiteral,
		Value:    "concurrent-value",
		ClientID: client1,
		Version:  2,
		Clock:    vectorclock.VectorClock{client1: 2, client2: 1}, // References both clients
	}
	
	err = tree.applyDeltaMutation(concurrentMut, false)
	assert.NoError(t, err, "Concurrent mutation with proper dependencies should succeed")

	// Try mutation with unsatisfied dependency
	invalidMut := DeltaMutation{
		NodeID:   tree.Root.ID,
		Op:       OPSetLiteral,
		Value:    "invalid-value",
		ClientID: client1,
		Version:  3,
		Clock:    vectorclock.VectorClock{client1: 3, client2: 5}, // client2:5 doesn't exist yet
	}
	
	err = tree.applyDeltaMutation(invalidMut, false)
	assert.Error(t, err, "Mutation with unsatisfied dependency should fail")
	assert.Contains(t, err.Error(), "missing causal dependency")
}

func TestDependencyValidationWithSecureMode(t *testing.T) {
	tree := NewTreeCRDT()
	identity, err := crypto.CreateIdendity()
	require.NoError(t, err)
	clientID := core.ClientID(identity.ID())

	// Create and sign a valid mutation
	mut := DeltaMutation{
		NodeID:   tree.Root.ID,
		Op:       OPSetLiteral,
		Value:    "secure-value",
		ClientID: clientID,
		Version:  1,
		Clock:    vectorclock.VectorClock{clientID: 1},
	}

	err = SignDeltaMutation(&mut, identity)
	require.NoError(t, err)

	// Apply in secure mode - should validate both security and dependencies
	err = tree.applyDeltaMutation(mut, true)
	assert.NoError(t, err, "Valid secure mutation should succeed")

	// Create mutation with dependency gap
	invalidMut := DeltaMutation{
		NodeID:   tree.Root.ID,
		Op:       OPSetLiteral,
		Value:    "invalid-secure-value",
		ClientID: clientID,
		Version:  3, // Gap - missing version 2
		Clock:    vectorclock.VectorClock{clientID: 3},
	}

	err = SignDeltaMutation(&invalidMut, identity)
	require.NoError(t, err)

	// Should fail due to dependency validation, even with valid signature
	err = tree.applyDeltaMutation(invalidMut, true)
	assert.Error(t, err, "Mutation with dependency gap should fail even in secure mode")
	assert.Contains(t, err.Error(), "missing causal dependency")
}

func TestEdgeDependencyValidation(t *testing.T) {
	tree := NewTreeCRDT()
	identity, err := crypto.CreateIdendity()
	require.NoError(t, err)
	clientID := core.ClientID(identity.ID())

	// First, create a child node
	childNode := tree.CreateNode("child", Literal, clientID)
	require.NotNil(t, childNode)

	// Test valid edge creation
	validEdgeMut := DeltaMutation{
		NodeID:     "edge-1",
		Op:         OPAddEdge,
		FromNodeID: tree.Root.ID,
		ToNodeID:   childNode.ID,
		Label:      "valid-edge",
		ClientID:   clientID,
		Version:    2,
		Clock:      vectorclock.VectorClock{clientID: 2},
	}

	err = tree.applyDeltaMutation(validEdgeMut, false)
	assert.NoError(t, err, "Valid edge mutation should succeed")

	// Test edge removal 
	removeEdgeMut := DeltaMutation{
		NodeID:     "edge-remove-1",
		Op:         OPRemoveEdge,
		FromNodeID: tree.Root.ID,
		ToNodeID:   childNode.ID,
		ClientID:   clientID,
		Version:    3,
		Clock:      vectorclock.VectorClock{clientID: 3},
	}

	err = tree.applyDeltaMutation(removeEdgeMut, false)
	assert.NoError(t, err, "Valid edge removal should succeed")

	// Test removing edge from non-existent node
	invalidRemoveMut := DeltaMutation{
		NodeID:     "edge-remove-2",
		Op:         OPRemoveEdge,
		FromNodeID: "non-existent-from",
		ToNodeID:   childNode.ID,
		ClientID:   clientID,
		Version:    4,
		Clock:      vectorclock.VectorClock{clientID: 4},
	}

	err = tree.applyDeltaMutation(invalidRemoveMut, false)
	assert.Error(t, err, "Removing edge from non-existent node should fail")
	assert.Contains(t, err.Error(), "cannot remove edge: from node non-existent-from does not exist")
}