package crdt

import (
	"testing"

	"github.com/eislab-cps/synctree/pkg/core"
)

func TestIsDescendant(t *testing.T) {
	tree := NewTreeCRDT()
	client := core.ClientID("test-client")

	// Build structure:
	// root
	// ├── A
	// │   └── B
	// │       └── C
	// └── X
	//     └── Y
	nodeA := tree.CreateAttachedNode("A", Map, tree.Root.ID, client)
	nodeB := tree.CreateAttachedNode("B", Map, nodeA.ID, client)
	nodeC := tree.CreateAttachedNode("C", Map, nodeB.ID, client)

	nodeX := tree.CreateAttachedNode("X", Map, tree.Root.ID, client)
	nodeY := tree.CreateAttachedNode("Y", Map, nodeX.ID, client)

	tests := []struct {
		name     string
		root     core.NodeID
		target   core.NodeID
		expected bool
	}{
		{"C is descendant of root", tree.Root.ID, nodeC.ID, true},
		{"B is descendant of A", nodeA.ID, nodeB.ID, true},
		{"A is not descendant of C", nodeC.ID, nodeA.ID, false},
		{"root is descendant of root", tree.Root.ID, tree.Root.ID, true},
		{"unrelated (B is not descendant of Y)", nodeY.ID, nodeB.ID, false},
		{"Y is descendant of X", nodeX.ID, nodeY.ID, true},
		{"X is not descendant of A", nodeA.ID, nodeX.ID, false},
		{"C is not under X", nodeX.ID, nodeC.ID, false},
		{"node not in tree", nodeC.ID, "missing-node", false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := tree.IsDescendant(test.root, test.target)
			if result != test.expected {
				t.Errorf("IsDescendant(%s, %s) = %v; want %v", test.root, test.target, result, test.expected)
			}
		})
	}
}
