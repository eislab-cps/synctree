package crdt

import (
	"fmt"

	"github.com/eislab-cps/synctree/pkg/core"
	"github.com/eislab-cps/synctree/pkg/vectorclock"
	log "github.com/sirupsen/logrus"
)

func (n *NodeCRDT) SetLiteral(value interface{}, clientID core.ClientID) (Mutation, error) {
	mut := n.generateSetLiteralMutations(value, clientID)
	if err := n.applySetLiteralMutations(mut); err != nil {
		log.WithFields(log.Fields{
			"NodeID":         n.ID,
			"AttemptedValue": value,
			"ClientID":       clientID,
			"Error":          err,
		}).Error("SetLiteral failed")
		return Mutation{}, fmt.Errorf("SetLiteral failed: %w", err)
	}

	return mut, nil
}

func (n *NodeCRDT) generateSetLiteralMutations(value interface{}, clientID core.ClientID) Mutation {
	// Find max version for this client
	maxVersion := 0
	for _, v := range n.Clock {
		if v > maxVersion {
			maxVersion = v
		}
	}
	version := maxVersion + 1

	mut := Mutation{
		NodeID:   n.ID,
		Op:       OPSetLiteral,
		Value:    value,
		ClientID: clientID,
		Version:  version,
	}

	return mut
}

func (n *NodeCRDT) applySetLiteralMutations(mut Mutation) error {
	value := normalizeNumber(mut.Value) // If value is a number, normalize it to float64 since JS uses float64 for all numbers
	currentClock := n.Clock
	newClock := make(vectorclock.VectorClock)
	newClock[mut.ClientID] = mut.Version

	winningClock, winningOwner := vectorclock.ResolveConflict(currentClock, newClock, n.Owner, mut.ClientID, false)

	if vectorclock.ClocksEqual(winningClock, newClock) && winningOwner == mut.ClientID {
		n.IsLiteral = true
		n.LiteralValue = value
		n.Clock = newClock
		n.Owner = mut.ClientID
		log.WithFields(log.Fields{
			"NodeID":       n.ID,
			"NodeClock":    currentClock,
			"NewClock":     newClock,
			"WinningClock": winningClock,
			"WinningOwner": winningOwner,
			"ClientID":     mut.ClientID,
			"LiteralValue": value}).Debug("Set literal value")

		// XXX: We cannot notify subscribers if node does not have a parent, this will happen when using CreateNode
		if n.ParentID != "" {
			n.tree.notifySubscribers(n.ID, EventUpdated)
		} else {
			//		panic("SetLiteral called on a node without parent, this should not happen")
		}
	} else {
		log.WithFields(log.Fields{"NodeID": n.ID,
			"AttemptedLiteralValue": value,
			"ClientID":              mut.ClientID,
			"NodeClock":             currentClock,
			"NewClock":              newClock,
			"WinningClock":          winningClock,
			"ExistingOwner":         n.Owner,
			"WinningOwner":          winningOwner}).Debug("Literal set ignored due to conflict")
		return fmt.Errorf("cannot set literal value, conflict detected: %s", n.ID)
	}

	return nil
}

func (n *NodeCRDT) setLiteralWithVersion(value interface{}, clientID core.ClientID, version int) error {
	mut := Mutation{
		NodeID:   n.ID,
		Op:       OPSetLiteral,
		Value:    value,
		ClientID: clientID,
		Version:  version,
	}

	if err := n.applySetLiteralMutations(mut); err != nil {
		log.WithFields(log.Fields{
			"NodeID":         n.ID,
			"AttemptedValue": value,
			"ClientID":       clientID,
			"Error":          err,
		}).Error("SetLiteral failed")
		return fmt.Errorf("SetLiteral failed: %w", err)
	}

	return nil
}
