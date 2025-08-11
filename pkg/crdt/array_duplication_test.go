package crdt

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/eislab-cps/synctree/pkg/core"
	"github.com/eislab-cps/synctree/pkg/random"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestArrayMoveDuplicationPrevention tests that concurrent moves prevent the 
// delete-insert duplication problem that occurs with traditional approaches
func TestArrayMoveDuplicationPrevention(t *testing.T) {
	t.Run("single element concurrent moves - no duplication", func(t *testing.T) {
		logrus.SetLevel(logrus.WarnLevel)
		tree := NewTreeCRDT()
		setupClient := core.ClientID(random.GenerateRandomID())
		
		// Create array with one element to test duplication prevention
		arrayRoot := tree.CreateNode("testArray", Map, setupClient)
		arrayRoot.IsArrayRoot = true
		
		element := tree.CreateNode("movableElement", Literal, setupClient)
		element.LiteralValue = "test_value"
		
		err := tree.AddArrayElement(arrayRoot.ID, element.ID, 0, setupClient)
		require.NoError(t, err)
		
		/*
		 * VISUAL REPRESENTATION - DUPLICATION PREVENTION TEST
		 * ==================================================
		 * 
		 * INITIAL STATE:
		 * ┌─────┐
		 * │ [0] │  <- Array position
		 * │elem │  <- Single element
		 * └─────┘
		 * 
		 * CONCURRENT MOVES (traditional delete-insert would duplicate):
		 * Client1: elem pos0 → pos1 ┐
		 * Client2: elem pos0 → pos2 ├─ Same element, different targets
		 * Client3: elem pos0 → pos3 ┘  (high contention scenario)
		 * 
		 * EXPECTED OUTCOME (atomic moves prevent duplication):
		 * ┌─────┬─────┬─────┬─────┐
		 * │ [0] │ [1] │ [2] │ [3] │  <- Possible final positions  
		 * │ ??  │ ??  │elem │ ??  │  <- Element appears EXACTLY ONCE
		 * └─────┴─────┴─────┴─────┘    (LWW determines final position)
		 */
		
		t.Logf("INITIAL STATE: Single element at position 0")
		t.Logf("CONCURRENT MOVES: 3 clients move same element to different positions")
		t.Logf("EXPECTED: Element appears exactly once (no duplication)")
		
		// Simulate high-contention concurrent moves
		var wg sync.WaitGroup
		results := make(chan string, 3)
		
		clients := []core.ClientID{
			core.ClientID(random.GenerateRandomID()),
			core.ClientID(random.GenerateRandomID()),
			core.ClientID(random.GenerateRandomID()),
		}
		
		targetPositions := []int{1, 2, 3}
		
		for i, client := range clients {
			wg.Add(1)
			go func(clientID core.ClientID, targetPos int) {
				defer wg.Done()
				time.Sleep(time.Duration(i*10) * time.Microsecond) // Slight timing variation
				
				err := tree.MoveArrayElement(element.ID, arrayRoot.ID, targetPos, clientID)
				if err != nil {
					results <- fmt.Sprintf("Move to pos%d FAILED: %v", targetPos, err)
				} else {
					results <- fmt.Sprintf("Move to pos%d SUCCESS", targetPos)
				}
			}(client, targetPositions[i])
		}
		
		wg.Wait()
		close(results)
		
		// Log concurrent operation results
		for result := range results {
			t.Logf("Concurrent move result: %s", result)
		}
		
		// Critical verification: No duplication
		finalElements := tree.GetArrayElements(arrayRoot.ID)
		t.Logf("ACTUAL STATE: Array contains %d elements", len(finalElements))
		
		// The key test: element should appear EXACTLY once
		assert.Len(t, finalElements, 1, "Element should appear exactly once (no duplication)")
		assert.Equal(t, element.ID, finalElements[0].ID, "Should be our original element")
		
		// Verify element is at one of the attempted positions
		finalPosition := finalElements[0].ArrayIndex
		t.Logf("Element final position: %d", finalPosition)
		assert.Contains(t, []int{0, 1, 2, 3}, finalPosition, "Element should be at one of the attempted positions")
		
		t.Logf("SUCCESS: No duplication occurred - element exists exactly once")
	})
	
	t.Run("multiple elements concurrent moves - no duplication", func(t *testing.T) {
		logrus.SetLevel(logrus.WarnLevel)
		tree := NewTreeCRDT()
		setupClient := core.ClientID(random.GenerateRandomID())
		
		// Create array with 3 elements
		arrayRoot := tree.CreateNode("multiArray", Map, setupClient)
		arrayRoot.IsArrayRoot = true
		
		elements := make([]*NodeCRDT, 3)
		for i := 0; i < 3; i++ {
			elem := tree.CreateNode(fmt.Sprintf("elem_%d", i), Literal, setupClient)
			elem.LiteralValue = fmt.Sprintf("value_%d", i)
			elements[i] = elem
			
			err := tree.AddArrayElement(arrayRoot.ID, elem.ID, i, setupClient)
			require.NoError(t, err)
		}
		
		t.Logf("INITIAL STATE: 3 elements at positions [0, 1, 2]")
		t.Logf("CONCURRENT STRESS TEST: Multiple elements moving simultaneously")
		
		// Create high-stress concurrent scenario
		var wg sync.WaitGroup
		results := make(chan string, 10)
		
		// Multiple concurrent operations on different elements
		operations := []struct {
			elementIdx  int
			targetPos   int
			clientSuffix string
		}{
			{0, 2, "A"}, // elem0 → pos2
			{1, 0, "B"}, // elem1 → pos0  
			{2, 1, "C"}, // elem2 → pos1
			{0, 1, "D"}, // elem0 → pos1 (competing with previous)
			{1, 2, "E"}, // elem1 → pos2 (competing)
		}
		
		for _, op := range operations {
			wg.Add(1)
			go func(elemIdx, targetPos int, clientSuffix string) {
				defer wg.Done()
				client := core.ClientID(random.GenerateRandomID() + clientSuffix)
				
				err := tree.MoveArrayElement(elements[elemIdx].ID, arrayRoot.ID, targetPos, client)
				if err != nil {
					results <- fmt.Sprintf("elem%d→pos%d FAILED: %v", elemIdx, targetPos, err)
				} else {
					results <- fmt.Sprintf("elem%d→pos%d SUCCESS", elemIdx, targetPos)
				}
			}(op.elementIdx, op.targetPos, op.clientSuffix)
		}
		
		wg.Wait()
		close(results)
		
		// Log all results
		for result := range results {
			t.Logf("Operation result: %s", result)
		}
		
		// Critical verification: All elements exist exactly once
		finalElements := tree.GetArrayElements(arrayRoot.ID)
		t.Logf("ACTUAL FINAL STATE:")
		
		// Check total count
		assert.Len(t, finalElements, 3, "Should have exactly 3 elements (no duplication or loss)")
		
		// Verify each original element appears exactly once
		elementCounts := make(map[core.NodeID]int)
		for _, finalElem := range finalElements {
			elementCounts[finalElem.ID]++
			t.Logf("  %s at position %d", finalElem.ID, finalElem.ArrayIndex)
		}
		
		for i, originalElem := range elements {
			count := elementCounts[originalElem.ID]
			assert.Equal(t, 1, count, "Element %d should appear exactly once, found %d times", i, count)
		}
		
		// Verify no position conflicts
		positionCounts := make(map[int]int)
		for _, elem := range finalElements {
			positionCounts[elem.ArrayIndex]++
		}
		
		for pos, count := range positionCounts {
			assert.Equal(t, 1, count, "Position %d should have exactly 1 element, found %d", pos, count)
		}
		
		t.Logf("SUCCESS: All elements preserved uniquely, no duplication or loss")
	})
}

