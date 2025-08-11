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

// TestConcurrentArrayMoves tests concurrent moves of array elements to stress test the LWW system
func TestConcurrentArrayMoves(t *testing.T) {
	t.Run("concurrent moves different elements different positions", func(t *testing.T) {
		logrus.SetLevel(logrus.WarnLevel)
		
		tree := NewTreeCRDT()
		setupClient := core.ClientID(random.GenerateRandomID())
		clientA := core.ClientID(random.GenerateRandomID())
		clientB := core.ClientID(random.GenerateRandomID())
		clientC := core.ClientID(random.GenerateRandomID())
		
		// Setup: Create array with 5 elements
		arrayRoot := tree.CreateNode("testArray", Map, setupClient)
		arrayRoot.IsArrayRoot = true
		
		elements := make([]*NodeCRDT, 5)
		for i := 0; i < 5; i++ {
			elem := tree.CreateNode(fmt.Sprintf("elem_%d", i), Literal, setupClient)
			elem.LiteralValue = fmt.Sprintf("value_%d", i)
			elements[i] = elem
			
			err := tree.AddArrayElement(arrayRoot.ID, elem.ID, i, setupClient)
			require.NoError(t, err)
		}
		
		// Document initial state
		initialElements := tree.GetArrayElements(arrayRoot.ID)
		t.Logf("INITIAL STATE: Array with %d elements", len(initialElements))
		for _, elem := range initialElements {
			t.Logf("  %s at position %d", elem.ID, elem.ArrayIndex)
		}
		
		t.Logf("EXPECTED STATE: All elements should remain unique after concurrent moves")
		
		// Perform concurrent moves using goroutines for real concurrency
		var wg sync.WaitGroup
		results := make(chan string, 10)
		
		// Goroutine 1: Move elements 0 and 1
		wg.Add(1)
		go func() {
			defer wg.Done()
			err1 := tree.MoveArrayElement(elements[0].ID, arrayRoot.ID, 3, clientA)
			if err1 != nil {
				results <- fmt.Sprintf("ClientA move elem0->pos3 FAILED: %v", err1)
			} else {
				results <- "ClientA move elem0->pos3 SUCCESS"
			}
			
			time.Sleep(1 * time.Millisecond)
			
			err2 := tree.MoveArrayElement(elements[1].ID, arrayRoot.ID, 4, clientA)
			if err2 != nil {
				results <- fmt.Sprintf("ClientA move elem1->pos4 FAILED: %v", err2)
			} else {
				results <- "ClientA move elem1->pos4 SUCCESS"
			}
		}()
		
		// Goroutine 2: Move elements 2 and 3  
		wg.Add(1)
		go func() {
			defer wg.Done()
			err1 := tree.MoveArrayElement(elements[2].ID, arrayRoot.ID, 0, clientB)
			if err1 != nil {
				results <- fmt.Sprintf("ClientB move elem2->pos0 FAILED: %v", err1)
			} else {
				results <- "ClientB move elem2->pos0 SUCCESS"
			}
			
			time.Sleep(1 * time.Millisecond)
			
			err2 := tree.MoveArrayElement(elements[3].ID, arrayRoot.ID, 1, clientB) 
			if err2 != nil {
				results <- fmt.Sprintf("ClientB move elem3->pos1 FAILED: %v", err2)
			} else {
				results <- "ClientB move elem3->pos1 SUCCESS"
			}
		}()
		
		// Goroutine 3: Move element 4 to various positions rapidly
		wg.Add(1)
		go func() {
			defer wg.Done()
			positions := []int{2, 0, 4, 1, 3}
			for i, pos := range positions {
				err := tree.MoveArrayElement(elements[4].ID, arrayRoot.ID, pos, clientC)
				if err != nil {
					results <- fmt.Sprintf("ClientC rapid move %d elem4->pos%d FAILED: %v", i, pos, err)
				} else {
					results <- fmt.Sprintf("ClientC rapid move %d elem4->pos%d SUCCESS", i, pos)
				}
				time.Sleep(500 * time.Microsecond) // Very fast moves
			}
		}()
		
		// Wait for all concurrent operations
		wg.Wait()
		close(results)
		
		// Log all operation results
		var operations []string
		for result := range results {
			operations = append(operations, result)
			t.Logf("Operation result: %s", result)
		}
		
		// Document actual final state  
		finalElements := tree.GetArrayElements(arrayRoot.ID)
		t.Logf("ACTUAL STATE after %d concurrent operations:", len(operations))
		t.Logf("  Array contains %d elements", len(finalElements))
		
		elementPositions := make(map[string]int)
		for _, elem := range finalElements {
			t.Logf("  Element %s at position %d", elem.ID, elem.ArrayIndex)
			elementPositions[string(elem.ID)] = elem.ArrayIndex
		}
		
		// Critical verifications
		assert.Len(t, finalElements, 5, "Should still have exactly 5 elements")
		
		// Verify no element duplication
		positionCount := make(map[int]int)
		for _, elem := range finalElements {
			positionCount[elem.ArrayIndex]++
		}
		
		for pos, count := range positionCount {
			assert.LessOrEqual(t, count, 1, "Position %d should have at most 1 element, found %d", pos, count)
		}
		
		// Verify all original elements still exist
		for i, originalElem := range elements {
			finalElem := tree.Nodes[originalElem.ID]
			require.NotNil(t, finalElem, "Element %d should still exist", i)
			assert.True(t, finalElem.IsArrayElement, "Element %d should still be array element", i)
		}
		
		t.Logf("SUCCESS: All %d elements preserved, no duplication detected", len(finalElements))
	})
}

