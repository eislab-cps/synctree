package crdt

import (
	"fmt"

	"github.com/eislab-cps/synctree/pkg/core"
	"github.com/eislab-cps/synctree/pkg/vectorclock"
	log "github.com/sirupsen/logrus"
)

func (c *TreeCRDT) AddEdgeMutation(from, to core.NodeID, label string, clientID core.ClientID) (Mutation, error) {
	mut := c.generateAddEdgeMutations(from, to, label, clientID)
	if err := c.applyAddEdgeMutations(mut); err != nil {
		log.WithFields(log.Fields{
			"From":     from,
			"To":       to,
			"Label":    label,
			"ClientID": clientID,
			"Error":    err,
		}).Error("AddEdge failed")
		return Mutation{}, fmt.Errorf("AddEdge failed: %w", err)
	}

	// Record delta operation if a recorder is set
	if c.deltaRecorder != nil {
		fromNode := c.Nodes[from]
		c.recordOperation(DeltaOperation{
			Type:     OPAddEdge,
			ClientID: clientID,
			Clock:    fromNode.Clock,
			EdgeInfo: &EdgeInfo{
				FromNodeID: from,
				ToNodeID:   to,
				Label:      label,
			},
		})
	}

	return mut, nil
}

func (c *TreeCRDT) generateAddEdgeMutations(from, to core.NodeID, label string, clientID core.ClientID) Mutation {
	fromNode := c.Nodes[from]
	version := fromNode.Clock[clientID] + 1

	mut := Mutation{
		NodeID:   from, // The node being modified
		Op:       OPAddEdge,
		ClientID: clientID,
		Version:  version,
		Metadata: map[string]interface{}{
			"from_node_id": from,
			"to_node_id":   to,
			"label":        label,
		},
	}

	return mut
}

func (c *TreeCRDT) applyAddEdgeMutations(mut Mutation) error {
	// Extract metadata
	metadata := mut.Metadata
	from, _ := metadata["from_node_id"].(core.NodeID)
	to, _ := metadata["to_node_id"].(core.NodeID)
	label, _ := metadata["label"].(string)

	// Validate nodes exist
	fromNode, ok := c.Nodes[from]
	if !ok {
		return fmt.Errorf("from node %s not found", from)
	}
	
	toNode, ok := c.Nodes[to]
	if !ok {
		return fmt.Errorf("to node %s not found", to)
	}

	// Check for cycles and multiple parents
	if err := c.validAttachment(from, to); err != nil {
		return fmt.Errorf("invalid attachment: %w", err)
	}

	// Prepare new clock
	newClock := vectorclock.CopyClock(fromNode.Clock)
	newClock[mut.ClientID] = mut.Version

	// Resolve conflicts
	winningClock, winningOwner := vectorclock.ResolveConflict(fromNode.Clock, newClock, fromNode.Owner, mut.ClientID, false)

	if vectorclock.ClocksEqual(winningClock, newClock) && winningOwner == mut.ClientID {
		// Add the edge
		edge := &EdgeCRDT{From: from, To: to, Label: label, LSEQPosition: make([]int, 0)}
		fromNode.Edges = append(fromNode.Edges, edge)
		fromNode.Clock = newClock
		fromNode.Owner = mut.ClientID
		toNode.ParentID = from

		c.notifySubscribers(fromNode.ID, EventAdded)

		log.WithFields(log.Fields{
			"From":    from,
			"To":      to,
			"Label":   label,
			"Version": mut.Version,
		}).Debug("Edge added")
	} else {
		log.WithFields(log.Fields{
			"From":    from,
			"To":      to,
			"Label":   label,
			"Version": mut.Version,
		}).Debug("Edge add ignored due to conflict")
		return fmt.Errorf("edge add conflict detected: %s -> %s", from, to)
	}

	return nil
}