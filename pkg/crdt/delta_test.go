package crdt

import (
	"fmt"
	"testing"

	"github.com/eislab-cps/synctree/pkg/core"
	"github.com/eislab-cps/synctree/pkg/vectorclock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Basic delta functionality tests

func TestDeltaSerialization(t *testing.T) {
	delta := &Delta{
		Operations: []DeltaOperation{
			{
				Type:     OPCreateNode,
				NodeID:   core.NodeID("test-node"),
				ClientID: core.ClientID("client-1"),
				Clock:    vectorclock.VectorClock{core.ClientID("client-1"): 1},
			},
		},
		SourceID: core.ClientID("client-1"),
	}

	// Test that delta can be created and accessed
	assert.NotNil(t, delta)
	assert.Len(t, delta.Operations, 1)
	assert.Equal(t, OPCreateNode, delta.Operations[0].Type)
	assert.Equal(t, core.ClientID("client-1"), delta.SourceID)
}

func TestDeltaOperationRecording(t *testing.T) {
	tree := NewTreeCRDT()
	deltaSync := NewDeltaSync(tree)

	// Initially no operations recorded
	assert.Len(t, deltaSync.history, 0)

	// Record an operation
	op := DeltaOperation{
		Type:     OPCreateNode,
		NodeID:   core.NodeID("test-node"),
		ClientID: core.ClientID("client-1"),
		Clock:    vectorclock.VectorClock{core.ClientID("client-1"): 1},
	}

	deltaSync.RecordOperation(op)

	// Should have recorded the operation
	assert.Len(t, deltaSync.history, 1)
	assert.Equal(t, OPCreateNode, deltaSync.history[0].Type)
	assert.Equal(t, core.NodeID("test-node"), deltaSync.history[0].NodeID)
}

func TestDeltaGeneration(t *testing.T) {
	tree := NewTreeCRDT()
	deltaSync := NewDeltaSync(tree)

	// Add some operations to history
	ops := []DeltaOperation{
		{
			Type:     OPCreateNode,
			NodeID:   core.NodeID("node-1"),
			ClientID: core.ClientID("client-1"),
			Clock:    vectorclock.VectorClock{core.ClientID("client-1"): 1},
		},
		{
			Type:     OPCreateNode,
			NodeID:   core.NodeID("node-2"),
			ClientID: core.ClientID("client-1"),
			Clock:    vectorclock.VectorClock{core.ClientID("client-1"): 2},
		},
	}

	for _, op := range ops {
		deltaSync.RecordOperation(op)
	}

	// Generate delta from empty clock
	clientID := core.ClientID("sync-client")
	delta := deltaSync.GenerateDelta(vectorclock.VectorClock{}, clientID)

	assert.NotNil(t, delta)
	assert.Equal(t, clientID, delta.SourceID)
	assert.Len(t, delta.Operations, 2)
	assert.Equal(t, OPCreateNode, delta.Operations[0].Type)
	assert.Equal(t, OPCreateNode, delta.Operations[1].Type)
}

func TestBasicDeltaSync(t *testing.T) {
	tree := NewTreeCRDT()
	deltaSync := NewDeltaSync(tree)

	// Test that deltaSync is properly initialized
	assert.NotNil(t, deltaSync)
	assert.Equal(t, tree, deltaSync.tree)
	assert.NotNil(t, deltaSync.history)
	assert.Equal(t, 1000, deltaSync.maxHistory) // Default value
}

// Delta application tests

func TestApplyDeltaModifiesTree(t *testing.T) {
	tree := NewTreeCRDT()
	deltaSync := NewDeltaSync(tree)

	// Create delta with node creation operation
	delta := &Delta{
		Operations: []DeltaOperation{
			{
				Type:     OPCreateNode,
				NodeID:   core.NodeID("new-node"),
				ParentID: core.NodeID("root"),
				ClientID: core.ClientID("client-1"),
				Clock:    vectorclock.VectorClock{core.ClientID("client-1"): 1},
				Metadata: map[string]interface{}{
					"is_map": true,
				},
			},
		},
		SourceID: core.ClientID("client-1"),
	}

	// Apply delta
	err := deltaSync.ApplyDelta(delta)
	assert.NoError(t, err)

	// Verify node was created
	node, exists := tree.Nodes[core.NodeID("new-node")]
	assert.True(t, exists, "Node should exist after applying delta")
	assert.NotNil(t, node)
	assert.Equal(t, core.NodeID("new-node"), node.ID)
	assert.Equal(t, tree.Root.ID, node.ParentID)
	assert.True(t, node.IsMap)
}

func TestApplyDeltaWithConflicts(t *testing.T) {
	tree := NewTreeCRDT()
	clientID := core.ClientID("client-1")

	// Add a literal node
	tree.Nodes["literal-node"] = &NodeCRDT{
		tree:         tree,
		ID:           "literal-node",
		ParentID:     tree.Root.ID,
		IsLiteral:    true,
		LiteralValue: "initial-value",
		Clock:        vectorclock.VectorClock{clientID: 2},
		Owner:        clientID,
	}

	deltaSync := NewDeltaSync(tree)

	// Create delta with conflicting literal set operations
	delta := &Delta{
		Operations: []DeltaOperation{
			{
				Type:     OPSetLiteral,
				NodeID:   core.NodeID("literal-node"),
				Value:    "new-value",
				ClientID: core.ClientID("client-2"),
				Clock:    vectorclock.VectorClock{core.ClientID("client-2"): 1}, // Concurrent with existing
			},
		},
		SourceID: core.ClientID("client-2"),
	}

	// Apply delta
	err := deltaSync.ApplyDelta(delta)
	assert.NoError(t, err)

	// Value should NOT change since existing node has higher version (2 > 1)
	node := tree.Nodes["literal-node"] 
	assert.Equal(t, "initial-value", node.LiteralValue, "Existing node should win due to higher version")
}

func TestApplyDeltaPreservesConsistency(t *testing.T) {
	tree := NewTreeCRDT()
	clientID := core.ClientID("client-1")
	tree.Root.Owner = clientID
	tree.Root.Clock = vectorclock.VectorClock{clientID: 1}

	deltaSync := NewDeltaSync(tree)

	// Create delta with multiple related operations
	delta := &Delta{
		Operations: []DeltaOperation{
			{
				Type:     OPCreateNode,
				NodeID:   core.NodeID("child-node"),
				ParentID: core.NodeID("root"),
				ClientID: clientID,
				Clock:    vectorclock.VectorClock{clientID: 2},
				Metadata: map[string]interface{}{
					"is_literal": true,
				},
			},
			{
				Type:     OPSetLiteral,
				NodeID:   core.NodeID("child-node"),
				Value:    "child-value",
				ClientID: clientID,
				Clock:    vectorclock.VectorClock{clientID: 3},
			},
			{
				Type:     OPAddEdge,
				ClientID: clientID,
				Clock:    vectorclock.VectorClock{clientID: 4},
				EdgeInfo: &EdgeInfo{
					FromNodeID: tree.Root.ID,
					ToNodeID:   core.NodeID("child-node"),
					Label:      "child",
				},
			},
		},
		SourceID: clientID,
	}

	// Apply delta
	err := deltaSync.ApplyDelta(delta)
	assert.NoError(t, err)

	// Verify consistency
	childNode, exists := tree.Nodes[core.NodeID("child-node")]
	assert.True(t, exists)
	assert.True(t, childNode.IsLiteral)
	assert.Equal(t, "child-value", childNode.LiteralValue)
	assert.Equal(t, tree.Root.ID, childNode.ParentID)

	// Verify edge was added
	assert.Len(t, tree.Root.Edges, 1)
	assert.Equal(t, "child", tree.Root.Edges[0].Label)
	assert.Equal(t, core.NodeID("child-node"), tree.Root.Edges[0].To)
}

// Individual apply method tests

func TestApplyCreateNodeOperation(t *testing.T) {
	tree := NewTreeCRDT()
	deltaSync := NewDeltaSync(tree)

	op := DeltaOperation{
		Type:     OPCreateNode,
		NodeID:   core.NodeID("test-node"),
		ParentID: core.NodeID("root"),
		ClientID: core.ClientID("client-1"),
		Clock:    vectorclock.VectorClock{core.ClientID("client-1"): 1},
		Metadata: map[string]interface{}{
			"is_map":     true,
			"is_array":   false,
			"is_literal": false,
		},
	}

	err := deltaSync.applyCreateNode(op)
	assert.NoError(t, err)

	// Verify node was created with correct properties
	node, exists := tree.Nodes[core.NodeID("test-node")]
	assert.True(t, exists)
	assert.Equal(t, core.NodeID("test-node"), node.ID)
	assert.Equal(t, tree.Root.ID, node.ParentID)
	assert.True(t, node.IsMap)
	assert.False(t, node.IsArray)
	assert.False(t, node.IsLiteral)
	assert.Equal(t, core.ClientID("client-1"), node.Owner)
	assert.Equal(t, 1, node.Clock[core.ClientID("client-1")])
}

func TestApplySetLiteralOperation(t *testing.T) {
	tree := NewTreeCRDT()
	deltaSync := NewDeltaSync(tree)

	// Create a node first
	nodeID := core.NodeID("literal-node")
	tree.Nodes[nodeID] = &NodeCRDT{
		tree:     tree,
		ID:       nodeID,
		ParentID: tree.Root.ID,
		Clock:    vectorclock.VectorClock{core.ClientID("client-1"): 1},
		Owner:    core.ClientID("client-1"),
		Edges:    make([]*EdgeCRDT, 0),
	}

	op := DeltaOperation{
		Type:      OPSetLiteral,
		NodeID:    nodeID,
		Value:     "test-literal-value",
		ClientID:  core.ClientID("client-2"),
		Clock:     vectorclock.VectorClock{core.ClientID("client-2"): 2},
	}

	err := deltaSync.applySetLiteral(op)
	assert.NoError(t, err)

	// Verify literal value was set
	node := tree.Nodes[nodeID]
	assert.True(t, node.IsLiteral)
	assert.Equal(t, "test-literal-value", node.LiteralValue)
	assert.Equal(t, core.ClientID("client-2"), node.Owner)
	assert.Equal(t, 2, node.Clock[core.ClientID("client-2")])
}

func TestApplyAddEdgeOperation(t *testing.T) {
	tree := NewTreeCRDT()
	deltaSync := NewDeltaSync(tree)

	// Create nodes to connect
	fromID := core.NodeID("from-node")
	toID := core.NodeID("to-node")

	tree.Nodes[fromID] = &NodeCRDT{
		tree:     tree,
		ID:       fromID,
		ParentID: tree.Root.ID,
		IsMap:    true,
		Clock:    vectorclock.VectorClock{core.ClientID("client-1"): 1},
		Owner:    core.ClientID("client-1"),
		Edges:    make([]*EdgeCRDT, 0),
	}

	tree.Nodes[toID] = &NodeCRDT{
		tree:      tree,
		ID:        toID,
		ParentID:  fromID,
		IsLiteral: true,
		Clock:     vectorclock.VectorClock{core.ClientID("client-1"): 1},
		Owner:     core.ClientID("client-1"),
		Edges:     make([]*EdgeCRDT, 0),
	}

	op := DeltaOperation{
		Type:     OPAddEdge,
		ClientID: core.ClientID("client-2"),
		Clock:    vectorclock.VectorClock{core.ClientID("client-2"): 1},
		EdgeInfo: &EdgeInfo{
			FromNodeID: fromID,
			ToNodeID:   toID,
			Label:      "test-edge",
		},
	}

	err := deltaSync.applyAddEdge(op)
	assert.NoError(t, err)

	// Verify edge was added
	fromNode := tree.Nodes[fromID]
	assert.Len(t, fromNode.Edges, 1)
	assert.Equal(t, fromID, fromNode.Edges[0].From)
	assert.Equal(t, toID, fromNode.Edges[0].To)
	assert.Equal(t, "test-edge", fromNode.Edges[0].Label)
}

func TestApplyRemoveEdgeOperation(t *testing.T) {
	tree := NewTreeCRDT()
	deltaSync := NewDeltaSync(tree)

	// Setup tree with edges
	tree.Root.Edges = []*EdgeCRDT{
		{From: tree.Root.ID, To: "child-1", Label: "first"},
		{From: tree.Root.ID, To: "child-2", Label: "second"},
		{From: tree.Root.ID, To: "child-3", Label: "third"},
	}

	// Create child nodes
	for i := 1; i <= 3; i++ {
		nodeID := core.NodeID(fmt.Sprintf("child-%d", i))
		tree.Nodes[nodeID] = &NodeCRDT{
			tree:     tree,
			ID:       nodeID,
			ParentID: tree.Root.ID,
			IsMap:    true,
			Clock:    vectorclock.VectorClock{core.ClientID("client-1"): i},
			Owner:    core.ClientID("client-1"),
			Edges:    make([]*EdgeCRDT, 0),
		}
	}

	op := DeltaOperation{
		Type:     OPRemoveEdge,
		ClientID: core.ClientID("client-2"),
		Clock:    vectorclock.VectorClock{core.ClientID("client-2"): 1},
		EdgeInfo: &EdgeInfo{
			FromNodeID: tree.Root.ID,
			ToNodeID:   core.NodeID("child-2"),
		},
	}

	err := deltaSync.applyRemoveEdge(op)
	assert.NoError(t, err)

	// Verify edge was removed
	assert.Len(t, tree.Root.Edges, 2)
	
	// Check remaining edges
	toNodes := []core.NodeID{}
	for _, edge := range tree.Root.Edges {
		toNodes = append(toNodes, edge.To)
	}
	assert.Contains(t, toNodes, core.NodeID("child-1"))
	assert.Contains(t, toNodes, core.NodeID("child-3"))
	assert.NotContains(t, toNodes, core.NodeID("child-2"))
}

func TestApplyDeleteNodeOperation(t *testing.T) {
	tree := NewTreeCRDT()
	deltaSync := NewDeltaSync(tree)

	// Create a node to delete
	tree.Nodes["test-node"] = &NodeCRDT{
		tree:      tree,
		ID:        "test-node",
		ParentID:  tree.Root.ID,
		IsMap:     true,
		IsDeleted: false,
		Clock:     vectorclock.VectorClock{core.ClientID("client-1"): 1},
		Owner:     core.ClientID("client-1"),
		Edges:     make([]*EdgeCRDT, 0),
	}

	op := DeltaOperation{
		Type:      OPDeleteNode,
		NodeID:    core.NodeID("test-node"),
		ClientID:  core.ClientID("client-2"),
		Clock:     vectorclock.VectorClock{core.ClientID("client-2"): 1},
	}

	err := deltaSync.applyDeleteNode(op)
	assert.NoError(t, err)

	// Verify node is marked as deleted
	node := tree.Nodes["test-node"]
	assert.True(t, node.IsDeleted)
	
	// Clock should be merged  
	assert.Equal(t, 1, node.Clock[core.ClientID("client-1")])
	assert.Equal(t, 1, node.Clock[core.ClientID("client-2")])
}

func TestApplyUpdateNodeOperation(t *testing.T) {
	tree := NewTreeCRDT()
	deltaSync := NewDeltaSync(tree)

	// Create a node to update
	nodeID := core.NodeID("test-node")
	tree.Nodes[nodeID] = &NodeCRDT{
		tree:     tree,
		ID:       nodeID,
		ParentID: tree.Root.ID,
		IsMap:    true,
		Clock:    vectorclock.VectorClock{core.ClientID("client-1"): 1},
		Owner:    core.ClientID("client-1"),
		Edges:    make([]*EdgeCRDT, 0),
	}

	op := DeltaOperation{
		Type:     OPUpdateNode,
		NodeID:   nodeID,
		ClientID: core.ClientID("client-2"),
		Clock:    vectorclock.VectorClock{core.ClientID("client-2"): 1},
		Metadata: map[string]interface{}{
			"test": "data",
		},
	}

	err := deltaSync.applyUpdateNode(op)
	assert.NoError(t, err)

	// Verify clock was merged
	node := tree.Nodes[nodeID]
	assert.Equal(t, 1, node.Clock[core.ClientID("client-1")])
	assert.Equal(t, 1, node.Clock[core.ClientID("client-2")])
}

func TestApplyUpdateClockOperation(t *testing.T) {
	tree := NewTreeCRDT()
	deltaSync := NewDeltaSync(tree)

	op := DeltaOperation{
		Type:     OPUpdateClock,
		NodeID:   tree.Root.ID,
		ClientID: core.ClientID("client-2"),
		Clock:    vectorclock.VectorClock{core.ClientID("client-1"): 5, core.ClientID("client-2"): 3},
	}

	err := deltaSync.applyUpdateClock(op)
	assert.NoError(t, err)

	// Verify clock was merged
	assert.Equal(t, 5, tree.Root.Clock[core.ClientID("client-1")])
	assert.Equal(t, 3, tree.Root.Clock[core.ClientID("client-2")])
}

// applyOperation switch case coverage tests

func TestApplyOperationAllSwitchCases(t *testing.T) {
	tree := NewTreeCRDT()
	deltaSync := NewDeltaSync(tree)

	// Test Case 1: OPCreateNode
	t.Run("OPCreateNode", func(t *testing.T) {
		op := DeltaOperation{
			Type:     OPCreateNode,
			NodeID:   core.NodeID("test-create-node"),
			ClientID: core.ClientID("client-1"),
			Clock:    vectorclock.VectorClock{core.ClientID("client-1"): 1},
			Metadata: map[string]interface{}{
				"is_map": true,
			},
			}

		err := deltaSync.applyOperation(op)
		assert.NoError(t, err)

		// Verify node was created
		_, exists := tree.Nodes[core.NodeID("test-create-node")]
		assert.True(t, exists, "Node should be created via OPCreateNode")
	})

	// Test Case 2: OPUpdateNode
	t.Run("OPUpdateNode", func(t *testing.T) {
		// First create a node to update
		nodeID := core.NodeID("test-update-node")
		tree.Nodes[nodeID] = &NodeCRDT{
			tree:     tree,
			ID:       nodeID,
			ParentID: tree.Root.ID,
			IsMap:    true,
			Clock:    vectorclock.VectorClock{core.ClientID("client-1"): 1},
			Owner:    core.ClientID("client-1"),
			Edges:    make([]*EdgeCRDT, 0),
		}

		op := DeltaOperation{
			Type:     OPUpdateNode,
			NodeID:   nodeID,
			ClientID: core.ClientID("client-2"),
			Clock:    vectorclock.VectorClock{core.ClientID("client-2"): 1},
			Metadata: map[string]interface{}{
				"updated": "metadata",
			},
			}

		err := deltaSync.applyOperation(op)
		assert.NoError(t, err)

		// Verify node was updated (clock should be merged)
		node := tree.Nodes[nodeID]
		assert.Equal(t, 1, node.Clock[core.ClientID("client-2")])
	})

	// Test Case 3: OPDeleteNode
	t.Run("OPDeleteNode", func(t *testing.T) {
		// First create a node to delete
		nodeID := core.NodeID("test-delete-node")
		tree.Nodes[nodeID] = &NodeCRDT{
			tree:      tree,
			ID:        nodeID,
			ParentID:  tree.Root.ID,
			IsMap:     true,
			IsDeleted: false,
			Clock:     vectorclock.VectorClock{core.ClientID("client-1"): 1},
			Owner:     core.ClientID("client-1"),
			Edges:     make([]*EdgeCRDT, 0),
		}

		op := DeltaOperation{
			Type:      OPDeleteNode,
			NodeID:    nodeID,
			ClientID:  core.ClientID("client-2"),
			Clock:     vectorclock.VectorClock{core.ClientID("client-2"): 1},
			}

		err := deltaSync.applyOperation(op)
		assert.NoError(t, err)

		// Verify node was marked as deleted
		node := tree.Nodes[nodeID]
		assert.True(t, node.IsDeleted, "Node should be marked as deleted via OPDeleteNode")
	})

	// Test Case 4: OPAddEdge
	t.Run("OPAddEdge", func(t *testing.T) {
		// Create nodes to connect
		fromNodeID := core.NodeID("test-from-node")
		toNodeID := core.NodeID("test-to-node")

		tree.Nodes[fromNodeID] = &NodeCRDT{
			tree:     tree,
			ID:       fromNodeID,
			ParentID: tree.Root.ID,
			IsMap:    true,
			Clock:    vectorclock.VectorClock{core.ClientID("client-1"): 1},
			Owner:    core.ClientID("client-1"),
			Edges:    make([]*EdgeCRDT, 0),
		}

		tree.Nodes[toNodeID] = &NodeCRDT{
			tree:         tree,
			ID:           toNodeID,
			ParentID:     fromNodeID,
			IsLiteral:    true,
			LiteralValue: "test-value",
			Clock:        vectorclock.VectorClock{core.ClientID("client-1"): 1},
			Owner:        core.ClientID("client-1"),
			Edges:        make([]*EdgeCRDT, 0),
		}

		op := DeltaOperation{
			Type:     OPAddEdge,
			ClientID: core.ClientID("client-2"),
			Clock:    vectorclock.VectorClock{core.ClientID("client-2"): 1},
			EdgeInfo: &EdgeInfo{
				FromNodeID: fromNodeID,
				ToNodeID:   toNodeID,
				Label:      "test-edge",
			},
			}

		err := deltaSync.applyOperation(op)
		assert.NoError(t, err)

		// Verify edge was added
		fromNode := tree.Nodes[fromNodeID]
		assert.Len(t, fromNode.Edges, 1, "Edge should be added via OPAddEdge")
		assert.Equal(t, "test-edge", fromNode.Edges[0].Label)
	})

	// Test Case 5: OPRemoveEdge
	t.Run("OPRemoveEdge", func(t *testing.T) {
		// Create nodes with an existing edge to remove
		parentNodeID := core.NodeID("test-parent-remove")
		childNodeID := core.NodeID("test-child-remove")

		tree.Nodes[parentNodeID] = &NodeCRDT{
			tree:     tree,
			ID:       parentNodeID,
			ParentID: tree.Root.ID,
			IsMap:    true,
			Clock:    vectorclock.VectorClock{core.ClientID("client-1"): 1},
			Owner:    core.ClientID("client-1"),
			Edges: []*EdgeCRDT{
				{From: parentNodeID, To: childNodeID, Label: "edge-to-remove"},
				{From: parentNodeID, To: "other-child", Label: "keep-this-edge"},
			},
		}

		tree.Nodes[childNodeID] = &NodeCRDT{
			tree:         tree,
			ID:           childNodeID,
			ParentID:     parentNodeID,
			IsLiteral:    true,
			LiteralValue: "child-value",
			Clock:        vectorclock.VectorClock{core.ClientID("client-1"): 1},
			Owner:        core.ClientID("client-1"),
			Edges:        make([]*EdgeCRDT, 0),
		}

		op := DeltaOperation{
			Type:     OPRemoveEdge,
			ClientID: core.ClientID("client-2"),
			Clock:    vectorclock.VectorClock{core.ClientID("client-2"): 1},
			EdgeInfo: &EdgeInfo{
				FromNodeID: parentNodeID,
				ToNodeID:   childNodeID,
				Label:      "edge-to-remove",
			},
			}

		err := deltaSync.applyOperation(op)
		assert.NoError(t, err)

		// Verify edge was removed
		parentNode := tree.Nodes[parentNodeID]
		assert.Len(t, parentNode.Edges, 1, "One edge should be removed via OPRemoveEdge")
		assert.Equal(t, "keep-this-edge", parentNode.Edges[0].Label, "Correct edge should remain")
	})

	// Test Case 6: OPSetLiteral
	t.Run("OPSetLiteral", func(t *testing.T) {
		// Create a node to set literal value on
		nodeID := core.NodeID("test-literal-node")
		tree.Nodes[nodeID] = &NodeCRDT{
			tree:         tree,
			ID:           nodeID,
			ParentID:     tree.Root.ID,
			IsLiteral:    true,
			LiteralValue: "old-value",
			Clock:        vectorclock.VectorClock{core.ClientID("client-1"): 1},
			Owner:        core.ClientID("client-1"),
			Edges:        make([]*EdgeCRDT, 0),
		}

		op := DeltaOperation{
			Type:      OPSetLiteral,
			NodeID:    nodeID,
			Value:     "new-literal-value",
			ClientID:  core.ClientID("client-2"),
			Clock:     vectorclock.VectorClock{core.ClientID("client-2"): 2}, // Higher version to win
			}

		err := deltaSync.applyOperation(op)
		assert.NoError(t, err)

		// Verify literal value was set
		node := tree.Nodes[nodeID]
		assert.Equal(t, "new-literal-value", node.LiteralValue, "Literal value should be set via OPSetLiteral")
	})

	// Test Case 7: OPUpdateClock
	t.Run("OPUpdateClock", func(t *testing.T) {
		// Use an existing node to update its clock
		existingNodeID := core.NodeID("test-create-node") // Reuse node from first test

		op := DeltaOperation{
			Type:     OPUpdateClock,
			NodeID:   existingNodeID,
			ClientID: core.ClientID("client-3"),
			Clock:    vectorclock.VectorClock{core.ClientID("client-3"): 5},
		}

		err := deltaSync.applyOperation(op)
		assert.NoError(t, err)

		// Verify clock was updated
		node := tree.Nodes[existingNodeID]
		assert.Equal(t, 5, node.Clock[core.ClientID("client-3")], "Clock should be updated via OPUpdateClock")
	})

	// Test Case 8: Default case (unknown operation type)
	t.Run("UnknownOperationType", func(t *testing.T) {
		op := DeltaOperation{
			Type:      Operation(999), // Invalid operation type
			NodeID:    core.NodeID("some-node"),
			ClientID:  core.ClientID("client-1"),
			Clock:     vectorclock.VectorClock{core.ClientID("client-1"): 1},
			}

		err := deltaSync.applyOperation(op)
		assert.Error(t, err, "Unknown operation type should return an error")
		assert.Contains(t, err.Error(), "unknown operation type", "Error should mention unknown operation type")
		assert.Contains(t, err.Error(), "999", "Error should include the actual unknown type")
	})
}

func TestApplyOperationThroughApplyDelta(t *testing.T) {
	tree := NewTreeCRDT()
	deltaSync := NewDeltaSync(tree)

	// Create a delta with multiple operation types to ensure applyOperation switch is exercised
	delta := &Delta{
		Operations: []DeltaOperation{
			// Test OPCreateNode path through applyOperation
			{
				Type:     OPCreateNode,
				NodeID:   core.NodeID("delta-create-node"),
				ClientID: core.ClientID("client-1"),
				Clock:    vectorclock.VectorClock{core.ClientID("client-1"): 1},
				Metadata: map[string]interface{}{
					"is_literal": true,
				},
			},
			// Test OPSetLiteral path through applyOperation
			{
				Type:      OPSetLiteral,
				NodeID:    core.NodeID("delta-create-node"),
				Value:     "delta-literal-value",
				ClientID:  core.ClientID("client-1"),
				Clock:     vectorclock.VectorClock{core.ClientID("client-1"): 2},
			},
			// Test OPUpdateClock path through applyOperation
			{
				Type:     OPUpdateClock,
				NodeID:   core.NodeID("delta-create-node"),
				ClientID: core.ClientID("client-2"),
				Clock:    vectorclock.VectorClock{core.ClientID("client-2"): 1},
			},
		},
		SourceID: core.ClientID("client-1"),
	}

	// Apply the delta - this should call applyOperation for each operation
	err := deltaSync.ApplyDelta(delta)
	require.NoError(t, err)

	// Verify all operations were applied through applyOperation
	node, exists := tree.Nodes[core.NodeID("delta-create-node")]
	require.True(t, exists, "Node should exist after delta application")
	assert.True(t, node.IsLiteral, "Node should be literal as per metadata")
	assert.Equal(t, "delta-literal-value", node.LiteralValue, "Literal value should be set")
	assert.Equal(t, 1, node.Clock[core.ClientID("client-2")], "Clock should be updated")
}

// Error handling and edge case tests

func TestApplyDeltaToMissingNode(t *testing.T) {
	tree := NewTreeCRDT()
	deltaSync := NewDeltaSync(tree)

	// Create delta with operations on non-existent nodes
	delta := &Delta{
		Operations: []DeltaOperation{
			{
				Type:     OPSetLiteral,
				NodeID:   core.NodeID("non-existent"),
				Value:    "test",
				ClientID: core.ClientID("client-1"),
				Clock:    vectorclock.VectorClock{core.ClientID("client-1"): 1},
			},
			{
				Type:     OPUpdateNode,
				NodeID:   core.NodeID("also-missing"),
				ClientID: core.ClientID("client-1"),
				Clock:    vectorclock.VectorClock{core.ClientID("client-1"): 2},
			},
			{
				Type:     OPUpdateClock,
				NodeID:   core.NodeID("missing-too"),
				ClientID: core.ClientID("client-1"),
				Clock:    vectorclock.VectorClock{core.ClientID("client-1"): 3},
			},
		},
		SourceID: core.ClientID("client-1"),
	}

	err := deltaSync.ApplyDelta(delta)
	assert.Error(t, err, "ApplyDelta should fail when trying to apply operations to missing nodes")
	assert.Contains(t, err.Error(), "failed to apply operation")
}

func TestApplyMalformedDelta(t *testing.T) {
	tree := NewTreeCRDT()
	deltaSync := NewDeltaSync(tree)

	tests := []struct {
		name      string
		operation DeltaOperation
		wantError bool
	}{
		{
			name: "AddEdge without EdgeInfo",
			operation: DeltaOperation{
				Type:     OPAddEdge,
				ClientID: core.ClientID("client-1"),
				Clock:    vectorclock.VectorClock{core.ClientID("client-1"): 1},
				EdgeInfo: nil, // Missing EdgeInfo
			},
			wantError: true,
		},
		{
			name: "RemoveEdge without EdgeInfo",
			operation: DeltaOperation{
				Type:     OPRemoveEdge,
				ClientID: core.ClientID("client-1"),
				Clock:    vectorclock.VectorClock{core.ClientID("client-1"): 1},
				EdgeInfo: nil, // Missing EdgeInfo
			},
			wantError: true,
		},
		{
			name: "AddEdge with non-existent from node",
			operation: DeltaOperation{
				Type:     OPAddEdge,
				ClientID: core.ClientID("client-1"),
				Clock:    vectorclock.VectorClock{core.ClientID("client-1"): 1},
				EdgeInfo: &EdgeInfo{
					FromNodeID: core.NodeID("missing-from"),
					ToNodeID:   tree.Root.ID,
					Label:      "test",
				},
			},
			wantError: true,
		},
		{
			name: "Unknown operation type",
			operation: DeltaOperation{
				Type:     Operation(999), // Invalid operation type
				ClientID: core.ClientID("client-1"),
				Clock:    vectorclock.VectorClock{core.ClientID("client-1"): 1},
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			delta := &Delta{
				Operations: []DeltaOperation{tt.operation},
				SourceID:   core.ClientID("client-1"),
			}

			err := deltaSync.ApplyDelta(delta)
			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDeltaWithInvalidClock(t *testing.T) {
	tree := NewTreeCRDT()
	deltaSync := NewDeltaSync(tree)

	// Create a node with initial clock
	tree.Nodes["test-node"] = &NodeCRDT{
		tree:         tree,
		ID:           "test-node",
		ParentID:     tree.Root.ID,
		IsLiteral:    true,
		LiteralValue: "initial",
		Clock:        vectorclock.VectorClock{core.ClientID("client-1"): 5},
		Owner:        core.ClientID("client-1"),
		Edges:        make([]*EdgeCRDT, 0),
	}

	// Try to apply operation with older clock (should be ignored for set literal)
	op := DeltaOperation{
		Type:     OPSetLiteral,
		NodeID:   core.NodeID("test-node"),
		Value:    "should-be-ignored",
		ClientID: core.ClientID("client-2"),
		Clock:    vectorclock.VectorClock{core.ClientID("client-1"): 3}, // Older than existing
	}

	err := deltaSync.applySetLiteral(op)
	assert.NoError(t, err) // Should not error, but should not change value

	// Value should remain unchanged
	node := tree.Nodes["test-node"]
	assert.Equal(t, "initial", node.LiteralValue)
	assert.Equal(t, core.ClientID("client-1"), node.Owner)
}

func TestHistoryTrimmingPreservesEssentialOperations(t *testing.T) {
	tree := NewTreeCRDT()
	deltaSync := NewDeltaSync(tree)
	
	// Set a very small history limit for testing
	deltaSync.maxHistory = 3

	clientID := core.ClientID("test-client")

	// Add more operations than the limit
	for i := 0; i < 5; i++ {
		op := DeltaOperation{
			Type:     OPCreateNode,
			NodeID:   core.NodeID(fmt.Sprintf("node-%d", i)),
			ClientID: clientID,
			Clock:    vectorclock.VectorClock{clientID: i + 1},
			}
		deltaSync.RecordOperation(op)
	}

	// Should only keep the last 3 operations
	assert.Len(t, deltaSync.history, 3)
	
	// Should have the last 3 operations
	expectedNodes := []string{"node-2", "node-3", "node-4"}
	for i, op := range deltaSync.history {
		assert.Equal(t, core.NodeID(expectedNodes[i]), op.NodeID)
	}
}

func TestDeltaGenerationWithTrimmedHistory(t *testing.T) {
	tree := NewTreeCRDT() 
	deltaSync := NewDeltaSync(tree)
	
	// Set small history limit
	deltaSync.maxHistory = 2

	clientID := core.ClientID("test-client")

	// Add operations that will be trimmed
	for i := 0; i < 4; i++ {
		op := DeltaOperation{
			Type:     OPCreateNode,
			NodeID:   core.NodeID(fmt.Sprintf("node-%d", i)),
			ClientID: clientID,
			Clock:    vectorclock.VectorClock{clientID: i + 1},
			}
		deltaSync.RecordOperation(op)
	}

	// History should be trimmed to 2 operations
	assert.Len(t, deltaSync.history, 2)

	// Generate delta from empty clock
	delta := deltaSync.GenerateDelta(vectorclock.VectorClock{}, clientID)
	
	// Should only include operations still in history
	assert.Len(t, delta.Operations, 2)
	assert.Equal(t, core.NodeID("node-2"), delta.Operations[0].NodeID)
	assert.Equal(t, core.NodeID("node-3"), delta.Operations[1].NodeID)

	// Generate delta from partial clock that should filter some operations
	partialClock := vectorclock.VectorClock{clientID: 3} // Should filter operations with clock <= 3
	delta2 := deltaSync.GenerateDelta(partialClock, clientID)
	
	// Should include only operations newer than the from clock
	assert.Len(t, delta2.Operations, 1)
	assert.Equal(t, core.NodeID("node-3"), delta2.Operations[0].NodeID)
}

// SecureTree integration tests

func TestSecureTreeGeneratesDeltas(t *testing.T) {
	prvKey := "d6eb959e9aec2e6fdc44b5862b269e987b8a4d6f2baca542d8acaa97ee5e74f6"
	
	// Create SecureTree with DeltaSync integration
	secureTree, err := NewSecureTree(prvKey)
	require.NoError(t, err)
	
	// Get underlying TreeCRDT
	adapter := secureTree.(*AdapterSecureTreeCRDT)
	tree := adapter.treeCrdt
	
	// Create DeltaSync (which will auto-register with the tree)
	deltaSync := NewDeltaSync(tree)
	
	// Initially no operations recorded
	assert.Len(t, deltaSync.history, 0)
	
	// Perform a SetLiteral operation through SecureTree
	root, err := secureTree.GetNodeByPath("/")
	require.NoError(t, err)
	
	// Set a literal value (this should record a delta operation)
	_, err = root.SetLiteral("test-value", prvKey)
	assert.NoError(t, err)
	
	// Check that a delta operation was recorded
	assert.Len(t, deltaSync.history, 1, "SetLiteral should generate a delta operation")
	
	op := deltaSync.history[0]
	assert.Equal(t, OPSetLiteral, op.Type)
	assert.Equal(t, root.ID(), op.NodeID)
	assert.Equal(t, "test-value", op.Value)
	assert.NotEmpty(t, op.ClientID)
	assert.NotEmpty(t, op.Clock)
}

func TestSetKeyValueGeneratesDelta(t *testing.T) {
	prvKey := "d6eb959e9aec2e6fdc44b5862b269e987b8a4d6f2baca542d8acaa97ee5e74f6"
	
	secureTree, err := NewSecureTree(prvKey)
	require.NoError(t, err)
	
	adapter := secureTree.(*AdapterSecureTreeCRDT)
	tree := adapter.treeCrdt
	deltaSync := NewDeltaSync(tree)
	
	// Get root and create a map node
	root, err := secureTree.GetNodeByPath("/")
	require.NoError(t, err)
	
	_, mapNode, err := root.CreateMapNode(prvKey)
	require.NoError(t, err)
	
	// The CreateMapNode operation involves multiple steps, so we should have some operations recorded
	initialOpCount := len(deltaSync.history)
	
	// Now set a key-value pair (this involves creating a literal node)
	_, nodeID, err := mapNode.SetKeyValue("testKey", "testValue", prvKey)
	assert.NoError(t, err)
	assert.NotEmpty(t, nodeID)
	
	// Should have recorded more operations
	finalOpCount := len(deltaSync.history)
	assert.Greater(t, finalOpCount, initialOpCount, "SetKeyValue should generate additional delta operations")
	
	// Check that we can retrieve the value
	valueNode, err := secureTree.GetNodeByPath("/testKey")
	assert.NoError(t, err)
	
	value, err := valueNode.GetLiteral()
	assert.NoError(t, err)
	assert.Equal(t, "testValue", value)
}

func TestImportJSONGeneratesDelta(t *testing.T) {
	prvKey := "d6eb959e9aec2e6fdc44b5862b269e987b8a4d6f2baca542d8acaa97ee5e74f6"
	
	secureTree, err := NewSecureTree(prvKey)
	require.NoError(t, err)
	
	adapter := secureTree.(*AdapterSecureTreeCRDT)
	tree := adapter.treeCrdt
	deltaSync := NewDeltaSync(tree)
	
	// Import JSON data
	jsonData := []byte(`{"name": "test", "value": 42, "nested": {"key": "data"}}`)
	nodeID, err := secureTree.ImportJSON(jsonData, prvKey)
	assert.NoError(t, err)
	assert.NotEmpty(t, nodeID)
	
	// Should have recorded operations from the import
	assert.Greater(t, len(deltaSync.history), 0, "ImportJSON should generate delta operations")
	
	// Verify we can access the imported data
	nameNode, err := secureTree.GetNodeByPath("/name")
	assert.NoError(t, err)
	
	name, err := nameNode.GetLiteral()
	assert.NoError(t, err)
	assert.Equal(t, "test", name)
	
	valueNode, err := secureTree.GetNodeByPath("/value")
	assert.NoError(t, err)
	
	value, err := valueNode.GetLiteral()
	assert.NoError(t, err)
	assert.Equal(t, float64(42), value) // JSON numbers are float64
}

func TestDeltaGenerationFromRecordedOperations(t *testing.T) {
	prvKey := "d6eb959e9aec2e6fdc44b5862b269e987b8a4d6f2baca542d8acaa97ee5e74f6"
	
	secureTree, err := NewSecureTree(prvKey)
	require.NoError(t, err)
	
	adapter := secureTree.(*AdapterSecureTreeCRDT)
	tree := adapter.treeCrdt
	deltaSync := NewDeltaSync(tree)
	
	// Perform some operations
	root, err := secureTree.GetNodeByPath("/")
	require.NoError(t, err)
	
	_, err = root.SetLiteral("value1", prvKey) 
	assert.NoError(t, err)
	
	_, mapNode, err := root.CreateMapNode(prvKey)
	require.NoError(t, err)
	
	_, _, err = mapNode.SetKeyValue("key1", "value2", prvKey)
	assert.NoError(t, err)
	
	// Should have recorded several operations
	assert.Greater(t, len(deltaSync.history), 0)
	
	// Generate a delta from the recorded operations
	clientID := core.ClientID("test-client")
	delta := deltaSync.GenerateDelta(make(map[core.ClientID]int), clientID)
	
	assert.NotNil(t, delta)
	assert.Equal(t, clientID, delta.SourceID)
	assert.Greater(t, len(delta.Operations), 0, "Delta should contain operations from recorded history")
	
	// All recorded operations should be included in the delta (since we're generating from empty clock)
	assert.Equal(t, len(deltaSync.history), len(delta.Operations))
}

// End-to-end synchronization tests  

func TestTwoTreeDeltaSync(t *testing.T) {
	prvKey := "d6eb959e9aec2e6fdc44b5862b269e987b8a4d6f2baca542d8acaa97ee5e74f6"
	
	// Create two independent SecureTree instances
	tree1, err := NewSecureTree(prvKey)
	require.NoError(t, err)
	
	tree2, err := NewSecureTree(prvKey)
	require.NoError(t, err)
	
	// Set up delta sync for both trees
	adapter1 := tree1.(*AdapterSecureTreeCRDT)
	adapter2 := tree2.(*AdapterSecureTreeCRDT)
	
	deltaSync1 := NewDeltaSync(adapter1.treeCrdt)
	deltaSync2 := NewDeltaSync(adapter2.treeCrdt)
	
	// Perform operations on tree1
	root1, err := tree1.GetNodeByPath("/")
	require.NoError(t, err)
	
	// Convert root to map and add some data to tree1
	_, mapNode, err := root1.CreateMapNode(prvKey)
	require.NoError(t, err)
	
	_, nodeID, err := mapNode.SetKeyValue("key1", "value1", prvKey)
	require.NoError(t, err)
	assert.NotEmpty(t, nodeID)
	
	_, nodeID2, err := mapNode.SetKeyValue("key2", "value2", prvKey)
	require.NoError(t, err)
	assert.NotEmpty(t, nodeID2)
	
	// tree1 should have the data
	key1Value, err := tree1.GetStringValueByPath("/key1")
	assert.NoError(t, err)
	assert.Equal(t, "value1", key1Value)
	
	key2Value, err := tree1.GetStringValueByPath("/key2")
	assert.NoError(t, err)
	assert.Equal(t, "value2", key2Value)
	
	// tree2 should not have the data yet
	_, err = tree2.GetStringValueByPath("/key1")
	assert.Error(t, err) // Should error because key doesn't exist
	
	_, err = tree2.GetStringValueByPath("/key2")
	assert.Error(t, err) // Should error because key doesn't exist
	
	// Generate delta from tree1's operations
	clientID := core.ClientID("sync-client")
	delta := deltaSync1.GenerateDelta(vectorclock.VectorClock{}, clientID)
	
	// Delta should contain operations
	assert.Greater(t, len(delta.Operations), 0, "Delta should contain operations from tree1")
	
	// Apply delta to tree2
	err = deltaSync2.ApplyDelta(delta)
	assert.NoError(t, err, "Delta application should succeed")
	
	// Now tree2 should have the same data as tree1
	key1Value2, err := tree2.GetStringValueByPath("/key1")
	assert.NoError(t, err)
	assert.Equal(t, "value1", key1Value2)
	
	key2Value2, err := tree2.GetStringValueByPath("/key2")
	assert.NoError(t, err)
	assert.Equal(t, "value2", key2Value2)
}

func TestDeltaSyncPreservesSemantics(t *testing.T) {
	prvKey := "d6eb959e9aec2e6fdc44b5862b269e987b8a4d6f2baca542d8acaa97ee5e74f6"
	
	// Create three trees: original, replica1, replica2
	original, err := NewSecureTree(prvKey)
	require.NoError(t, err)
	
	replica1, err := NewSecureTree(prvKey)
	require.NoError(t, err)
	
	replica2, err := NewSecureTree(prvKey)
	require.NoError(t, err)
	
	// Set up delta sync
	originalAdapter := original.(*AdapterSecureTreeCRDT)
	replica1Adapter := replica1.(*AdapterSecureTreeCRDT)
	replica2Adapter := replica2.(*AdapterSecureTreeCRDT)
	
	originalDelta := NewDeltaSync(originalAdapter.treeCrdt)
	replica1Delta := NewDeltaSync(replica1Adapter.treeCrdt)
	replica2Delta := NewDeltaSync(replica2Adapter.treeCrdt)
	
	// Perform operations on original
	jsonData := []byte(`{
		"users": {
			"alice": {"name": "Alice", "age": 30},
			"bob": {"name": "Bob", "age": 25}
		},
		"settings": {
			"theme": "dark",
			"notifications": true
		}
	}`)
	
	_, err = original.ImportJSON(jsonData, prvKey)
	require.NoError(t, err)
	
	// Generate delta and sync to both replicas
	syncClient := core.ClientID("sync-client")
	delta := originalDelta.GenerateDelta(vectorclock.VectorClock{}, syncClient)
	
	err = replica1Delta.ApplyDelta(delta)
	assert.NoError(t, err)
	
	err = replica2Delta.ApplyDelta(delta)
	assert.NoError(t, err)
	
	// All trees should have the same data
	testPaths := []string{
		"/users/alice/name",
		"/users/alice/age",
		"/users/bob/name", 
		"/users/bob/age",
		"/settings/theme",
		"/settings/notifications",
	}
	
	expectedValues := []interface{}{
		"Alice", float64(30), "Bob", float64(25), "dark", true,
	}
	
	for i, path := range testPaths {
		// Get value from original
		originalValue, err := original.GetValueByPath(path)
		assert.NoError(t, err, "Original should have path %s", path)
		
		// Get value from replica1
		replica1Value, err := replica1.GetValueByPath(path)
		assert.NoError(t, err, "Replica1 should have path %s", path)
		
		// Get value from replica2
		replica2Value, err := replica2.GetValueByPath(path)
		assert.NoError(t, err, "Replica2 should have path %s", path)
		
		// All should be equal
		assert.Equal(t, expectedValues[i], originalValue)
		assert.Equal(t, originalValue, replica1Value)
		assert.Equal(t, originalValue, replica2Value)
	}
}

func TestDeltaSyncWithConflicts(t *testing.T) {
	// Use different private keys to simulate different clients in a distributed system
	prvKey1 := "d6eb959e9aec2e6fdc44b5862b269e987b8a4d6f2baca542d8acaa97ee5e74f6"
	prvKey2 := "a1b2c3d4e5f6789abcdef0123456789abcdef0123456789abcdef0123456789a"
	
	// Create two trees that will have concurrent modifications
	tree1, err := NewSecureTree(prvKey1)
	require.NoError(t, err)
	
	tree2, err := NewSecureTree(prvKey2)
	require.NoError(t, err)
	
	adapter1 := tree1.(*AdapterSecureTreeCRDT)
	adapter2 := tree2.(*AdapterSecureTreeCRDT)
	
	deltaSync1 := NewDeltaSync(adapter1.treeCrdt)
	deltaSync2 := NewDeltaSync(adapter2.treeCrdt)
	
	// First, sync initial state
	root1, err := tree1.GetNodeByPath("/")
	require.NoError(t, err)
	
	_, mapNode1, err := root1.CreateMapNode(prvKey1)
	require.NoError(t, err)
	
	_, _, err = mapNode1.SetKeyValue("counter", float64(0), prvKey1)
	require.NoError(t, err)
	
	// Sync initial state to tree2
	syncClient := core.ClientID("sync-client")
	delta := deltaSync1.GenerateDelta(vectorclock.VectorClock{}, syncClient)
	err = deltaSync2.ApplyDelta(delta)
	require.NoError(t, err)
	
	// Both trees should have counter = 0
	counter1, err := tree1.GetValueByPath("/counter")
	assert.NoError(t, err)
	assert.Equal(t, float64(0), counter1)
	
	counter2, err := tree2.GetValueByPath("/counter")
	assert.NoError(t, err)
	assert.Equal(t, float64(0), counter2)
	
	// Now make concurrent conflicting updates
	// tree1: set counter to 100
	counterNode1, err := tree1.GetNodeByPath("/counter")
	require.NoError(t, err)
	_, err = counterNode1.SetLiteral(float64(100), prvKey1)
	assert.NoError(t, err)
	
	// tree2: set counter to 200 (conflicts with tree1)
	counterNode2, err := tree2.GetNodeByPath("/counter")
	require.NoError(t, err)
	_, err = counterNode2.SetLiteral(float64(200), prvKey2)
	assert.NoError(t, err)
	
	// Generate deltas from both trees
	tree1Clock := adapter1.treeCrdt.GetVectorClock()
	tree2Clock := adapter2.treeCrdt.GetVectorClock()
	
	delta1 := deltaSync1.GenerateDelta(tree2Clock, syncClient)
	delta2 := deltaSync2.GenerateDelta(tree1Clock, syncClient)
	
	// Apply cross-deltas (simulate distributed sync)
	err = deltaSync2.ApplyDelta(delta1) // Apply tree1 changes to tree2
	assert.NoError(t, err)
	
	err = deltaSync1.ApplyDelta(delta2) // Apply tree2 changes to tree1  
	assert.NoError(t, err)
	
	// After sync, both trees should have converged to the same value
	// The exact value depends on conflict resolution rules (typically last-writer-wins or lowest-client-ID-wins)
	finalValue1, err := tree1.GetValueByPath("/counter")
	assert.NoError(t, err)
	
	finalValue2, err := tree2.GetValueByPath("/counter")
	assert.NoError(t, err)
	
	// Both should converge to the same value
	assert.Equal(t, finalValue1, finalValue2, "Both trees should converge after delta sync")
	
	// The final value should be one of the concurrent values
	assert.True(t, finalValue1 == float64(100) || finalValue1 == float64(200),
		"Final value should be one of the conflicting values: %v", finalValue1)
}

// Legacy tests for backward compatibility

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
			Type:     OPCreateNode,
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