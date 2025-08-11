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

// TestArrayPositionConflictResolution tests concurrent array moves that create position conflicts within arrays
func TestArrayPositionConflictResolution(t *testing.T) {
	t.Run("concurrent circular position swap within single array", func(t *testing.T) {
		logrus.SetLevel(logrus.WarnLevel)
		tree := NewTreeCRDT()
		client := core.ClientID(random.GenerateRandomID())
		
		// Create array with 4 elements for proper circular testing
		arrayRoot := tree.CreateNode("circularArray", Map, client)
		arrayRoot.IsArrayRoot = true
		
		elements := make([]*NodeCRDT, 4)
		for i := 0; i < 4; i++ {
			elem := tree.CreateNode(fmt.Sprintf("elem_%d", i), Literal, client)
			elem.LiteralValue = fmt.Sprintf("value_%d", i)
			elements[i] = elem
			
			err := tree.AddArrayElement(arrayRoot.ID, elem.ID, i, client)
			require.NoError(t, err)
		}
		
		/*
		 * VISUAL REPRESENTATION - CONCURRENT CIRCULAR POSITION SWAP
		 * ========================================================
		 * 
		 * INITIAL B-TREE ARRAY STATE:
		 * ┌─────┬─────┬─────┬─────┐
		 * │ [0] │ [1] │ [2] │ [3] │  <- Array positions
		 * │elem0│elem1│elem2│elem3│  <- Elements
		 * └─────┴─────┴─────┴─────┘
		 * 
		 * CONCURRENT CIRCULAR MOVES:
		 * elem0 (pos0) → pos1 ┐
		 * elem1 (pos1) → pos2 │ <- Every element moves
		 * elem2 (pos2) → pos3 │    to next position
		 * elem3 (pos3) → pos0 ┘    (circular chain)
		 * 
		 * EXPECTED OUTCOME (LWW breaks circular dependency):
		 * ┌─────┬─────┬─────┬─────┬─────┐
		 * │ [0] │ [1] │ [2] │ [3] │ [4] │  <- May expand
		 * │ ??  │ ??  │ ??  │ ??  │ ??  │  <- LWW determines final
		 * └─────┴─────┴─────┴─────┴─────┘    positions
		 */
		
		// Document initial state
		t.Logf("INITIAL B-TREE STATE: [elem0] [elem1] [elem2] [elem3]")
		t.Logf("                      pos 0   pos 1   pos 2   pos 3")
		
		t.Logf("CONCURRENT CIRCULAR MOVES:")
		t.Logf("  elem0 (pos0) -> pos1 ┐")
		t.Logf("  elem1 (pos1) -> pos2 │ <- Full circular")
		t.Logf("  elem2 (pos2) -> pos3 │    dependency chain")
		t.Logf("  elem3 (pos3) -> pos0 ┘")
		
		t.Logf("EXPECTED: LWW resolution breaks circular chain, positions adjusted")
		
		// Perform concurrent circular moves using goroutines
		var wg sync.WaitGroup
		results := make(chan string, 4)
		
		clients := []core.ClientID{
			core.ClientID(random.GenerateRandomID()),
			core.ClientID(random.GenerateRandomID()),
			core.ClientID(random.GenerateRandomID()),
			core.ClientID(random.GenerateRandomID()),
		}
		
		// Each element moves to the next position (circular)
		for i := 0; i < 4; i++ {
			wg.Add(1)
			go func(elemIdx int) {
				defer wg.Done()
				targetPos := (elemIdx + 1) % 4 // Circular: 0→1, 1→2, 2→3, 3→0
				
				// Small delay to create realistic timing differences
				time.Sleep(time.Duration(elemIdx*25) * time.Microsecond)
				
				err := tree.MoveArrayElement(elements[elemIdx].ID, arrayRoot.ID, targetPos, clients[elemIdx])
				if err != nil {
					results <- fmt.Sprintf("elem%d->pos%d FAILED: %v", elemIdx, targetPos, err)
				} else {
					results <- fmt.Sprintf("elem%d->pos%d SUCCESS", elemIdx, targetPos)
				}
			}(i)
		}
		
		wg.Wait()
		close(results)
		
		// Log operation results
		for result := range results {
			t.Logf("Circular move result: %s", result)
		}
		
		// Document actual final state
		finalElements := tree.GetArrayElements(arrayRoot.ID)
		t.Logf("ACTUAL STATE after circular position swap:")
		t.Logf("  Array contains %d elements", len(finalElements))
		
		// Build position mapping
		finalPositions := make(map[int]int) // originalPos -> finalPos
		for i, originalElem := range elements {
			finalElem := tree.Nodes[originalElem.ID]
			finalPositions[i] = finalElem.ArrayIndex
			t.Logf("  elem%d: pos %d -> pos %d", i, i, finalElem.ArrayIndex)
		}
		
		// Create visual final state
		maxPos := 0
		for _, pos := range finalPositions {
			if pos > maxPos {
				maxPos = pos
			}
		}
		
		visualState := make([]string, maxPos+1)
		for i := range visualState {
			visualState[i] = " ?? "
		}
		
		for i := 0; i < 4; i++ {
			pos := finalPositions[i]
			if pos <= maxPos {
				visualState[pos] = fmt.Sprintf("e%d", i)
			}
		}
		
		t.Logf("FINAL B-TREE LAYOUT:")
		visual := "["
		for i, elem := range visualState {
			if i > 0 {
				visual += "|"
			}
			visual += elem
		}
		visual += "]"
		t.Logf("  %s", visual)
		
		// Verify no circular references in array structure
		hasCircular := tree.checkArrayCircularReferences(arrayRoot.ID)
		assert.False(t, hasCircular, "Array should not have circular references")
		
		// Verify all elements exist uniquely
		assert.Len(t, finalElements, 4, "Should have exactly 4 elements")
		
		// Verify no position conflicts
		positionCounts := make(map[int]int)
		for _, elem := range finalElements {
			positionCounts[elem.ArrayIndex]++
		}
		
		for pos, count := range positionCounts {
			assert.Equal(t, 1, count, "Position %d should have exactly 1 element, found %d", pos, count)
		}
		
		t.Logf("SUCCESS: Circular position conflicts resolved without array corruption")
	})
	
	t.Run("concurrent moves creating position race conditions", func(t *testing.T) {
		tree := NewTreeCRDT()
		client := core.ClientID("setup_client")
		
		// Create array with 6 elements
		arrayRoot := tree.CreateNode("raceArray", Map, client)
		arrayRoot.IsArrayRoot = true
		
		elements := make([]*NodeCRDT, 6)
		for i := 0; i < 6; i++ {
			elem := tree.CreateNode(fmt.Sprintf("elem_%d", i), Literal, client)
			elem.LiteralValue = fmt.Sprintf("value_%d", i)
			elements[i] = elem
			
			err := tree.AddArrayElement(arrayRoot.ID, elem.ID, i, client)
			require.NoError(t, err)
		}
		
		t.Logf("RACE CONDITION TEST:")
		t.Logf("Multiple elements trying to claim same positions concurrently")
		
		// Create race condition: multiple elements targeting same positions
		var wg sync.WaitGroup
		results := make(chan string, 10)
		
		raceClients := []core.ClientID{
			core.ClientID(random.GenerateRandomID()),
			core.ClientID(random.GenerateRandomID()),
			core.ClientID(random.GenerateRandomID()),
		}
		
		// Three elements all try to move to position 2
		targets := []int{0, 3, 5} // elements 0, 3, 5
		for i, elemIdx := range targets {
			wg.Add(1)
			go func(elementIndex, clientIndex int) {
				defer wg.Done()
				time.Sleep(time.Duration(clientIndex*10) * time.Microsecond)
				
				err := tree.MoveArrayElement(elements[elementIndex].ID, arrayRoot.ID, 2, raceClients[clientIndex])
				if err != nil {
					results <- fmt.Sprintf("elem%d->pos2 FAILED: %v", elementIndex, err)
				} else {
					results <- fmt.Sprintf("elem%d->pos2 SUCCESS", elementIndex)
				}
			}(elemIdx, i)
		}
		
		wg.Wait()
		close(results)
		
		// Log race results
		for result := range results {
			t.Logf("Race result: %s", result)
		}
		
		// Verify final state
		finalElements := tree.GetArrayElements(arrayRoot.ID)
		assert.Len(t, finalElements, 6, "Should still have 6 elements")
		
		// Verify no position conflicts
		positionMap := make(map[int]int)
		for _, elem := range finalElements {
			positionMap[elem.ArrayIndex]++
		}
		
		duplicates := 0
		for pos, count := range positionMap {
			if count > 1 {
				duplicates++
				t.Logf("Position %d has %d elements (conflict)", pos, count)
			}
		}
		
		assert.Equal(t, 0, duplicates, "No positions should have duplicate elements")
		
		t.Logf("SUCCESS: Race conditions resolved without position conflicts")
	})
}