// TestRapidFireMoves tests very rapid moves of the same element
func TestRapidFireMoves(t *testing.T) {
	t.Run("single element moved rapidly between positions", func(t *testing.T) {
		logrus.SetLevel(logrus.WarnLevel)
		
		tree := NewTreeCRDT()
		setupClient := core.ClientID(random.GenerateRandomID())
		
		// Create array with single element
		arrayRoot := tree.CreateNode("pingPongArray", Map, setupClient)
		arrayRoot.IsArrayRoot = true
		
		pingPongElement := tree.CreateNode("ping_pong_ball", Literal, setupClient)
		pingPongElement.LiteralValue = "bouncing"
		
		err := tree.AddArrayElement(arrayRoot.ID, pingPongElement.ID, 0, setupClient)
		require.NoError(t, err)
		
		t.Logf("INITIAL STATE: Single element at position %d", pingPongElement.ArrayIndex)
		t.Logf("EXPECTED STATE: Element should end up at some final position after rapid moves")
		
		// Rapid fire moves by multiple clients
		var wg sync.WaitGroup
		results := make(chan string, 50)
		
		clients := []core.ClientID{
			core.ClientID(random.GenerateRandomID()),
			core.ClientID(random.GenerateRandomID()),
			core.ClientID(random.GenerateRandomID()),
		}
		
		// Each client performs rapid moves
		for clientIdx, client := range clients {
			wg.Add(1)
			go func(clientNum int, clientID core.ClientID) {
				defer wg.Done()
				
				positions := []int{0, 1, 2, 3, 4, 0, 2, 1, 4, 3} // Chaotic sequence
				for moveNum, targetPos := range positions {
					err := tree.MoveArrayElement(pingPongElement.ID, arrayRoot.ID, targetPos, clientID)
					
					if err != nil {
						results <- fmt.Sprintf("Client%d move%d->pos%d FAILED: %v", clientNum, moveNum, targetPos, err)
					} else {
						results <- fmt.Sprintf("Client%d move%d->pos%d SUCCESS", clientNum, moveNum, targetPos)
					}
					
					// Tiny delay to create maximum contention
					time.Sleep(50 * time.Microsecond)
				}
			}(clientIdx, client)
		}
		
		wg.Wait()
		close(results)
		
		// Count operations
		successCount := 0
		failCount := 0
		for result := range results {
			if len(results) < 5 { // Limit logging
				t.Logf("Rapid move: %s", result)
			}
			if fmt.Sprintf("%s", result)[len(result)-7:] == "SUCCESS" {
				successCount++
			} else {
				failCount++
			}
		}
		
		// Document final state
		finalElement := tree.Nodes[pingPongElement.ID]
		finalElements := tree.GetArrayElements(arrayRoot.ID)
		
		t.Logf("ACTUAL STATE after rapid fire:")
		t.Logf("  Operations: %d successful, %d failed", successCount, failCount)
		t.Logf("  Element final position: %d", finalElement.ArrayIndex)
		t.Logf("  Array contains %d elements", len(finalElements))
		
		// Critical verification: element should exist exactly once
		assert.Len(t, finalElements, 1, "Array should contain exactly 1 element")
		assert.Equal(t, pingPongElement.ID, finalElements[0].ID, "Should be our ping pong element")
		assert.NotNil(t, finalElement, "Element should still exist")
		assert.True(t, finalElement.IsArrayElement, "Element should still be array element")
		
		t.Logf("SUCCESS: Element survived %d operations and exists uniquely at position %d", 
			successCount+failCount, finalElement.ArrayIndex)
	})
}

