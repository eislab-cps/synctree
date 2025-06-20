package crdt

import (
	"testing"

	"github.com/eislab-cps/synctree/internal/crypto"
	"github.com/stretchr/testify/assert"
)

type mockTree struct{}

func (m *mockTree) isDescendant(root NodeID, target NodeID) bool {
	return root == "parent" && target == "child"
}

func TestABACPolicyWithModifyOnly(t *testing.T) {
	tree := &mockTree{}

	prvKey := "d6eb959e9aec2e6fdc44b5862b269e987b8a4d6f2baca542d8acaa97ee5e74f6"
	identity, err := crypto.CreateIdendityFromString(prvKey)
	assert.NoError(t, err)

	policy := NewABACPolicy(tree, identity.ID(), identity)

	clientA := "alice"
	clientB := "bob"
	nodeX := NodeID("node-x")
	nodeY := NodeID("node-y")
	parent := NodeID("parent")
	child := NodeID("child")

	// Setup rules
	policy.Allow(clientA, ActionModify, "*", false)   // Alice can modify anything
	policy.Allow(clientA, ActionModify, parent, true) // Alice can modify parent and children
	policy.Allow(clientB, ActionModify, nodeY, false) // Bob can modify node-y only

	tests := []struct {
		name     string
		id       string
		action   ABACAction
		nodeID   NodeID
		expected bool
	}{
		{"Alice modify node-x", clientA, ActionModify, nodeX, true},
		{"Alice modify node-y", clientA, ActionModify, nodeY, true},
		{"Bob modify node-x", clientB, ActionModify, nodeX, false},
		{"Bob modify node-y", clientB, ActionModify, nodeY, true},
		{"Charlie modify node-y", "charlie", ActionModify, nodeY, false},
		{"Alice recursive modify child of parent", clientA, ActionModify, child, true},
		{"Bob recursive modify child of parent", clientB, ActionModify, child, false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := policy.IsAllowed(test.id, test.action, test.nodeID)
			if result != test.expected {
				t.Errorf("IsAllowed(%s, %s, %s) = %v; want %v", test.id, test.action, test.nodeID, result, test.expected)
			}
		})
	}
}

func TestABACPolicyUpdateAndRemove(t *testing.T) {
	tree := &mockTree{}

	prvKey := "d6eb959e9aec2e6fdc44b5862b269e987b8a4d6f2baca542d8acaa97ee5e74f6"
	identity, err := crypto.CreateIdendityFromString(prvKey)
	assert.NoError(t, err)

	policy := NewABACPolicy(tree, identity.ID(), identity)
	client := "carol"
	node := NodeID("node-test")

	// Initially deny
	if policy.IsAllowed(client, ActionModify, node) {
		t.Errorf("Expected not allowed before rule is added")
	}

	// Add and verify
	err = policy.Allow(client, ActionModify, node, false)
	assert.NoError(t, err)
	if !policy.IsAllowed(client, ActionModify, node) {
		t.Errorf("Expected allowed after rule is added")
	}

	// Update to recursive
	err = policy.UpdateRule(client, ActionModify, node, true)
	assert.NoError(t, err)
	rule := policy.Rules[client][ActionModify][node]
	if !rule.Recursive {
		t.Errorf("Expected rule to be recursive after update")
	}

	// Remove and verify
	err = policy.RemoveRule(client, ActionModify, node)
	assert.NoError(t, err)
	if policy.IsAllowed(client, ActionModify, node) {
		t.Errorf("Expected not allowed after rule is removed")
	}
}
