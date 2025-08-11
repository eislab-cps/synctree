package crdt

import (
	"fmt"
	"testing"

	"github.com/eislab-cps/synctree/pkg/core"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConcurrentMoveDuplication tests that concurrent moves don't cause element duplication
func TestConcurrentMoveDuplication(t *testing.T) {
	t.Run("concurrent moves of same element - no duplication", func(t *testing.T) {
		logrus.SetLevel(logrus.WarnLevel)
		
		// Create initial tree
		tree1 := NewTreeCRDT()
		clientSetup := core.ClientID("setup")
		
		// Create array with one element
		arrayRoot := tree1.CreateNode("array", Map, clientSetup)
		arrayRoot.IsArrayRoot = true
		
		element := tree1.CreateNode("elem", Literal, clientSetup)
		element.LiteralValue = "test_value"
		
		err := tree1.AddArrayElement(arrayRoot.ID, element.ID, 0, clientSetup)
		require.NoError(t, err)
		
		// Clone for second client
		tree2, err := tree1.Clone()
		require.NoError(t, err)
		
		// Different clients move same element to different positions
		clientA := core.ClientID("clientA")
		clientB := core.ClientID("clientB")
		
		err = tree1.MoveArrayElement(element.ID, arrayRoot.ID, 1, clientA)
		require.NoError(t, err)
		
		err = tree2.MoveArrayElement(element.ID, arrayRoot.ID, 2, clientB)
		require.NoError(t, err)
		
		// Merge trees
		err = tree1.Merge(tree2)
		require.NoError(t, err)
		err = tree2.Merge(tree1)
		require.NoError(t, err)
		
		// Verify no duplication
		elements1 := tree1.GetArrayElements(arrayRoot.ID)
		elements2 := tree2.GetArrayElements(arrayRoot.ID)
		
		assert.Len(t, elements1, 1, "Should have exactly 1 element after merge (no duplication)")
		assert.Len(t, elements2, 1, "Should have exactly 1 element after merge (no duplication)")
		assert.Equal(t, elements1[0].ID, element.ID, "Should be the original element")
		
		t.Logf("SUCCESS: No duplication occurred. Element at position %d", elements1[0].ArrayIndex)
	})
	
	t.Run("multiple concurrent moves - no duplication", func(t *testing.T) {
		logrus.SetLevel(logrus.WarnLevel)
		
		// Setup
		tree1 := NewTreeCRDT()
		clientSetup := core.ClientID("setup")
		
		arrayRoot := tree1.CreateNode("array", Map, clientSetup)
		arrayRoot.IsArrayRoot = true
		
		// Create 3 elements
		elements := make([]*NodeCRDT, 3)
		for i := 0; i < 3; i++ {
			elem := tree1.CreateNode(fmt.Sprintf("elem%d", i), Literal, clientSetup)
			elem.LiteralValue = fmt.Sprintf("value%d", i)
			elements[i] = elem
			
			err := tree1.AddArrayElement(arrayRoot.ID, elem.ID, i, clientSetup)
			require.NoError(t, err)
		}
		
		// Create replicas
		tree2, _ := tree1.Clone()
		tree3, _ := tree1.Clone()
		
		// Each replica moves elements differently
		clientA := core.ClientID("clientA")
		clientB := core.ClientID("clientB")
		clientC := core.ClientID("clientC")
		
		// Tree1: swap first two
		_ = tree1.MoveArrayElement(elements[0].ID, arrayRoot.ID, 1, clientA)
		_ = tree1.MoveArrayElement(elements[1].ID, arrayRoot.ID, 0, clientA)
		
		// Tree2: move last to first
		_ = tree2.MoveArrayElement(elements[2].ID, arrayRoot.ID, 0, clientB)
		
		// Tree3: reverse all
		_ = tree3.MoveArrayElement(elements[0].ID, arrayRoot.ID, 2, clientC)
		_ = tree3.MoveArrayElement(elements[2].ID, arrayRoot.ID, 0, clientC)
		
		// Merge all trees
		_ = tree1.Merge(tree2)
		_ = tree2.Merge(tree1)
		_ = tree1.Merge(tree3)
		_ = tree3.Merge(tree1)
		_ = tree2.Merge(tree3)
		_ = tree3.Merge(tree2)
		
		// Verify no duplication
		final := tree1.GetArrayElements(arrayRoot.ID)
		assert.Len(t, final, 3, "Should still have exactly 3 elements")
		
		// Check each element appears exactly once
		seen := make(map[core.NodeID]int)
		for _, elem := range final {
			seen[elem.ID]++
		}
		
		for id, count := range seen {
			assert.Equal(t, 1, count, "Element %s should appear exactly once", id)
		}
		
		t.Logf("SUCCESS: All 3 elements preserved without duplication")
	})
}

// TestConcurrentMoveTreeLoops tests that moves don't create loops in tree structure
func TestConcurrentMoveTreeLoops(t *testing.T) {
	t.Run("moves within array don't create tree loops", func(t *testing.T) {
		logrus.SetLevel(logrus.WarnLevel)
		
		tree := NewTreeCRDT()
		client := core.ClientID("client")
		
		// Create tree structure: root -> parent -> array -> elements
		parent := tree.CreateNode("parent", Map, client)
		err := tree.AddEdge(tree.Root.ID, parent.ID, "parent", client)
		require.NoError(t, err)
		
		arrayRoot := tree.CreateNode("array", Map, client)
		arrayRoot.IsArrayRoot = true
		err = tree.AddEdge(parent.ID, arrayRoot.ID, "array", client)
		require.NoError(t, err)
		
		// Add elements
		elem1 := tree.CreateNode("elem1", Literal, client)
		elem1.LiteralValue = "val1"
		elem2 := tree.CreateNode("elem2", Literal, client)
		elem2.LiteralValue = "val2"
		
		_ = tree.AddArrayElement(arrayRoot.ID, elem1.ID, 0, client)
		_ = tree.AddArrayElement(arrayRoot.ID, elem2.ID, 1, client)
		
		// Perform moves
		_ = tree.MoveArrayElement(elem1.ID, arrayRoot.ID, 1, client)
		_ = tree.MoveArrayElement(elem2.ID, arrayRoot.ID, 0, client)
		
		// Check for cycles
		assert.False(t, hasCycle(tree), "Tree should not have cycles after array moves")
		
		// Verify tree structure integrity
		assert.Equal(t, tree.Root.ID, parent.ParentID, "Parent should still be under root")
		assert.Equal(t, parent.ID, arrayRoot.ParentID, "Array should still be under parent")
		assert.Equal(t, arrayRoot.ID, elem1.ParentID, "Element1 should still be under array")
		assert.Equal(t, arrayRoot.ID, elem2.ParentID, "Element2 should still be under array")
		
		t.Logf("SUCCESS: Tree structure preserved, no loops created")
	})
	
	t.Run("reject moves that would create tree loops", func(t *testing.T) {
		logrus.SetLevel(logrus.WarnLevel)
		
		tree := NewTreeCRDT()
		client := core.ClientID("client")
		
		// Create: root -> nodeA -> arrayB
		nodeA := tree.CreateNode("nodeA", Map, client)
		err := tree.AddEdge(tree.Root.ID, nodeA.ID, "nodeA", client)
		require.NoError(t, err)
		
		arrayB := tree.CreateNode("arrayB", Map, client)
		arrayB.IsArrayRoot = true
		err = tree.AddEdge(nodeA.ID, arrayB.ID, "arrayB", client)
		require.NoError(t, err)
		
		// Try to move nodeA into arrayB (would create cycle)
		err = tree.MoveArrayElement(nodeA.ID, arrayB.ID, 0, client)
		
		// Should be rejected because nodeA is not an array element
		assert.Error(t, err, "Should reject move that would create cycle")
		assert.Contains(t, err.Error(), "not an array element")
		
		// Verify no cycle was created
		assert.False(t, hasCycle(tree), "Tree should not have cycles")
		
		t.Logf("SUCCESS: Circular move properly rejected")
	})
}

// Helper to check for cycles in tree
func hasCycle(tree *TreeCRDT) bool {
	visited := make(map[core.NodeID]bool)
	recStack := make(map[core.NodeID]bool)
	
	var dfs func(nodeID core.NodeID) bool
	dfs = func(nodeID core.NodeID) bool {
		visited[nodeID] = true
		recStack[nodeID] = true
		
		node := tree.Nodes[nodeID]
		if node == nil {
			return false
		}
		
		// Check edges
		for _, edge := range node.Edges {
			if !visited[edge.To] {
				if dfs(edge.To) {
					return true
				}
			} else if recStack[edge.To] {
				return true // Found cycle
			}
		}
		
		// Check array children
		if node.IsArrayRoot {
			for _, child := range tree.GetArrayElements(nodeID) {
				if !visited[child.ID] {
					if dfs(child.ID) {
						return true
					}
				} else if recStack[child.ID] {
					return true // Found cycle
				}
			}
		}
		
		recStack[nodeID] = false
		return false
	}
	
	return dfs(tree.Root.ID)
}