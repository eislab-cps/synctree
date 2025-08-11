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

// TestArrayMetadataSerialization tests that array metadata is properly saved and loaded
func TestArrayMetadataSerialization(t *testing.T) {
	t.Run("save and load array with elements", func(t *testing.T) {
		logrus.SetLevel(logrus.WarnLevel)
		
		// Create tree with array structure
		tree1 := NewTreeCRDT()
		client := core.ClientID(random.GenerateRandomID())
		
		// Create array root
		arrayRoot := tree1.CreateNode("testArray", Map, client)
		arrayRoot.IsArrayRoot = true
		err := tree1.AddEdge(tree1.Root.ID, arrayRoot.ID, "array", client)
		require.NoError(t, err)
		
		// Add elements to array
		elements := make([]*NodeCRDT, 5)
		for i := 0; i < 5; i++ {
			elem := tree1.CreateNode(fmt.Sprintf("elem_%d", i), Literal, client)
			elem.LiteralValue = fmt.Sprintf("value_%d", i)
			elements[i] = elem
			
			err := tree1.AddArrayElement(arrayRoot.ID, elem.ID, i, client)
			require.NoError(t, err)
		}
		
		// Verify initial state
		t.Logf("INITIAL STATE: Array with %d elements", len(elements))
		for i, elem := range elements {
			assert.True(t, elem.IsArrayElement, "Element %d should be marked as array element", i)
			assert.Equal(t, i, elem.ArrayIndex, "Element %d should have correct index", i)
			assert.NotEmpty(t, elem.BTreeKey, "Element %d should have B-tree key", i)
		}
		assert.True(t, arrayRoot.IsArrayRoot, "Array root should be marked as array root")
		
		// Save the tree
		data, err := tree1.Save()
		require.NoError(t, err, "Save should succeed")
		t.Logf("Saved tree data size: %d bytes", len(data))
		
		// Load into new tree
		tree2 := NewTreeCRDT()
		err = tree2.Load(data)
		require.NoError(t, err, "Load should succeed")
		
		// Verify loaded tree has correct array metadata
		t.Logf("LOADED STATE: Verifying array metadata preservation")
		
		// Check array root
		loadedArrayRoot := tree2.Nodes[arrayRoot.ID]
		require.NotNil(t, loadedArrayRoot, "Array root should exist in loaded tree")
		assert.True(t, loadedArrayRoot.IsArrayRoot, "Loaded array root should be marked as array root")
		
		// Check array elements
		loadedElements := tree2.GetArrayElements(arrayRoot.ID)
		assert.Len(t, loadedElements, 5, "Should have 5 elements after load")
		
		for i, origElem := range elements {
			loadedElem := tree2.Nodes[origElem.ID]
			require.NotNil(t, loadedElem, "Element %d should exist in loaded tree", i)
			
			// Verify array metadata preserved
			assert.True(t, loadedElem.IsArrayElement, "Loaded element %d should be marked as array element", i)
			assert.Equal(t, origElem.ArrayIndex, loadedElem.ArrayIndex, "Loaded element %d should have same array index", i)
			assert.Equal(t, origElem.BTreeKey, loadedElem.BTreeKey, "Loaded element %d should have same B-tree key", i)
			assert.Equal(t, arrayRoot.ID, loadedElem.ParentID, "Loaded element %d should have correct parent", i)
			
			t.Logf("  Element %d: IsArrayElement=%v, ArrayIndex=%d, BTreeKey=%s",
				i, loadedElem.IsArrayElement, loadedElem.ArrayIndex, loadedElem.BTreeKey)
		}
		
		// Verify trees are equal
		assert.True(t, tree1.Equal(tree2), "Original and loaded trees should be equal")
		
		t.Logf("SUCCESS: Array metadata properly serialized and deserialized")
	})
	
	t.Run("save and load after array moves", func(t *testing.T) {
		logrus.SetLevel(logrus.WarnLevel)
		
		tree1 := NewTreeCRDT()
		client := core.ClientID(random.GenerateRandomID())
		
		// Create array with elements
		arrayRoot := tree1.CreateNode("moveTestArray", Map, client)
		arrayRoot.IsArrayRoot = true
		
		elements := make([]*NodeCRDT, 3)
		for i := 0; i < 3; i++ {
			elem := tree1.CreateNode(fmt.Sprintf("moveElem_%d", i), Literal, client)
			elem.LiteralValue = fmt.Sprintf("moveValue_%d", i)
			elements[i] = elem
			
			err := tree1.AddArrayElement(arrayRoot.ID, elem.ID, i, client)
			require.NoError(t, err)
		}
		
		t.Logf("INITIAL: Elements at positions [0, 1, 2]")
		
		// Perform moves
		err := tree1.MoveArrayElement(elements[0].ID, arrayRoot.ID, 2, client)
		require.NoError(t, err)
		err = tree1.MoveArrayElement(elements[2].ID, arrayRoot.ID, 0, client)
		require.NoError(t, err)
		
		t.Logf("AFTER MOVES: Elements rearranged")
		for i, elem := range elements {
			t.Logf("  Original elem%d now at position %d", i, elem.ArrayIndex)
		}
		
		// Save after moves
		data, err := tree1.Save()
		require.NoError(t, err)
		
		// Load into new tree
		tree2 := NewTreeCRDT()
		err = tree2.Load(data)
		require.NoError(t, err)
		
		// Verify moved positions are preserved
		for i, origElem := range elements {
			loadedElem := tree2.Nodes[origElem.ID]
			require.NotNil(t, loadedElem)
			assert.Equal(t, origElem.ArrayIndex, loadedElem.ArrayIndex, 
				"Element %d position should be preserved after save/load", i)
		}
		
		// Verify array still functions correctly after load
		loadedElements := tree2.GetArrayElements(arrayRoot.ID)
		assert.Len(t, loadedElements, 3, "Should have 3 elements after load")
		
		// Verify no position conflicts
		positionMap := make(map[int]int)
		for _, elem := range loadedElements {
			positionMap[elem.ArrayIndex]++
		}
		
		for pos, count := range positionMap {
			assert.Equal(t, 1, count, "Position %d should have exactly 1 element", pos)
		}
		
		t.Logf("SUCCESS: Array moves preserved through serialization")
	})
	
	t.Run("clone preserves array metadata", func(t *testing.T) {
		logrus.SetLevel(logrus.WarnLevel)
		
		tree1 := NewTreeCRDT()
		client := core.ClientID(random.GenerateRandomID())
		
		// Create array structure
		arrayRoot := tree1.CreateNode("cloneArray", Map, client)
		arrayRoot.IsArrayRoot = true
		
		element := tree1.CreateNode("cloneElem", Literal, client)
		element.LiteralValue = "clone_value"
		
		err := tree1.AddArrayElement(arrayRoot.ID, element.ID, 0, client)
		require.NoError(t, err)
		
		t.Logf("ORIGINAL: Array root IsArrayRoot=%v, Element IsArrayElement=%v",
			arrayRoot.IsArrayRoot, element.IsArrayElement)
		
		// Clone the tree
		tree2, err := tree1.Clone()
		require.NoError(t, err, "Clone should succeed")
		
		// Verify cloned tree has array metadata
		clonedArrayRoot := tree2.Nodes[arrayRoot.ID]
		require.NotNil(t, clonedArrayRoot)
		assert.True(t, clonedArrayRoot.IsArrayRoot, "Cloned array root should be marked as array root")
		
		clonedElement := tree2.Nodes[element.ID]
		require.NotNil(t, clonedElement)
		assert.True(t, clonedElement.IsArrayElement, "Cloned element should be marked as array element")
		assert.Equal(t, element.ArrayIndex, clonedElement.ArrayIndex, "Cloned element should have same index")
		assert.Equal(t, element.BTreeKey, clonedElement.BTreeKey, "Cloned element should have same B-tree key")
		
		t.Logf("CLONED: Array root IsArrayRoot=%v, Element IsArrayElement=%v",
			clonedArrayRoot.IsArrayRoot, clonedElement.IsArrayElement)
		
		// Verify cloned tree functions correctly
		clonedElements := tree2.GetArrayElements(arrayRoot.ID)
		assert.Len(t, clonedElements, 1, "Cloned array should have 1 element")
		
		// Test that moves work on cloned tree
		newClient := core.ClientID(random.GenerateRandomID())
		err = tree2.MoveArrayElement(element.ID, arrayRoot.ID, 1, newClient)
		require.NoError(t, err, "Move should work on cloned tree")
		
		assert.Equal(t, 1, clonedElement.ArrayIndex, "Element should be at new position after move")
		
		t.Logf("SUCCESS: Clone preserves array metadata and functionality")
	})
}