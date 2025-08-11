package crdt

import (
	"testing"

	"github.com/eislab-cps/synctree/pkg/core"
	"github.com/eislab-cps/synctree/pkg/vectorclock"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

// TestArrayCloneDebug debugs move behavior after cloning
func TestArrayCloneDebug(t *testing.T) {
	t.Run("debug move after clone", func(t *testing.T) {
		logrus.SetLevel(logrus.WarnLevel)
		
		// Create initial tree exactly like in the failing test
		tree := NewTreeCRDT()
		setupClient := core.ClientID("setup")
		
		// Create array and element
		arrayRoot := tree.CreateNode("testArray", Map, setupClient)
		arrayRoot.IsArrayRoot = true
		err := tree.AddEdge(tree.Root.ID, arrayRoot.ID, "testArray", setupClient)
		require.NoError(t, err)
		
		elem := tree.CreateNode("elem_1", Literal, setupClient)
		elem.LiteralValue = "1"
		
		err = tree.AddArrayElement(arrayRoot.ID, elem.ID, 0, setupClient)
		require.NoError(t, err)
		
		t.Logf("ORIGINAL TREE - Element state:")
		t.Logf("  ArrayIndex: %d", elem.ArrayIndex)
		t.Logf("  Clock: %v", elem.Clock)
		t.Logf("  Owner: %s", elem.Owner)
		
		// Clone the tree (like in failing test)
		clonedTree, err := tree.Clone()
		require.NoError(t, err, "Clone should succeed")
		
		// Find the element in the cloned tree
		clonedElem := clonedTree.Nodes[elem.ID]
		require.NotNil(t, clonedElem, "Element should exist in cloned tree")
		
		t.Logf("\nCLONED TREE - Element state:")
		t.Logf("  ArrayIndex: %d", clonedElem.ArrayIndex)
		t.Logf("  Clock: %v", clonedElem.Clock)
		t.Logf("  Owner: %s", clonedElem.Owner)
		t.Logf("  IsArrayElement: %v", clonedElem.IsArrayElement)
		
		// Try to move in cloned tree
		newClient := core.ClientID("newClient")
		
		t.Logf("\nATTEMPTING MOVE IN CLONED TREE:")
		
		// Check what the LWW resolution would be
		newClock := vectorclock.CopyClock(clonedElem.Clock)
		newClock[newClient] = newClock[newClient] + 1
		
		t.Logf("  Original clock: %v", clonedElem.Clock)
		t.Logf("  New clock: %v", newClock)
		
		winningClock, winningOwner := vectorclock.ResolveConflict(clonedElem.Clock, newClock, clonedElem.Owner, newClient, false)
		
		t.Logf("  Winning clock: %v", winningClock)
		t.Logf("  Winning owner: %s", winningOwner)
		shouldMove := vectorclock.ClocksEqual(winningClock, newClock) && winningOwner == newClient
		t.Logf("  Should move: %v", shouldMove)
		
		// Actually perform the move
		err = clonedTree.MoveArrayElement(elem.ID, arrayRoot.ID, 2, newClient)
		require.NoError(t, err, "Move should not return error")
		
		t.Logf("\nAFTER MOVE:")
		t.Logf("  ArrayIndex: %d", clonedElem.ArrayIndex)
		t.Logf("  Clock: %v", clonedElem.Clock)
		
		if clonedElem.ArrayIndex == 2 {
			t.Logf("SUCCESS: Move worked in cloned tree!")
		} else {
			t.Logf("FAILED: Move was rejected in cloned tree")
		}
		
		// Check array state
		elements := clonedTree.GetArrayElements(arrayRoot.ID)
		t.Logf("\nFinal array state:")
		for i, el := range elements {
			t.Logf("  Position %d: %v (ArrayIndex: %d)", i, el.LiteralValue, el.ArrayIndex)
		}
	})
}