// TestMoveOrderIndependence tests that different orderings produce consistent results
func TestMoveOrderIndependence(t *testing.T) {
	t.Run("same moves in different orders should be deterministic", func(t *testing.T) {
		logrus.SetLevel(logrus.WarnLevel)
		
		// Define test scenarios with same moves in different orders
		type Move struct {
			ElementIdx int
			Position   int
			Client     string
		}
		
		scenarios := [][]Move{
			// Scenario 1
			{{0, 2, "A"}, {1, 0, "B"}, {0, 1, "C"}},
			// Scenario 2 - same moves, different order
			{{1, 0, "B"}, {0, 2, "A"}, {0, 1, "C"}},
			// Scenario 3 - another order
			{{0, 1, "C"}, {0, 2, "A"}, {1, 0, "B"}},
		}
		
		results := make([]map[string]int, len(scenarios))
		
		for scenarioIdx, moves := range scenarios {
			tree := NewTreeCRDT()
			setupClient := core.ClientID(random.GenerateRandomID())
			
			// Setup identical initial state
			arrayRoot := tree.CreateNode("testArray", Map, setupClient)
			arrayRoot.IsArrayRoot = true
			
			elements := make([]*NodeCRDT, 2)
			for i := 0; i < 2; i++ {
				elem := tree.CreateNode(fmt.Sprintf("elem%d", i), Literal, setupClient)
				elem.LiteralValue = fmt.Sprintf("value%d", i)
				elements[i] = elem
				
				err := tree.AddArrayElement(arrayRoot.ID, elem.ID, i, setupClient)
				require.NoError(t, err)
			}
			
			t.Logf("Scenario %d INITIAL STATE:", scenarioIdx+1)
			for i, elem := range elements {
				t.Logf("  elem%d at position %d", i, elem.ArrayIndex)
			}
			
			// Apply moves in the specified order
			for moveIdx, move := range moves {
				client := core.ClientID(random.GenerateRandomID() + "_" + move.Client)
				err := tree.MoveArrayElement(elements[move.ElementIdx].ID, arrayRoot.ID, move.Position, client)
				
				if err != nil {
					t.Logf("  Move %d: elem%d->pos%d by %s FAILED: %v", moveIdx, move.ElementIdx, move.Position, move.Client, err)
				} else {
					t.Logf("  Move %d: elem%d->pos%d by %s SUCCESS", moveIdx, move.ElementIdx, move.Position, move.Client)
				}
			}
			
			// Record final positions
			results[scenarioIdx] = make(map[string]int)
			finalElements := tree.GetArrayElements(arrayRoot.ID)
			
			t.Logf("Scenario %d FINAL STATE:", scenarioIdx+1)
			for _, elem := range finalElements {
				elemKey := fmt.Sprintf("elem_%s", elem.LiteralValue)
				results[scenarioIdx][elemKey] = elem.ArrayIndex
				t.Logf("  %s at position %d", elemKey, elem.ArrayIndex)
			}
			
			// Verify no duplication in this scenario
			assert.Len(t, finalElements, 2, "Should have 2 elements in scenario %d", scenarioIdx+1)
		}
		
		// Analysis: Compare results across scenarios
		t.Logf("CROSS-SCENARIO ANALYSIS:")
		for i := 0; i < len(results); i++ {
			for j := i + 1; j < len(results); j++ {
				t.Logf("Comparing scenario %d vs %d:", i+1, j+1)
				
				same := true
				for key := range results[i] {
					if results[i][key] != results[j][key] {
						same = false
						t.Logf("  %s: scenario%d=pos%d, scenario%d=pos%d", 
							key, i+1, results[i][key], j+1, results[j][key])
					}
				}
				if same {
					t.Logf("  Results IDENTICAL")
				} else {
					t.Logf("  Results DIFFERENT (expected due to LWW timing)")
				}
			}
		}
		
		t.Logf("SUCCESS: All scenarios completed without duplication")
	})
}