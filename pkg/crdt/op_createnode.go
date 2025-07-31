package crdt

import (
	"fmt"

	"github.com/eislab-cps/synctree/pkg/core"
	"github.com/eislab-cps/synctree/pkg/vectorclock"
	log "github.com/sirupsen/logrus"
)

func (c *TreeCRDT) CreateNodeMutation(name string, nodeType core.NodeType, parentID core.NodeID, clientID core.ClientID) (Mutation, *NodeCRDT, error) {
	mut := c.generateCreateNodeMutations(name, nodeType, parentID, clientID)
	node, err := c.applyCreateNodeMutations(mut)
	if err != nil {
		log.WithFields(log.Fields{
			"Name":     name,
			"NodeType": nodeType,
			"ParentID": parentID,
			"ClientID": clientID,
			"Error":    err,
		}).Error("CreateNode failed")
		return Mutation{}, nil, fmt.Errorf("CreateNode failed: %w", err)
	}

	// Record delta operation if a recorder is set
	if c.deltaRecorder != nil {
		c.recordOperation(DeltaOperation{
			Type:     OPCreateNode,
			NodeID:   node.ID,
			ParentID: parentID,
			ClientID: clientID,
			Clock:    node.Clock,
			Metadata: map[string]interface{}{
				"is_map":     node.IsMap,
				"is_array":   node.IsArray,
				"is_literal": node.IsLiteral,
			},
		})
	}

	return mut, node, nil
}

func (c *TreeCRDT) generateCreateNodeMutations(name string, nodeType core.NodeType, parentID core.NodeID, clientID core.ClientID) Mutation {
	id := generateRandomNodeID(name)
	
	// Find max version for this client across the tree
	maxVersion := 0
	for _, node := range c.Nodes {
		if v, exists := node.Clock[clientID]; exists && v > maxVersion {
			maxVersion = v
		}
	}
	version := maxVersion + 1

	mut := Mutation{
		NodeID:   id,
		Op:       OPCreateNode,
		ClientID: clientID,
		Version:  version,
		Metadata: map[string]interface{}{
			"name":      name,
			"node_type": nodeType,
			"parent_id": parentID,
		},
	}

	return mut
}

func (c *TreeCRDT) applyCreateNodeMutations(mut Mutation) (*NodeCRDT, error) {
	// Extract metadata
	metadata := mut.Metadata
	name, _ := metadata["name"].(string)
	nodeType, _ := metadata["node_type"].(core.NodeType)
	parentID, _ := metadata["parent_id"].(core.NodeID)

	// Check if node already exists
	if _, exists := c.Nodes[mut.NodeID]; exists {
		return c.Nodes[mut.NodeID], nil // Node already exists, skip
	}

	// Create new node
	node := &NodeCRDT{
		tree:     c,
		ID:       mut.NodeID,
		ParentID: parentID,
		Clock:    vectorclock.VectorClock{mut.ClientID: mut.Version},
		Owner:    mut.ClientID,
		Edges:    make([]*EdgeCRDT, 0),
	}

	// Set node type
	setNodeTypeFlags(node, nodeType)
	
	c.Nodes[mut.NodeID] = node

	log.WithFields(log.Fields{
		"NodeID":   mut.NodeID,
		"Name":     name,
		"NodeType": nodeType,
		"ParentID": parentID,
		"ClientID": mut.ClientID,
		"Version":  mut.Version,
	}).Debug("Node created")

	return node, nil
}