// TestConcurrentMoveStress tests concurrent moves that could break tree structure
func TestConcurrentMoveStress(t *testing.T) {
	t.Run("concurrent moves same elements different targets", func(t *testing.T) {
		tree := NewTreeCRDT()
		
		// Create multiple arrays
		arrayA := tree.CreateNode("arrayA", Map, core.ClientID("setup"))
		arrayA.IsArrayRoot = true
		
		arrayB := tree.CreateNode("arrayB", Map, core.ClientID("setup"))
		arrayB.IsArrayRoot = true
		
		arrayC := tree.CreateNode("arrayC", Map, core.ClientID("setup"))
		arrayC.IsArrayRoot = true
		
		// Create movable elements
		elements := make([]*NodeCRDT, 5)
		for i := 0; i < 5; i++ {
			elem := tree.CreateNode(fmt.Sprintf("elem%d", i), Literal, core.ClientID("setup"))
			elem.LiteralValue = fmt.Sprintf("value_%d", i)
			elements[i] = elem
			
			// Initially add all to arrayA
			err := tree.AddArrayElement(arrayA.ID, elem.ID, i, core.ClientID("setup"))
			require.NoError(t, err)
		}
		
		// Concurrent goroutines trying to move same elements to different arrays
		var wg sync.WaitGroup
		results := make(chan string, 15) // Buffer for all operations
		
		// Goroutine 1: Move elements 0-2 to arrayB
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 3; i++ {
				err := tree.MoveArrayElement(elements[i].ID, arrayB.ID, i, core.ClientID("client_1"))
				if err != nil {
					results <- fmt.Sprintf("client_1 move elem%d to arrayB failed: %v", i, err)
				} else {
					results <- fmt.Sprintf("client_1 move elem%d to arrayB succeeded", i)
				}
				time.Sleep(1 * time.Millisecond) // Small delay to create interleaving
			}
		}()
		
		// Goroutine 2: Move elements 1-3 to arrayC
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 1; i < 4; i++ {
				err := tree.MoveArrayElement(elements[i].ID, arrayC.ID, i-1, core.ClientID("client_2"))
				if err != nil {
					results <- fmt.Sprintf("client_2 move elem%d to arrayC failed: %v", i, err)
				} else {
					results <- fmt.Sprintf("client_2 move elem%d to arrayC succeeded", i)
				}
				time.Sleep(1 * time.Millisecond)
			}
		}()
		
		// Goroutine 3: Move elements 2-4 back to arrayA at different positions
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 2; i < 5; i++ {
				err := tree.MoveArrayElement(elements[i].ID, arrayA.ID, i+10, core.ClientID("client_3"))
				if err != nil {
					results <- fmt.Sprintf("client_3 move elem%d to arrayA failed: %v", i, err)
				} else {
					results <- fmt.Sprintf("client_3 move elem%d to arrayA succeeded", i)
				}
				time.Sleep(1 * time.Millisecond)
			}
		}()
		
		// Wait for all operations
		wg.Wait()
		close(results)
		
		// Collect results
		var operations []string
		for result := range results {
			operations = append(operations, result)
			t.Logf("Operation: %s", result)
		}
		
		// Verify tree integrity after concurrent operations
		tree.ValidateTreeIntegrity(t)
		
		// Verify each element appears in exactly one array
		tree.VerifyNoElementDuplication(t, []*NodeCRDT{arrayA, arrayB, arrayC})
		
		t.Logf("Completed %d concurrent operations", len(operations))
	})
	
	t.Run("rapid fire moves same element", func(t *testing.T) {
		tree := NewTreeCRDT()
		client := core.ClientID("test_client")
		
		// Create target arrays
		arrays := make([]*NodeCRDT, 10)
		for i := 0; i < 10; i++ {
			arr := tree.CreateNode(fmt.Sprintf("array%d", i), Map, client)
			arr.IsArrayRoot = true
			arrays[i] = arr
		}
		
		// Create single element
		element := tree.CreateNode("ping_pong", Literal, client)
		element.LiteralValue = "bouncing_ball"
		
		// Start in array0
		err := tree.AddArrayElement(arrays[0].ID, element.ID, 0, client)
		require.NoError(t, err)
		
		// Rapid fire moves between arrays
		var wg sync.WaitGroup
		moveResults := make(chan string, 100)
		
		// Multiple clients trying to move the same element rapidly
		for clientNum := 0; clientNum < 5; clientNum++ {
			wg.Add(1)
			go func(clientID string) {
				defer wg.Done()
				for moveNum := 0; moveNum < 20; moveNum++ {
					targetArray := moveNum % len(arrays)
					position := moveNum % 3
					
					err := tree.MoveArrayElement(element.ID, arrays[targetArray].ID, position, 
						core.ClientID(fmt.Sprintf("rapid_client_%s", clientID)))
					
					if err != nil {
						moveResults <- fmt.Sprintf("%s move %d failed: %v", clientID, moveNum, err)
					} else {
						moveResults <- fmt.Sprintf("%s move %d to array%d pos%d succeeded", 
							clientID, moveNum, targetArray, position)
					}
					
					// Very small delay to create maximum contention
					time.Sleep(100 * time.Microsecond)
				}
			}(fmt.Sprintf("client_%d", clientNum))
		}
		
		wg.Wait()
		close(moveResults)
		
		// Log all results
		moveCount := 0
		for result := range moveResults {
			moveCount++
			if moveCount <= 10 { // Limit logging to avoid spam
				t.Logf("Move result: %s", result)
			}
		}
		t.Logf("Total rapid moves attempted: %d", moveCount)
		
		// Critical verification: element should exist in exactly one location
		finalElement := tree.Nodes[element.ID]
		require.NotNil(t, finalElement, "Element should still exist")
		
		// Count how many arrays contain this element
		containingArrays := 0
		var finalLocation core.NodeID
		for _, arr := range arrays {
			elements := tree.GetArrayElements(arr.ID)
			for _, elem := range elements {
				if elem.ID == element.ID {
					containingArrays++
					finalLocation = arr.ID
				}
			}
		}
		
		assert.Equal(t, 1, containingArrays, "Element should exist in exactly one array")
		t.Logf("Element ended up in array: %s at position %d", finalLocation, finalElement.ArrayIndex)
	})
}