// TestArrayMoveTreeStructurePreservation tests that array moves don't break tree hierarchy
func TestArrayMoveTreeStructurePreservation(t *testing.T) {
	t.Run("moves within array preserve tree structure", func(t *testing.T) {
		tree := NewTreeCRDT()
		client := core.ClientID("structure_test")
		
		// Create hierarchical structure: root → arrayParent → array → elements
		arrayParent := tree.CreateNode("arrayParent", Map, client)
		err := tree.AddEdge(tree.Root.ID, arrayParent.ID, "parent", client)
		require.NoError(t, err)
		
		arrayRoot := tree.CreateNode("structArray", Map, client)
		arrayRoot.IsArrayRoot = true
		err = tree.AddEdge(arrayParent.ID, arrayRoot.ID, "array", client)
		require.NoError(t, err)
		
		// Add elements to array
		elements := make([]*NodeCRDT, 3)
		for i := 0; i < 3; i++ {
			elem := tree.CreateNode(fmt.Sprintf("structElem_%d", i), Literal, client)
			elem.LiteralValue = fmt.Sprintf("struct_value_%d", i)
			elements[i] = elem
			
			err := tree.AddArrayElement(arrayRoot.ID, elem.ID, i, client)
			require.NoError(t, err)
		}
		
		t.Logf("INITIAL TREE STRUCTURE:")
		t.Logf("  Root → ArrayParent → Array → [elem0, elem1, elem2]")
		
		// Perform moves within array
		err = tree.MoveArrayElement(elements[0].ID, arrayRoot.ID, 2, client)
		require.NoError(t, err)
		err = tree.MoveArrayElement(elements[2].ID, arrayRoot.ID, 0, client)
		require.NoError(t, err)
		
		t.Logf("AFTER MOVES: Positions rearranged within array")
		
		// Verify tree structure preservation
		t.Logf("VERIFYING TREE STRUCTURE INTEGRITY:")
		
		// 1. Verify array parent relationships unchanged
		assert.Equal(t, tree.Root.ID, arrayParent.ParentID, "ArrayParent should still be under root")
		assert.Equal(t, arrayParent.ID, arrayRoot.ParentID, "Array should still be under arrayParent")
		
		// 2. Verify all elements still belong to array
		finalElements := tree.GetArrayElements(arrayRoot.ID)
		assert.Len(t, finalElements, 3, "Array should still have 3 elements")
		
		for _, elem := range finalElements {
			assert.Equal(t, arrayRoot.ID, elem.ParentID, "Element should still belong to array")
			assert.True(t, elem.IsArrayElement, "Element should still be marked as array element")
		}
		
		// 3. Verify no cycles in tree structure
		assert.False(t, hasCycle(tree), "Tree should not have cycles")
		
		t.Logf("SUCCESS: Tree structure preserved during array moves")
	})
	
	t.Run("prevent moves that would create tree cycles", func(t *testing.T) {
		tree := NewTreeCRDT()
		client := core.ClientID("cycle_test")
		
		// Create nested structure that could create cycles
		// Root → NodeA → ArrayB → ElementC
		nodeA := tree.CreateNode("nodeA", Map, client)
		err := tree.AddEdge(tree.Root.ID, nodeA.ID, "nodeA", client)
		require.NoError(t, err)
		
		arrayB := tree.CreateNode("arrayB", Map, client)
		arrayB.IsArrayRoot = true
		err = tree.AddEdge(nodeA.ID, arrayB.ID, "arrayB", client)
		require.NoError(t, err)
		
		elementC := tree.CreateNode("elementC", Literal, client)
		elementC.LiteralValue = "test"
		err = tree.AddArrayElement(arrayB.ID, elementC.ID, 0, client)
		require.NoError(t, err)
		
		t.Logf("INITIAL STRUCTURE: Root → NodeA → ArrayB → ElementC")
		
		// Try to create cycle: move NodeA into ArrayB (its descendant)
		t.Logf("ATTEMPTING CYCLE: Move NodeA into ArrayB (would create Root → NodeA → ArrayB → NodeA)")
		
		err = tree.MoveArrayElement(nodeA.ID, arrayB.ID, 1, client)
		
		// This should either be rejected OR handled gracefully without creating cycles
		if err != nil {
			t.Logf("Cycle prevention: Move properly REJECTED - %v", err)
		} else {
			t.Logf("Move allowed - verifying no cycles created")
			
			// If move was allowed, verify no actual cycles exist
			assert.False(t, hasCycle(tree), "No cycles should exist even if move was allowed")
		}
		
		// Verify tree integrity regardless of whether move was allowed
		assert.False(t, hasCycle(tree), "Tree should never have cycles")
		
		t.Logf("SUCCESS: Tree structure integrity maintained")
	})
}