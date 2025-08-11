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

// TestDistributedArrayMoves tests array moves in a distributed setting using Clone() and Merge()
func TestDistributedArrayMoves(t *testing.T) {
	t.Run("distributed concurrent moves with merge", func(t *testing.T) {
		logrus.SetLevel(logrus.WarnLevel)
		
		// Create initial tree with array
		tree1 := NewTreeCRDT()
		clientA := core.ClientID(random.GenerateRandomID())
		clientB := core.ClientID(random.GenerateRandomID())
		
		// Setup array with elements
		arrayRoot := tree1.CreateNode("distArray", Map, clientA)
		arrayRoot.IsArrayRoot = true
		
		element1 := tree1.CreateNode("elem1", Literal, clientA)
		element1.LiteralValue = "value1"
		element2 := tree1.CreateNode("elem2", Literal, clientA)
		element2.LiteralValue = "value2"
		element3 := tree1.CreateNode("elem3", Literal, clientA)
		element3.LiteralValue = "value3"
		
		err := tree1.AddArrayElement(arrayRoot.ID, element1.ID, 0, clientA)
		require.NoError(t, err)
		err = tree1.AddArrayElement(arrayRoot.ID, element2.ID, 1, clientA)
		require.NoError(t, err)
		err = tree1.AddArrayElement(arrayRoot.ID, element3.ID, 2, clientA)
		require.NoError(t, err)
		
		t.Logf("INITIAL STATE: 3 elements at positions [0, 1, 2]")
		
		// Clone tree to simulate distributed replicas
		tree2, err := tree1.Clone()
		require.NoError(t, err, "Clone should succeed")
		
		// Verify clone has array metadata
		elem2InTree2 := tree2.Nodes[element2.ID]
		require.NotNil(t, elem2InTree2)
		assert.True(t, elem2InTree2.IsArrayElement, "Cloned element should be array element")
		
		t.Logf("DISTRIBUTED SCENARIO: Two replicas performing concurrent moves")
		
		// Tree1: Move element2 to position 0
		err = tree1.MoveArrayElement(element2.ID, arrayRoot.ID, 0, clientA)
		require.NoError(t, err, "Move in tree1 should succeed")
		t.Logf("  Tree1: Moved element2 to position 0")
		
		// Tree2: Move element2 to position 2
		err = tree2.MoveArrayElement(element2.ID, arrayRoot.ID, 2, clientB)
		require.NoError(t, err, "Move in tree2 should succeed")
		t.Logf("  Tree2: Moved element2 to position 2")
		
		// Document local states
		elem2InTree1 := tree1.Nodes[element2.ID]
		elem2InTree2 = tree2.Nodes[element2.ID]
		t.Logf("LOCAL STATES before merge:")
		t.Logf("  Tree1: element2 at position %d", elem2InTree1.ArrayIndex)
		t.Logf("  Tree2: element2 at position %d", elem2InTree2.ArrayIndex)
		
		// Merge trees - simulate network sync
		t.Logf("MERGING: Simulating network synchronization")
		err = tree1.Merge(tree2)
		require.NoError(t, err, "Merge should succeed")
		err = tree2.Merge(tree1)
		require.NoError(t, err, "Reverse merge should succeed")
		
		// Verify convergence
		finalElem2Tree1 := tree1.Nodes[element2.ID]
		finalElem2Tree2 := tree2.Nodes[element2.ID]
		
		t.Logf("CONVERGED STATES after merge:")
		t.Logf("  Tree1: element2 at position %d", finalElem2Tree1.ArrayIndex)
		t.Logf("  Tree2: element2 at position %d", finalElem2Tree2.ArrayIndex)
		
		// Both trees should converge to same state
		assert.Equal(t, finalElem2Tree1.ArrayIndex, finalElem2Tree2.ArrayIndex,
			"Both trees should converge to same position for element2")
		
		// Verify array integrity
		elements1 := tree1.GetArrayElements(arrayRoot.ID)
		elements2 := tree2.GetArrayElements(arrayRoot.ID)
		
		assert.Len(t, elements1, 3, "Tree1 should have 3 elements")
		assert.Len(t, elements2, 3, "Tree2 should have 3 elements")
		
		// Verify no duplicates
		for i, elem1 := range elements1 {
			elem2 := elements2[i]
			assert.Equal(t, elem1.ID, elem2.ID, "Element %d should match", i)
			assert.Equal(t, elem1.ArrayIndex, elem2.ArrayIndex, "Element %d position should match", i)
		}
		
		// Verify trees are equal
		assert.True(t, tree1.Equal(tree2), "Trees should be equal after convergence")
		
		t.Logf("SUCCESS: Distributed array moves converged correctly with LWW resolution")
	})
	
	t.Run("three-way distributed merge", func(t *testing.T) {
		logrus.SetLevel(logrus.WarnLevel)
		
		// Create initial tree
		tree1 := NewTreeCRDT()
		setupClient := core.ClientID("setup")
		
		// Create array with 4 elements
		arrayRoot := tree1.CreateNode("threeWayArray", Map, setupClient)
		arrayRoot.IsArrayRoot = true
		
		elements := make([]*NodeCRDT, 4)
		for i := 0; i < 4; i++ {
			elem := tree1.CreateNode(fmt.Sprintf("elem%d", i), Literal, setupClient)
			elem.LiteralValue = fmt.Sprintf("value%d", i)
			elements[i] = elem
			
			err := tree1.AddArrayElement(arrayRoot.ID, elem.ID, i, setupClient)
			require.NoError(t, err)
		}
		
		t.Logf("INITIAL: 4 elements at positions [0, 1, 2, 3]")
		
		// Create 3 distributed replicas
		tree2, err := tree1.Clone()
		require.NoError(t, err)
		tree3, err := tree1.Clone()
		require.NoError(t, err)
		
		// Each replica performs different moves
		clientA := core.ClientID(random.GenerateRandomID())
		clientB := core.ClientID(random.GenerateRandomID())
		clientC := core.ClientID(random.GenerateRandomID())
		
		// Tree1: Move elem0 to position 3
		err = tree1.MoveArrayElement(elements[0].ID, arrayRoot.ID, 3, clientA)
		require.NoError(t, err)
		t.Logf("Tree1: Moved elem0 to position 3")
		
		// Tree2: Move elem1 to position 0
		err = tree2.MoveArrayElement(elements[1].ID, arrayRoot.ID, 0, clientB)
		require.NoError(t, err)
		t.Logf("Tree2: Moved elem1 to position 0")
		
		// Tree3: Move elem3 to position 1
		err = tree3.MoveArrayElement(elements[3].ID, arrayRoot.ID, 1, clientC)
		require.NoError(t, err)
		t.Logf("Tree3: Moved elem3 to position 1")
		
		// Perform three-way merge
		t.Logf("MERGING: Three-way synchronization")
		
		// Tree1 merges with Tree2
		err = tree1.Merge(tree2)
		require.NoError(t, err)
		err = tree2.Merge(tree1)
		require.NoError(t, err)
		
		// Tree1 merges with Tree3
		err = tree1.Merge(tree3)
		require.NoError(t, err)
		err = tree3.Merge(tree1)
		require.NoError(t, err)
		
		// Tree2 merges with Tree3
		err = tree2.Merge(tree3)
		require.NoError(t, err)
		err = tree3.Merge(tree2)
		require.NoError(t, err)
		
		// Verify all trees converged
		t.Logf("VERIFICATION: Checking three-way convergence")
		
		finalElements1 := tree1.GetArrayElements(arrayRoot.ID)
		finalElements2 := tree2.GetArrayElements(arrayRoot.ID)
		finalElements3 := tree3.GetArrayElements(arrayRoot.ID)
		
		assert.Len(t, finalElements1, 4, "All trees should have 4 elements")
		assert.Len(t, finalElements2, 4)
		assert.Len(t, finalElements3, 4)
		
		// Verify position consistency across all trees
		for i := 0; i < 4; i++ {
			pos1 := finalElements1[i].ArrayIndex
			pos2 := finalElements2[i].ArrayIndex
			pos3 := finalElements3[i].ArrayIndex
			
			assert.Equal(t, pos1, pos2, "Position should match between tree1 and tree2")
			assert.Equal(t, pos2, pos3, "Position should match between tree2 and tree3")
			
			t.Logf("  Element at position %d: %s (consistent across all replicas)",
				pos1, finalElements1[i].ID)
		}
		
		// Verify all trees are equal
		assert.True(t, tree1.Equal(tree2), "Tree1 and Tree2 should be equal")
		assert.True(t, tree2.Equal(tree3), "Tree2 and Tree3 should be equal")
		
		t.Logf("SUCCESS: Three-way distributed merge converged correctly")
	})
}