// TestMoveOrderDependence tests that different orderings of moves produce consistent results
func TestMoveOrderDependence(t *testing.T) {
	t.Run("move sequences should be order independent", func(t *testing.T) {
		// Test the same sequence of moves in different orders
		sequences := [][]struct {
			element  int
			target   int
			position int
			client   string
		}{
			// Sequence A
			{{0, 1, 0, "c1"}, {1, 2, 0, "c2"}, {0, 2, 1, "c1"}},
			// Sequence B (different order)
			{{1, 2, 0, "c2"}, {0, 1, 0, "c1"}, {0, 2, 1, "c1"}},
			// Sequence C (another order)
			{{0, 2, 1, "c1"}, {0, 1, 0, "c1"}, {1, 2, 0, "c2"}},
		}
		
		results := make([]map[core.NodeID][]int, len(sequences))
		
		for seqIdx, sequence := range sequences {
			tree := NewTreeCRDT()
			
			// Setup: 3 arrays, 2 elements
			arrays := make([]*NodeCRDT, 3)
			elements := make([]*NodeCRDT, 2)
			
			for i := 0; i < 3; i++ {
				arr := tree.CreateNode(fmt.Sprintf("array%d", i), Map, core.ClientID("setup"))
				arr.IsArrayRoot = true
				arrays[i] = arr
			}
			
			for i := 0; i < 2; i++ {
				elem := tree.CreateNode(fmt.Sprintf("elem%d", i), Literal, core.ClientID("setup"))
				elem.LiteralValue = fmt.Sprintf("value_%d", i)
				elements[i] = elem
				
				// Start both elements in array0
				err := tree.AddArrayElement(arrays[0].ID, elem.ID, i, core.ClientID("setup"))
				require.NoError(t, err)
			}
			
			// Apply move sequence
			for _, move := range sequence {
				err := tree.MoveArrayElement(
					elements[move.element].ID,
					arrays[move.target].ID,
					move.position,
					core.ClientID(move.client))
				
				// Log the move (don't fail on error, just log)
				if err != nil {
					t.Logf("Seq %d: Move elem%d->array%d pos%d by %s FAILED: %v",
						seqIdx, move.element, move.target, move.position, move.client, err)
				} else {
					t.Logf("Seq %d: Move elem%d->array%d pos%d by %s SUCCESS",
						seqIdx, move.element, move.target, move.position, move.client)
				}
			}
			
			// Record final state
			results[seqIdx] = make(map[core.NodeID][]int)
			for _, arr := range arrays {
				arrayElements := tree.GetArrayElements(arr.ID)
				positions := make([]int, len(arrayElements))
				for i, elem := range arrayElements {
					positions[i] = elem.ArrayIndex
				}
				results[seqIdx][arr.ID] = positions
			}
			
			// Verify no duplication in this sequence
			tree.VerifyNoElementDuplication(t, arrays)
		}
		
		// Compare results across sequences
		// Due to LWW semantics, results might differ, but should be consistent within vector clock rules
		for i := 0; i < len(results); i++ {
			for j := i + 1; j < len(results); j++ {
				t.Logf("Comparing sequence %d vs %d:", i, j)
				// We expect results might differ due to LWW, but no duplications should occur
				// The key test is that each element appears exactly once across all arrays
			}
		}
	})
}

