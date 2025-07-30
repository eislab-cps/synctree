package crdt

import (
	"testing"

	"github.com/eislab-cps/synctree/internal/crypto"
	"github.com/eislab-cps/synctree/pkg/core"
	"github.com/eislab-cps/synctree/pkg/vectorclock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSignAndVerifyDeltaMutation(t *testing.T) {
	// Create an identity for signing
	identity, err := crypto.CreateIdendity()
	require.NoError(t, err)

	// Create a test mutation
	mut := DeltaMutation{
		NodeID:   "test-node",
		Op:       OPSetLiteral,
		Key:      "testkey",
		Value:    "testvalue",
		ClientID: core.ClientID(identity.ID()),
		Version:  1,
		Clock:    vectorclock.VectorClock{"client1": 1},
	}

	// Sign the mutation
	err = SignDeltaMutation(&mut, identity)
	require.NoError(t, err)
	assert.NotEmpty(t, mut.Signature)

	// Verify the mutation
	recoveredID, err := VerifyDeltaMutation(mut)
	require.NoError(t, err)
	assert.Equal(t, identity.ID(), recoveredID)
}

func TestVerifyDeltaMutationWithInvalidSignature(t *testing.T) {
	// Create an identity for signing
	identity, err := crypto.CreateIdendity()
	require.NoError(t, err)

	// Create a test mutation
	mut := DeltaMutation{
		NodeID:   "test-node",
		Op:       OPSetLiteral,
		Key:      "testkey",
		Value:    "testvalue",
		ClientID: core.ClientID(identity.ID()),
		Version:  1,
		Clock:    vectorclock.VectorClock{"client1": 1},
	}

	// Sign the mutation
	err = SignDeltaMutation(&mut, identity)
	require.NoError(t, err)

	// Tamper with the signature
	mut.Signature = "invalid_signature"

	// Verify should fail
	_, err = VerifyDeltaMutation(mut)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to decode signature")
}

func TestVerifyDeltaMutationWithTamperedData(t *testing.T) {
	// Create an identity for signing
	identity, err := crypto.CreateIdendity()
	require.NoError(t, err)

	// Create a test mutation
	mut := DeltaMutation{
		NodeID:   "test-node",
		Op:       OPSetLiteral,
		Key:      "testkey",
		Value:    "testvalue",
		ClientID: core.ClientID(identity.ID()),
		Version:  1,
		Clock:    vectorclock.VectorClock{"client1": 1},
	}

	// Sign the mutation
	err = SignDeltaMutation(&mut, identity)
	require.NoError(t, err)
	originalSig := mut.Signature

	// Tamper with the data
	mut.Value = "tampered_value"

	// Verify should succeed but recover a different ID than expected
	recoveredID2, err := VerifyDeltaMutation(mut)
	require.NoError(t, err) // Signature verification itself succeeds
	assert.NotEqual(t, identity.ID(), recoveredID2) // But recovered ID is different

	// Restore original value and verify works again
	mut.Value = "testvalue"
	mut.Signature = originalSig
	recoveredID, err := VerifyDeltaMutation(mut)
	require.NoError(t, err)
	assert.Equal(t, identity.ID(), recoveredID)
}

func TestSecureMergeDeltaWithValidSignatures(t *testing.T) {
	// Create two trees
	tree1 := NewTreeCRDT()
	tree2 := NewTreeCRDT()

	// Create an identity
	identity, err := crypto.CreateIdendity()
	require.NoError(t, err)
	clientID := core.ClientID(identity.ID())

	// Create and sign a mutation
	mut := DeltaMutation{
		NodeID:   tree1.Root.ID,
		Op:       OPSetLiteral,
		Key:      "testkey",
		Value:    "testvalue",
		ClientID: clientID,
		Version:  1,
		Clock:    vectorclock.VectorClock{clientID: 1},
	}

	err = SignDeltaMutation(&mut, identity)
	require.NoError(t, err)

	// Create delta
	delta := &DeltaCRDT{
		Mutations: []DeltaMutation{mut},
		Clock:     vectorclock.VectorClock{clientID: 1},
	}

	// Secure merge should succeed
	err = tree2.SecureMergeDelta(delta, "dummy_key")
	require.NoError(t, err)
}

func TestSecureMergeDeltaWithInvalidSignature(t *testing.T) {
	// Create two trees
	tree1 := NewTreeCRDT()
	tree2 := NewTreeCRDT()

	// Create two different identities
	identity1, err := crypto.CreateIdendity()
	require.NoError(t, err)
	
	identity2, err := crypto.CreateIdendity()
	require.NoError(t, err)

	// Create a mutation claiming to be from identity2 but signed by identity1
	mut := DeltaMutation{
		NodeID:   tree1.Root.ID,
		Op:       OPSetLiteral,
		Key:      "testkey",
		Value:    "testvalue",
		ClientID: core.ClientID(identity2.ID()), // Claiming to be identity2
		Version:  1,
		Clock:    vectorclock.VectorClock{core.ClientID(identity2.ID()): 1},
	}

	// Sign with identity1 (different from claimed ClientID)
	err = SignDeltaMutation(&mut, identity1)
	require.NoError(t, err)

	// Create delta
	delta := &DeltaCRDT{
		Mutations: []DeltaMutation{mut},
		Clock:     vectorclock.VectorClock{core.ClientID(identity2.ID()): 1},
	}

	// Secure merge should fail due to ID mismatch
	err = tree2.SecureMergeDelta(delta, "dummy_key")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "does not match claimed client ID")
}

func TestSecureMergeDeltaWithUnsignedMutation(t *testing.T) {
	// Create two trees
	tree1 := NewTreeCRDT()
	tree2 := NewTreeCRDT()

	// Create an identity
	identity, err := crypto.CreateIdendity()
	require.NoError(t, err)
	clientID := core.ClientID(identity.ID())

	// Create an unsigned mutation
	mut := DeltaMutation{
		NodeID:   tree1.Root.ID,
		Op:       OPSetLiteral,
		Key:      "testkey",
		Value:    "testvalue",
		ClientID: clientID,
		Version:  1,
		Clock:    vectorclock.VectorClock{clientID: 1},
		// No signature
	}

	// Create delta
	delta := &DeltaCRDT{
		Mutations: []DeltaMutation{mut},
		Clock:     vectorclock.VectorClock{clientID: 1},
	}

	// Secure merge should fail due to missing signature
	err = tree2.SecureMergeDelta(delta, "dummy_key")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "has no signature in secure mode")
}