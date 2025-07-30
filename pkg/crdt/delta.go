package crdt

import (
	"encoding/json"
	"fmt"
	"time"
)

// Operation types for delta synchronization
type OperationType string

const (
	OpCreateNode  OperationType = "create_node"
	OpUpdateNode  OperationType = "update_node"
	OpDeleteNode  OperationType = "delete_node"
	OpAddEdge     OperationType = "add_edge"
	OpRemoveEdge  OperationType = "remove_edge"
	OpSetLiteral  OperationType = "set_literal"
	OpUpdateClock OperationType = "update_clock"
)

// DeltaOperation represents a single operation in a delta
type DeltaOperation struct {
	Type      OperationType          `json:"type"`
	NodeID    NodeID                 `json:"node_id,omitempty"`
	ParentID  NodeID                 `json:"parent_id,omitempty"`
	EdgeInfo  *EdgeInfo              `json:"edge_info,omitempty"`
	Value     interface{}            `json:"value,omitempty"`
	Clock     VectorClock            `json:"clock"`
	ClientID  ClientID               `json:"client_id"`
	Timestamp time.Time              `json:"timestamp"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// EdgeInfo contains information about edge operations
type EdgeInfo struct {
	FromNodeID NodeID `json:"from_node_id"`
	ToNodeID   NodeID `json:"to_node_id"`
	Label      string `json:"label"`
	Position   int    `json:"position,omitempty"`
}

// Delta represents a collection of operations between two states
type Delta struct {
	Operations []DeltaOperation `json:"operations"`
	FromClock  VectorClock      `json:"from_clock"`
	ToClock    VectorClock      `json:"to_clock"`
	SourceID   ClientID         `json:"source_id"`
	Created    time.Time        `json:"created"`
}

// DeltaSync provides delta synchronization capabilities for TreeCRDT
type DeltaSync struct {
	tree       *TreeCRDT
	history    []DeltaOperation
	maxHistory int
}

// NewDeltaSync creates a new DeltaSync instance
func NewDeltaSync(tree *TreeCRDT) *DeltaSync {
	return &DeltaSync{
		tree:       tree,
		history:    make([]DeltaOperation, 0),
		maxHistory: 1000, // Default max history size
	}
}

// RecordOperation records an operation in the delta history
func (ds *DeltaSync) RecordOperation(op DeltaOperation) {
	ds.history = append(ds.history, op)

	// Trim history if it exceeds max size
	if len(ds.history) > ds.maxHistory {
		ds.history = ds.history[len(ds.history)-ds.maxHistory:]
	}
}

// GenerateDelta creates a delta from a given clock state
func (ds *DeltaSync) GenerateDelta(fromClock VectorClock, clientID ClientID) *Delta {
	operations := []DeltaOperation{}

	// Find all operations that happened after fromClock
	for _, op := range ds.history {
		if !clockDominatesOrEqual(fromClock, op.Clock) {
			operations = append(operations, op)
		}
	}

	// Get current tree clock
	currentClock := ds.tree.GetVectorClock()

	return &Delta{
		Operations: operations,
		FromClock:  fromClock,
		ToClock:    currentClock,
		SourceID:   clientID,
		Created:    time.Now(),
	}
}

// ApplyDelta applies a delta to the current tree
func (ds *DeltaSync) ApplyDelta(delta *Delta) error {
	// Apply each operation in order
	for _, op := range delta.Operations {
		if err := ds.applyOperation(op); err != nil {
			return fmt.Errorf("failed to apply operation %s: %w", op.Type, err)
		}
	}

	return nil
}

// applyOperation applies a single delta operation to the tree
func (ds *DeltaSync) applyOperation(op DeltaOperation) error {
	switch op.Type {
	case OpCreateNode:
		// Handle node creation
		return ds.applyCreateNode(op)
	case OpUpdateNode:
		// Handle node update
		return ds.applyUpdateNode(op)
	case OpDeleteNode:
		// Handle node deletion
		return ds.applyDeleteNode(op)
	case OpAddEdge:
		// Handle edge addition
		return ds.applyAddEdge(op)
	case OpRemoveEdge:
		// Handle edge removal
		return ds.applyRemoveEdge(op)
	case OpSetLiteral:
		// Handle literal value setting
		return ds.applySetLiteral(op)
	case OpUpdateClock:
		// Handle clock update
		return ds.applyUpdateClock(op)
	default:
		return fmt.Errorf("unknown operation type: %s", op.Type)
	}
}

// Implementation of individual operation handlers
func (ds *DeltaSync) applyCreateNode(op DeltaOperation) error {
	// Check if node already exists
	if _, exists := ds.tree.Nodes[op.NodeID]; exists {
		return nil // Node already exists, skip
	}

	// Create new node
	node := &NodeCRDT{
		tree:     ds.tree,
		ID:       op.NodeID,
		ParentID: op.ParentID,
		Clock:    op.Clock,
		Owner:    op.ClientID,
		Edges:    make([]*EdgeCRDT, 0),
	}

	// Set node type from metadata
	if metadata := op.Metadata; metadata != nil {
		if isMap, ok := metadata["is_map"].(bool); ok {
			node.IsMap = isMap
		}
		if isArray, ok := metadata["is_array"].(bool); ok {
			node.IsArray = isArray
		}
		if isLiteral, ok := metadata["is_literal"].(bool); ok {
			node.IsLiteral = isLiteral
		}
	}

	ds.tree.Nodes[op.NodeID] = node
	return nil
}

func (ds *DeltaSync) applyUpdateNode(op DeltaOperation) error {
	node, exists := ds.tree.Nodes[op.NodeID]
	if !exists {
		return fmt.Errorf("node %s not found", op.NodeID)
	}

	// Update node properties based on metadata
	if metadata := op.Metadata; metadata != nil {
		// Update clock if newer
		if !clockDominatesOrEqual(node.Clock, op.Clock) {
			node.Clock = mergeClock(node.Clock, op.Clock)
		}
	}

	return nil
}

func (ds *DeltaSync) applyDeleteNode(op DeltaOperation) error {
	node, exists := ds.tree.Nodes[op.NodeID]
	if !exists {
		return nil // Already deleted
	}

	node.IsDeleted = true
	node.Clock = mergeClock(node.Clock, op.Clock)
	return nil
}

func (ds *DeltaSync) applyAddEdge(op DeltaOperation) error {
	if op.EdgeInfo == nil {
		return fmt.Errorf("edge info missing for add edge operation")
	}

	fromNode, exists := ds.tree.Nodes[op.EdgeInfo.FromNodeID]
	if !exists {
		return fmt.Errorf("from node %s not found", op.EdgeInfo.FromNodeID)
	}

	// Create edge
	edge := &EdgeCRDT{
		From:  op.EdgeInfo.FromNodeID,
		To:    op.EdgeInfo.ToNodeID,
		Label: op.EdgeInfo.Label,
	}

	// Add edge to node
	fromNode.Edges = append(fromNode.Edges, edge)
	fromNode.Clock = mergeClock(fromNode.Clock, op.Clock)

	return nil
}

func (ds *DeltaSync) applyRemoveEdge(op DeltaOperation) error {
	if op.EdgeInfo == nil {
		return fmt.Errorf("edge info missing for remove edge operation")
	}

	fromNode, exists := ds.tree.Nodes[op.EdgeInfo.FromNodeID]
	if !exists {
		return fmt.Errorf("from node %s not found", op.EdgeInfo.FromNodeID)
	}

	// Find and remove edge
	newEdges := make([]*EdgeCRDT, 0, len(fromNode.Edges))
	for _, edge := range fromNode.Edges {
		if edge.To != op.EdgeInfo.ToNodeID {
			newEdges = append(newEdges, edge)
		}
	}

	fromNode.Edges = newEdges
	fromNode.Clock = mergeClock(fromNode.Clock, op.Clock)

	return nil
}

func (ds *DeltaSync) applySetLiteral(op DeltaOperation) error {
	node, exists := ds.tree.Nodes[op.NodeID]
	if !exists {
		return fmt.Errorf("node %s not found", op.NodeID)
	}

	// Check clock ordering
	if !clockDominatesOrEqual(node.Clock, op.Clock) {
		node.LiteralValue = op.Value
		node.Clock = mergeClock(node.Clock, op.Clock)
		node.Owner = op.ClientID
	}

	return nil
}

func (ds *DeltaSync) applyUpdateClock(op DeltaOperation) error {
	node, exists := ds.tree.Nodes[op.NodeID]
	if !exists {
		return fmt.Errorf("node %s not found", op.NodeID)
	}

	node.Clock = mergeClock(node.Clock, op.Clock)
	return nil
}

// GetVectorClock returns the current vector clock of the tree
func (t *TreeCRDT) GetVectorClock() VectorClock {
	clock := make(VectorClock)

	// Merge clocks from all nodes
	for _, node := range t.Nodes {
		clock = mergeClock(clock, node.Clock)
	}

	return clock
}

// Helper functions for clock operations
func clockDominatesOrEqual(a, b VectorClock) bool {
	comp := compareClocks(a, b)
	return comp == ClockDominates || comp == ClockEqual
}

func mergeClock(a, b VectorClock) VectorClock {
	result := make(VectorClock)

	// Copy all entries from a
	for k, v := range a {
		result[k] = v
	}

	// Merge with b, taking max values
	for k, v := range b {
		if existing, ok := result[k]; ok {
			if v > existing {
				result[k] = v
			}
		} else {
			result[k] = v
		}
	}

	return result
}

// Serialization methods
func (d *Delta) ToJSON() ([]byte, error) {
	return json.Marshal(d)
}

func (d *Delta) FromJSON(data []byte) error {
	return json.Unmarshal(data, d)
}