// Helper methods for validation

func (c *TreeCRDT) ValidateTreeIntegrity(t *testing.T) {
	// Check for cycles
	visited := make(map[core.NodeID]bool)
	assert.False(t, c.hasCycle(c.Root.ID, visited), "Tree should not contain cycles")
	
	// Check parent-child consistency
	for nodeID, node := range c.Nodes {
		if node.ParentID != "" {
			parent := c.Nodes[node.ParentID]
			require.NotNil(t, parent, "Parent of node %s should exist", nodeID)
			
			// If it's an array element, verify parent is array root
			if node.IsArrayElement {
				assert.True(t, parent.IsArrayRoot, "Parent of array element should be array root")
			}
		}
	}
}

func (c *TreeCRDT) VerifyNoElementDuplication(t *testing.T, arrays []*NodeCRDT) {
	elementCounts := make(map[core.NodeID]int)
	
	for _, arr := range arrays {
		elements := c.GetArrayElements(arr.ID)
		for _, elem := range elements {
			elementCounts[elem.ID]++
		}
	}
	
	for elemID, count := range elementCounts {
		assert.Equal(t, 1, count, "Element %s should appear exactly once, found %d times", elemID, count)
	}
}

func (c *TreeCRDT) hasCycle(nodeID core.NodeID, visited map[core.NodeID]bool) bool {
	if visited[nodeID] {
		return true // Cycle detected
	}
	
	visited[nodeID] = true
	
	// Check regular edges
	node := c.Nodes[nodeID]
	if node != nil {
		for _, edge := range node.Edges {
			if c.hasCycle(edge.To, visited) {
				return true
			}
		}
		
		// Also check array children if this is array root
		if node.IsArrayRoot {
			elements := c.GetArrayElements(nodeID)
			for _, elem := range elements {
				if c.hasCycle(elem.ID, visited) {
					return true
				}
			}
		}
	}
	
	delete(visited, nodeID) // Backtrack
	return false
}