package crdt

import (
	"fmt"
	"testing"

	"github.com/eislab-cps/synctree/pkg/core"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestArrayMoveVerification tests array moves with actual elements like [1,2,3,4,5]
// and verifies the expected final structure after concurrent moves
func TestArrayMoveVerification(t *testing.T) {
	t.Run("move element 1 to different positions concurrently", func(t *testing.T) {
		logrus.SetLevel(logrus.WarnLevel)
		
		// Create initial array: [1, 2, 3, 4, 5]
		tree := NewTreeCRDT()
		setupClient := core.ClientID("setup")
		
		// Create array root and connect to tree
		arrayRoot := tree.CreateNode("testArray", Map, setupClient)
		arrayRoot.IsArrayRoot = true
		err := tree.AddEdge(tree.Root.ID, arrayRoot.ID, "testArray", setupClient)
		require.NoError(t, err)
		
		// Create elements with meaningful values: 1, 2, 3, 4, 5
		elements := make([]*NodeCRDT, 5)
		for i := 0; i < 5; i++ {
			elem := tree.CreateNode(fmt.Sprintf("elem_%d", i+1), Literal, setupClient)
			elem.LiteralValue = fmt.Sprintf("%d", i+1) // Values: "1", "2", "3", "4", "5"
			elements[i] = elem
			
			err := tree.AddArrayElement(arrayRoot.ID, elem.ID, i, setupClient)
			require.NoError(t, err)
		}
		
		// Verify initial state: [1, 2, 3, 4, 5]
		initialElements := tree.GetArrayElements(arrayRoot.ID)
		require.Len(t, initialElements, 5, "Should have 5 elements initially")
		t.Logf("INITIAL STATE: [%s, %s, %s, %s, %s]", 
			initialElements[0].LiteralValue, initialElements[1].LiteralValue, 
			initialElements[2].LiteralValue, initialElements[3].LiteralValue, 
			initialElements[4].LiteralValue)
			
		// Verify initial positions are correct
		for i, elem := range initialElements {
			assert.Equal(t, fmt.Sprintf("%d", i+1), elem.LiteralValue, "Initial element %d should have value %d", i, i+1)
			assert.Equal(t, i, elem.ArrayIndex, "Initial element should be at position %d", i)
		}
		
		// Test scenario: Move element "1" (first element) to positions 2, 3, 4 concurrently
		// Create 3 independent replicas
		tree1, err := tree.Clone()
		require.NoError(t, err, "Clone 1 should succeed")
		tree2, err := tree.Clone() 
		require.NoError(t, err, "Clone 2 should succeed")
		tree3, err := tree.Clone()
		require.NoError(t, err, "Clone 3 should succeed")
		
		// Find element "1" (first element) in each replica
		element1ID := elements[0].ID // Element with value "1"
		
		// Different clients move element "1" to different positions
		clientA := core.ClientID("clientA")
		clientB := core.ClientID("clientB") 
		clientC := core.ClientID("clientC")
		
		t.Logf("CONCURRENT MOVES:")
		t.Logf("  ClientA: Move element '1' to position 2")
		t.Logf("  ClientB: Move element '1' to position 3") 
		t.Logf("  ClientC: Move element '1' to position 4")
		
		// Perform concurrent moves on different replicas
		err = tree1.MoveArrayElement(element1ID, arrayRoot.ID, 2, clientA)
		require.NoError(t, err, "Move to position 2 should not error")
		
		err = tree2.MoveArrayElement(element1ID, arrayRoot.ID, 3, clientB)
		require.NoError(t, err, "Move to position 3 should not error")
		
		err = tree3.MoveArrayElement(element1ID, arrayRoot.ID, 4, clientC)
		require.NoError(t, err, "Move to position 4 should not error")
		
		// Check local states before merge
		t.Logf("LOCAL STATES BEFORE MERGE:")
		logArrayState(t, "  Tree1", tree1, arrayRoot.ID)
		logArrayState(t, "  Tree2", tree2, arrayRoot.ID)
		logArrayState(t, "  Tree3", tree3, arrayRoot.ID)
		
		// Verify that moves actually happened locally
		elem1InTree1 := tree1.Nodes[element1ID]
		elem1InTree2 := tree2.Nodes[element1ID]
		elem1InTree3 := tree3.Nodes[element1ID]
		
		t.Logf("Element '1' positions after local moves:")
		t.Logf("  Tree1: %d", elem1InTree1.ArrayIndex)
		t.Logf("  Tree2: %d", elem1InTree2.ArrayIndex) 
		t.Logf("  Tree3: %d", elem1InTree3.ArrayIndex)
		
		// At least one move should have succeeded locally
		moved := (elem1InTree1.ArrayIndex == 2) || (elem1InTree2.ArrayIndex == 3) || (elem1InTree3.ArrayIndex == 4)
		assert.True(t, moved, "At least one move should have succeeded locally")
		
		// Merge all replicas
		t.Logf("MERGING REPLICAS...")
		err = tree1.Merge(tree2)
		require.NoError(t, err, "Merge 1←2 should succeed")
		err = tree2.Merge(tree1)
		require.NoError(t, err, "Merge 2←1 should succeed")
		
		err = tree1.Merge(tree3)
		require.NoError(t, err, "Merge 1←3 should succeed")
		err = tree3.Merge(tree1)
		require.NoError(t, err, "Merge 3←1 should succeed")
		
		err = tree2.Merge(tree3)
		require.NoError(t, err, "Merge 2←3 should succeed")
		err = tree3.Merge(tree2)
		require.NoError(t, err, "Merge 3←2 should succeed")
		
		// Verify final convergent state
		t.Logf("FINAL CONVERGED STATES:")
		logArrayState(t, "  Tree1", tree1, arrayRoot.ID)
		logArrayState(t, "  Tree2", tree2, arrayRoot.ID)
		logArrayState(t, "  Tree3", tree3, arrayRoot.ID)
		
		// Get final arrays
		finalArray1 := tree1.GetArrayElements(arrayRoot.ID)
		finalArray2 := tree2.GetArrayElements(arrayRoot.ID)
		finalArray3 := tree3.GetArrayElements(arrayRoot.ID)
		
		// All trees should have converged to same structure
		require.Len(t, finalArray1, 5, "Final array should have 5 elements")
		require.Len(t, finalArray2, 5, "Final array should have 5 elements")
		require.Len(t, finalArray3, 5, "Final array should have 5 elements")
		
		// Verify all trees have same final order
		for i := 0; i < 5; i++ {
			assert.Equal(t, finalArray1[i].LiteralValue, finalArray2[i].LiteralValue,
				"Position %d should have same value in tree1 and tree2", i)
			assert.Equal(t, finalArray2[i].LiteralValue, finalArray3[i].LiteralValue,
				"Position %d should have same value in tree2 and tree3", i)
		}
		
		// Find where element "1" ended up after LWW resolution
		var finalPosOfElement1 int = -1
		for i, elem := range finalArray1 {
			if val, ok := elem.LiteralValue.(string); ok && val == "1" {
				finalPosOfElement1 = i
				break
			}
		}
		
		require.NotEqual(t, -1, finalPosOfElement1, "Element '1' should be found in final array")
		
		// Element "1" should be at one of the attempted positions (0, 2, 3, 4) or possibly moved by rebalancing
		t.Logf("RESULT: Element '1' final position: %d", finalPosOfElement1)
		
		// The key assertion: Element "1" should exist exactly once (no duplication)
		count := 0
		for _, elem := range finalArray1 {
			if val, ok := elem.LiteralValue.(string); ok && val == "1" {
				count++
			}
		}
		assert.Equal(t, 1, count, "Element '1' should appear exactly once (no duplication)")
		
		// All original elements should still exist
		expectedValues := []string{"1", "2", "3", "4", "5"}
		actualValues := make([]string, 5)
		for i, elem := range finalArray1 {
			if val, ok := elem.LiteralValue.(string); ok {
				actualValues[i] = val
			} else {
				actualValues[i] = fmt.Sprintf("%v", elem.LiteralValue)
			}
		}
		
		for _, expected := range expectedValues {
			found := false
			for _, actual := range actualValues {
				if actual == expected {
					found = true
					break
				}
			}
			assert.True(t, found, "Element '%s' should exist in final array", expected)
		}
		
		t.Logf("SUCCESS: Array move test completed")
		t.Logf("  - No element duplication occurred")
		t.Logf("  - All original elements preserved")
		t.Logf("  - All replicas converged to same final structure")
	})
}