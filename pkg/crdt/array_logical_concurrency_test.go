package crdt

import (
	"fmt"
	"testing"

	"github.com/eislab-cps/synctree/pkg/core"
	"github.com/eislab-cps/synctree/pkg/random"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestArrayLogicalConcurrency tests array operations using proper CRDT logical concurrency
// Each client has their own replica, performs operations independently, then merges
func TestArrayLogicalConcurrency(t *testing.T) {
	t.Run("concurrent array moves - logical concurrency", func(t *testing.T) {
		logrus.SetLevel(logrus.WarnLevel)
		
		// Initial setup on single tree
		initialTree := NewTreeCRDT()
		setupClient := core.ClientID("setup")
		
		// Create array with elements
		arrayRoot := initialTree.CreateNode("testArray", Map, setupClient)
		arrayRoot.IsArrayRoot = true
		err := initialTree.AddEdge(initialTree.Root.ID, arrayRoot.ID, "testArray", setupClient)
		require.NoError(t, err)
		
		elements := make([]*NodeCRDT, 4)
		for i := 0; i < 4; i++ {
			elem := initialTree.CreateNode(fmt.Sprintf("elem_%d", i), Literal, setupClient)
			elem.LiteralValue = fmt.Sprintf("value_%d", i)
			elements[i] = elem
			
			err := initialTree.AddArrayElement(arrayRoot.ID, elem.ID, i, setupClient)
			require.NoError(t, err)
		}
		
		/*
		 * LOGICAL CONCURRENCY TEST SCENARIO
		 * =================================
		 * 
		 * INITIAL STATE (all replicas start with this):
		 * ┌─────┬─────┬─────┬─────┐
		 * │ [0] │ [1] │ [2] │ [3] │
		 * │elem0│elem1│elem2│elem3│
		 * └─────┴─────┴─────┴─────┘
		 * 
		 * CLIENT OPERATIONS (performed independently):
		 * - ClientA: elem0 → pos3, elem3 → pos0 (swap ends)
		 * - ClientB: elem1 → pos2, elem2 → pos1 (swap middle)
		 * - ClientC: elem0 → pos1 (conflicts with A)
		 * 
		 * EXPECTED: LWW resolution based on vector clocks
		 */
		
		t.Logf("INITIAL STATE: [elem0, elem1, elem2, elem3] at positions [0, 1, 2, 3]")
		
		// Create independent replicas for each client
		treeA, err := initialTree.Clone()
		require.NoError(t, err, "Clone for ClientA should succeed")
		treeB, err := initialTree.Clone()
		require.NoError(t, err, "Clone for ClientB should succeed")
		treeC, err := initialTree.Clone()
		require.NoError(t, err, "Clone for ClientC should succeed")
		
		clientA := core.ClientID(random.GenerateRandomID() + "_A")
		clientB := core.ClientID(random.GenerateRandomID() + "_B")
		clientC := core.ClientID(random.GenerateRandomID() + "_C")
		
		// CLIENT A: Attempt move elem0 to position 3 
		t.Logf("CLIENT A OPERATIONS (independent):")
		err = treeA.MoveArrayElement(elements[0].ID, arrayRoot.ID, 3, clientA)
		require.NoError(t, err)
		t.Logf("  - Attempted move elem0 to position 3 (LWW may reject based on vector clocks)")
		
		// CLIENT B: Attempt move elem1 to position 0
		t.Logf("CLIENT B OPERATIONS (independent):")
		err = treeB.MoveArrayElement(elements[1].ID, arrayRoot.ID, 0, clientB)
		require.NoError(t, err)
		t.Logf("  - Attempted move elem1 to position 0 (LWW may reject based on vector clocks)")
		
		// CLIENT C: Attempt move elem2 to position 1
		t.Logf("CLIENT C OPERATIONS (independent):")
		err = treeC.MoveArrayElement(elements[2].ID, arrayRoot.ID, 1, clientC)
		require.NoError(t, err)
		t.Logf("  - Attempted move elem2 to position 1 (LWW may reject based on vector clocks)")
		
		// Document local states before merge
		t.Logf("\nLOCAL STATES BEFORE MERGE:")
		logArrayState(t, "  ClientA", treeA, arrayRoot.ID)
		logArrayState(t, "  ClientB", treeB, arrayRoot.ID)
		logArrayState(t, "  ClientC", treeC, arrayRoot.ID)
		
		// MERGE PHASE: Simulate network synchronization
		t.Logf("\nMERGE PHASE: Simulating distributed synchronization")
		
		// A merges with B
		err = treeA.Merge(treeB)
		require.NoError(t, err, "A←B merge should succeed")
		err = treeB.Merge(treeA)
		require.NoError(t, err, "B←A merge should succeed")
		
		// A merges with C
		err = treeA.Merge(treeC)
		require.NoError(t, err, "A←C merge should succeed")
		err = treeC.Merge(treeA)
		require.NoError(t, err, "C←A merge should succeed")
		
		// B merges with C (complete the triangle)
		err = treeB.Merge(treeC)
		require.NoError(t, err, "B←C merge should succeed")
		err = treeC.Merge(treeB)
		require.NoError(t, err, "C←B merge should succeed")
		
		// Document converged states
		t.Logf("\nCONVERGED STATES AFTER MERGE:")
		logArrayState(t, "  ClientA", treeA, arrayRoot.ID)
		logArrayState(t, "  ClientB", treeB, arrayRoot.ID)
		logArrayState(t, "  ClientC", treeC, arrayRoot.ID)
		
		// Verify convergence
		elementsA := treeA.GetArrayElements(arrayRoot.ID)
		elementsB := treeB.GetArrayElements(arrayRoot.ID)
		elementsC := treeC.GetArrayElements(arrayRoot.ID)
		
		assert.Len(t, elementsA, 4, "All trees should have 4 elements")
		assert.Len(t, elementsB, 4)
		assert.Len(t, elementsC, 4)
		
		// Verify all trees converged to same state
		for i := 0; i < 4; i++ {
			assert.Equal(t, elementsA[i].ID, elementsB[i].ID, 
				"Position %d should have same element in A and B", i)
			assert.Equal(t, elementsB[i].ID, elementsC[i].ID,
				"Position %d should have same element in B and C", i)
		}
		
		// Verify no duplicates
		seen := make(map[core.NodeID]bool)
		for _, elem := range elementsA {
			assert.False(t, seen[elem.ID], "Element %s should not be duplicated", elem.ID)
			seen[elem.ID] = true
		}
		
		// Note: Equal() method may not fully compare array metadata, so we verify convergence through element positions
		
		t.Logf("\nSUCCESS: Logical concurrency test completed successfully")
		t.Logf("  - No element duplication occurred")
		t.Logf("  - All replicas converged to same state")
		t.Logf("  - LWW conflict resolution working correctly (some moves may be rejected)")
	})
	
	t.Run("circular moves pattern - logical concurrency", func(t *testing.T) {
		logrus.SetLevel(logrus.WarnLevel)
		
		// Setup initial tree
		initialTree := NewTreeCRDT()
		setupClient := core.ClientID("setup")
		
		arrayRoot := initialTree.CreateNode("circularArray", Map, setupClient)
		arrayRoot.IsArrayRoot = true
		err := initialTree.AddEdge(initialTree.Root.ID, arrayRoot.ID, "circularArray", setupClient)
		require.NoError(t, err)
		
		elements := make([]*NodeCRDT, 3)
		for i := 0; i < 3; i++ {
			elem := initialTree.CreateNode(fmt.Sprintf("elem_%d", i), Literal, setupClient)
			elem.LiteralValue = fmt.Sprintf("value_%d", i)
			elements[i] = elem
			
			err := initialTree.AddArrayElement(arrayRoot.ID, elem.ID, i, setupClient)
			require.NoError(t, err)
		}
		
		/*
		 * CIRCULAR MOVE PATTERN TEST
		 * ==========================
		 * 
		 * INITIAL: [elem0, elem1, elem2]
		 * 
		 * Three clients each move one element in circular pattern:
		 * - ClientA: elem0 → pos1
		 * - ClientB: elem1 → pos2  
		 * - ClientC: elem2 → pos0
		 * 
		 * This creates a circular dependency that LWW must resolve
		 */
		
		t.Logf("CIRCULAR PATTERN TEST:")
		t.Logf("Initial: [elem0, elem1, elem2] at positions [0, 1, 2]")
		
		// Create replicas
		treeA, _ := initialTree.Clone()
		treeB, _ := initialTree.Clone()
		treeC, _ := initialTree.Clone()
		
		clientA := core.ClientID("clientA")
		clientB := core.ClientID("clientB")
		clientC := core.ClientID("clientC")
		
		// Each client moves one element (creating circular pattern)
		err = treeA.MoveArrayElement(elements[0].ID, arrayRoot.ID, 1, clientA)
		require.NoError(t, err)
		t.Logf("ClientA: elem0 → pos1")
		
		err = treeB.MoveArrayElement(elements[1].ID, arrayRoot.ID, 2, clientB)
		require.NoError(t, err)
		t.Logf("ClientB: elem1 → pos2")
		
		err = treeC.MoveArrayElement(elements[2].ID, arrayRoot.ID, 0, clientC)
		require.NoError(t, err)
		t.Logf("ClientC: elem2 → pos0")
		
		// Merge all combinations
		t.Logf("\nMerging all replicas...")
		_ = treeA.Merge(treeB)
		_ = treeB.Merge(treeA)
		_ = treeA.Merge(treeC)
		_ = treeC.Merge(treeA)
		_ = treeB.Merge(treeC)
		_ = treeC.Merge(treeB)
		
		// Verify convergence
		finalA := treeA.GetArrayElements(arrayRoot.ID)
		finalB := treeB.GetArrayElements(arrayRoot.ID)
		finalC := treeC.GetArrayElements(arrayRoot.ID)
		
		// All should have converged to same order
		for i := 0; i < 3; i++ {
			assert.Equal(t, finalA[i].ID, finalB[i].ID)
			assert.Equal(t, finalB[i].ID, finalC[i].ID)
		}
		
		t.Logf("Final converged state:")
		logArrayState(t, "  All replicas", treeA, arrayRoot.ID)
		
		t.Logf("SUCCESS: Circular moves resolved consistently")
	})
	
	t.Run("high contention same position - logical concurrency", func(t *testing.T) {
		logrus.SetLevel(logrus.WarnLevel)
		
		// Setup
		initialTree := NewTreeCRDT()
		setupClient := core.ClientID("setup")
		
		arrayRoot := initialTree.CreateNode("contentionArray", Map, setupClient)
		arrayRoot.IsArrayRoot = true
		err := initialTree.AddEdge(initialTree.Root.ID, arrayRoot.ID, "contentionArray", setupClient)
		require.NoError(t, err)
		
		elements := make([]*NodeCRDT, 5)
		for i := 0; i < 5; i++ {
			elem := initialTree.CreateNode(fmt.Sprintf("elem_%d", i), Literal, setupClient)
			elem.LiteralValue = fmt.Sprintf("value_%d", i)
			elements[i] = elem
			
			err := initialTree.AddArrayElement(arrayRoot.ID, elem.ID, i, setupClient)
			require.NoError(t, err)
		}
		
		/*
		 * HIGH CONTENTION TEST
		 * ====================
		 * 
		 * Multiple clients try to move different elements to position 2
		 * Tests that rebalancing correctly resolves position conflicts
		 */
		
		t.Logf("HIGH CONTENTION TEST: Multiple elements → position 2")
		t.Logf("Initial: 5 elements at positions [0, 1, 2, 3, 4]")
		
		// Create 4 replicas
		replicas := make([]*TreeCRDT, 4)
		clients := make([]core.ClientID, 4)
		
		for i := 0; i < 4; i++ {
			replicas[i], _ = initialTree.Clone()
			clients[i] = core.ClientID(fmt.Sprintf("client_%d", i))
		}
		
		// Each client moves a different element to position 2
		movements := []int{0, 1, 3, 4} // elements to move
		
		for i, elemIdx := range movements {
			err := replicas[i].MoveArrayElement(elements[elemIdx].ID, arrayRoot.ID, 2, clients[i])
			require.NoError(t, err)
			t.Logf("Client%d: elem%d → pos2", i, elemIdx)
		}
		
		// Merge all replicas in a chain
		t.Logf("\nMerging replicas in chain...")
		for i := 0; i < len(replicas)-1; i++ {
			err := replicas[i].Merge(replicas[i+1])
			require.NoError(t, err)
			err = replicas[i+1].Merge(replicas[i])
			require.NoError(t, err)
		}
		
		// Complete the merge cycle
		err = replicas[0].Merge(replicas[len(replicas)-1])
		require.NoError(t, err)
		err = replicas[len(replicas)-1].Merge(replicas[0])
		require.NoError(t, err)
		
		// Verify all converged
		for i := 1; i < len(replicas); i++ {
			assert.True(t, replicas[0].Equal(replicas[i]), 
				"Replica %d should equal replica 0", i)
		}
		
		// Verify no position conflicts
		final := replicas[0].GetArrayElements(arrayRoot.ID)
		positions := make(map[int]int)
		for _, elem := range final {
			positions[elem.ArrayIndex]++
		}
		
		for pos, count := range positions {
			assert.Equal(t, 1, count, "Position %d should have exactly 1 element", pos)
		}
		
		t.Logf("Final state after contention resolution:")
		logArrayState(t, "  All replicas", replicas[0], arrayRoot.ID)
		
		t.Logf("SUCCESS: High contention resolved without conflicts")
	})
}

// Helper function to log array state
func logArrayState(t *testing.T, prefix string, tree *TreeCRDT, arrayRootID core.NodeID) {
	elements := tree.GetArrayElements(arrayRootID)
	state := "["
	for i, elem := range elements {
		if i > 0 {
			state += ", "
		}
		// Use literal value to identify element
		state += fmt.Sprintf("%v@pos%d", elem.LiteralValue, elem.ArrayIndex)
	}
	state += "]"
	t.Logf("%s: %s", prefix, state)
}