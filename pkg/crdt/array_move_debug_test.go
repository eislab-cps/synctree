package crdt

import (
	"testing"

	"github.com/eislab-cps/synctree/pkg/core"
	"github.com/eislab-cps/synctree/pkg/vectorclock"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

// TestArrayMoveDebug debugs why moves are being rejected
func TestArrayMoveDebug(t *testing.T) {
	t.Run("debug move rejection", func(t *testing.T) {
		logrus.SetLevel(logrus.WarnLevel)
		
		// Create simple test case
		tree := NewTreeCRDT()
		setupClient := core.ClientID("setup")
		
		// Create array
		arrayRoot := tree.CreateNode("testArray", Map, setupClient)
		arrayRoot.IsArrayRoot = true
		err := tree.AddEdge(tree.Root.ID, arrayRoot.ID, "testArray", setupClient)
		require.NoError(t, err)
		
		// Create one element
		elem := tree.CreateNode("elem_1", Literal, setupClient)
		elem.LiteralValue = "1"
		
		err = tree.AddArrayElement(arrayRoot.ID, elem.ID, 0, setupClient)
		require.NoError(t, err)
		
		t.Logf("INITIAL ELEMENT STATE:")
		t.Logf("  ID: %s", elem.ID)
		t.Logf("  IsArrayElement: %v", elem.IsArrayElement)
		t.Logf("  ArrayIndex: %d", elem.ArrayIndex)
		t.Logf("  Clock: %v", elem.Clock)
		t.Logf("  Owner: %s", elem.Owner)
		
		// Now try to move it with a new client
		newClient := core.ClientID("newClient")
		
		t.Logf("\nATTEMPTING MOVE:")
		t.Logf("  From position: %d", elem.ArrayIndex)
		t.Logf("  To position: 2")
		t.Logf("  New client: %s", newClient)
		
		// Manually simulate what MoveArrayElement does
		newClock := vectorclock.CopyClock(elem.Clock)
		newClock[newClient] = newClock[newClient] + 1
		
		t.Logf("\nCLOCK COMPARISON:")
		t.Logf("  Original clock: %v", elem.Clock)
		t.Logf("  New clock: %v", newClock)
		
		winningClock, winningOwner := vectorclock.ResolveConflict(elem.Clock, newClock, elem.Owner, newClient, false)
		
		t.Logf("  Winning clock: %v", winningClock)
		t.Logf("  Winning owner: %s", winningOwner)
		t.Logf("  New clock equals winning? %v", vectorclock.ClocksEqual(winningClock, newClock))
		t.Logf("  New owner equals winning? %v", winningOwner == newClient)
		
		shouldMove := vectorclock.ClocksEqual(winningClock, newClock) && winningOwner == newClient
		t.Logf("  Should move: %v", shouldMove)
		
		// Now actually try the move
		err = tree.MoveArrayElement(elem.ID, arrayRoot.ID, 2, newClient)
		require.NoError(t, err, "Move should not return error")
		
		t.Logf("\nAFTER MOVE ATTEMPT:")
		t.Logf("  ArrayIndex: %d", elem.ArrayIndex)
		t.Logf("  Clock: %v", elem.Clock)
		t.Logf("  Owner: %s", elem.Owner)
		
		if elem.ArrayIndex == 2 {
			t.Logf("SUCCESS: Move worked!")
		} else {
			t.Logf("FAILED: Move was rejected")
		}
	})
}