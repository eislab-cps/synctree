package crdt

import (
	"encoding/json"
	"fmt"

	"github.com/eislab-cps/synctree/pkg/core"
	"github.com/eislab-cps/synctree/pkg/vectorclock"
)

// OperationRecorder interface for recording delta operations
type OperationRecorder interface {
	RecordOperation(op DeltaOperation)
}

// DeltaOperation represents a single operation in a delta
type DeltaOperation struct {
	Type      Operation                       `json:"type"`
	NodeID    core.NodeID                     `json:"node_id,omitempty"`
	ParentID  core.NodeID                     `json:"parent_id,omitempty"`
	EdgeInfo  *EdgeInfo                       `json:"edge_info,omitempty"`
	Value     interface{}                     `json:"value,omitempty"`
	Clock     vectorclock.VectorClock         `json:"clock"`
	ClientID  core.ClientID                   `json:"client_id"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// EdgeInfo contains information about edge operations
type EdgeInfo struct {
	FromNodeID core.NodeID `json:"from_node_id"`
	ToNodeID   core.NodeID `json:"to_node_id"`
	Label      string      `json:"label"`
	Position   int         `json:"position,omitempty"`
}

// Delta represents a collection of operations between two states
type Delta struct {
	Operations []DeltaOperation        `json:"operations"`
	FromClock  vectorclock.VectorClock `json:"from_clock"`
	ToClock    vectorclock.VectorClock `json:"to_clock"`
	SourceID   core.ClientID           `json:"source_id"`
}

// DeltaSync provides delta synchronization capabilities for TreeCRDT
type DeltaSync struct {
	tree       *TreeCRDT
	history    []DeltaOperation
	maxHistory int
}

// NewDeltaSync creates a new DeltaSync instance and registers it with the tree
func NewDeltaSync(tree *TreeCRDT) *DeltaSync {
	ds := &DeltaSync{
		tree:       tree,
		history:    make([]DeltaOperation, 0),
		maxHistory: 1000, // Default max history size
	}
	
	// Register this DeltaSync as the operation recorder for the tree
	tree.SetDeltaRecorder(ds)
	
	return ds
}

// RecordOperation records an operation in the delta history
// This implements the OperationRecorder interface
func (ds *DeltaSync) RecordOperation(op DeltaOperation) {
	ds.history = append(ds.history, op)

	// Trim history if it exceeds max size
	if len(ds.history) > ds.maxHistory {
		ds.history = ds.history[len(ds.history)-ds.maxHistory:]
	}
}

// GenerateDeltaState creates a delta by extracting state newer than fromClock
func (ds *DeltaSync) GenerateDeltaState(fromClock vectorclock.VectorClock) *TreeCRDT {
	// Create a new TreeCRDT to hold the delta state
	delta := NewTreeCRDT()
	
	// Extract nodes that have been modified after fromClock
	for nodeID, node := range ds.tree.Nodes {
		// Check if this node has changes newer than fromClock
		if !clockDominatesOrEqual(fromClock, node.Clock) {
			// Clone the node into the delta
			deltaNode := &NodeCRDT{
				tree:         delta,
				ID:           node.ID,
				ParentID:     node.ParentID,
				IsLiteral:    node.IsLiteral,
				IsMap:        node.IsMap,
				IsArray:      node.IsArray,
				IsPromoted:   node.IsPromoted,
				LiteralValue: node.LiteralValue,
				Clock:        vectorclock.CopyClock(node.Clock),
				Owner:        node.Owner,
				IsDeleted:    node.IsDeleted,
				IsRoot:       node.IsRoot,
				Nonce:        node.Nonce,
				Signature:    node.Signature,
				Edges:        make([]*EdgeCRDT, len(node.Edges)),
			}
			
			// Clone edges
			for i, edge := range node.Edges {
				// Deep copy LSEQPosition slice
				lseqPos := make([]int, len(edge.LSEQPosition))
				copy(lseqPos, edge.LSEQPosition)
				
				deltaNode.Edges[i] = &EdgeCRDT{
					From:         edge.From,
					To:           edge.To,
					Label:        edge.Label,
					LSEQPosition: lseqPos,
				}
			}
			
			delta.Nodes[nodeID] = deltaNode
		}
	}
	
	// Ensure parent nodes are included for tree consistency
	ds.includeRequiredParents(delta, fromClock)
	
	return delta
}

// GenerateDelta creates a delta from a given clock state (legacy operation-based API)
// DEPRECATED: Use GenerateDeltaState for true delta-state functionality
func (ds *DeltaSync) GenerateDelta(fromClock vectorclock.VectorClock, clientID core.ClientID) *Delta {
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
	}
}

// includeRequiredParents ensures all necessary parent nodes are included in delta
func (ds *DeltaSync) includeRequiredParents(delta *TreeCRDT, fromClock vectorclock.VectorClock) {
	// Keep adding missing parents until tree is consistent
	changed := true
	for changed {
		changed = false
		
		for _, node := range delta.Nodes {
			if node.ParentID != "" {
				// Check if parent exists in delta
				if _, exists := delta.Nodes[node.ParentID]; !exists {
					// Check if parent exists in source tree
					if parent, exists := ds.tree.Nodes[node.ParentID]; exists {
						// Clone parent node into delta
						deltaParent := &NodeCRDT{
							tree:         delta,
							ID:           parent.ID,
							ParentID:     parent.ParentID,
							IsLiteral:    parent.IsLiteral,
							IsMap:        parent.IsMap,
							IsArray:      parent.IsArray,
							IsPromoted:   parent.IsPromoted,
							LiteralValue: parent.LiteralValue,
							Clock:        vectorclock.CopyClock(parent.Clock),
							Owner:        parent.Owner,
							IsDeleted:    parent.IsDeleted,
							IsRoot:       parent.IsRoot,
							Nonce:        parent.Nonce,
							Signature:    parent.Signature,
							Edges:        make([]*EdgeCRDT, len(parent.Edges)),
						}
						
						// Clone edges
						for i, edge := range parent.Edges {
							// Deep copy LSEQPosition slice
							lseqPos := make([]int, len(edge.LSEQPosition))
							copy(lseqPos, edge.LSEQPosition)
							
							deltaParent.Edges[i] = &EdgeCRDT{
								From:         edge.From,
								To:           edge.To,
								Label:        edge.Label,
								LSEQPosition: lseqPos,
							}
						}
						
						delta.Nodes[parent.ID] = deltaParent
						changed = true
					}
				}
			}
		}
	}
}

// ApplyDeltaState applies a delta state to the current tree using merge
func (ds *DeltaSync) ApplyDeltaState(deltaState *TreeCRDT) error {
	// Use the existing, battle-tested merge logic
	return ds.tree.Merge(deltaState)
}

// ApplyDelta applies a delta to the current tree (legacy operation-based API)
// DEPRECATED: Use ApplyDeltaState for true delta-state functionality
func (ds *DeltaSync) ApplyDelta(delta *Delta) error {
	// Apply each operation in order
	for _, op := range delta.Operations {
		if err := ds.applyOperation(op); err != nil {
			return fmt.Errorf("failed to apply operation %d: %w", op.Type, err)
		}
	}

	return nil
}

// applyOperation applies a single delta operation to the tree
func (ds *DeltaSync) applyOperation(op DeltaOperation) error {
	switch op.Type {
	case OPCreateNode:
		// Handle node creation
		return ds.applyCreateNode(op)
	case OPAddEdge:
		// Handle edge addition
		return ds.applyAddEdge(op)
	case OPRemoveEdge:
		// Handle edge removal
		return ds.applyRemoveEdge(op)
	case OPSetLiteral:
		// Handle literal value setting
		return ds.applySetLiteral(op)
	case OPDeleteNode:
		// Handle node deletion
		return ds.applyDeleteNode(op)
	case OPUpdateNode:
		// Handle node update
		return ds.applyUpdateNode(op)
	case OPUpdateClock:
		// Handle clock update
		return ds.applyUpdateClock(op)
	default:
		return fmt.Errorf("unknown operation type: %d", op.Type)
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

	// Use proper conflict resolution like the rest of the CRDT system
	winningClock, winningOwner := vectorclock.ResolveConflict(
		node.Clock, 
		op.Clock, 
		node.Owner, 
		op.ClientID, 
		false, // LWW mode, not append
	)

	// Apply the operation only if the operation wins the conflict resolution
	if vectorclock.ClocksEqual(winningClock, op.Clock) && winningOwner == op.ClientID {
		node.IsLiteral = true
		node.LiteralValue = op.Value
		node.Clock = op.Clock
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
func (t *TreeCRDT) GetVectorClock() vectorclock.VectorClock {
	clock := make(vectorclock.VectorClock)

	// Merge clocks from all nodes
	for _, node := range t.Nodes {
		clock = mergeClock(clock, node.Clock)
	}

	return clock
}

// Helper functions for clock operations
func clockDominatesOrEqual(a, b vectorclock.VectorClock) bool {
	// Check if a dominates or equals b
	if vectorclock.ClocksEqual(a, b) {
		return true
	}
	
	// Check if a dominates b (all elements in a >= corresponding elements in b)
	keys := make(map[core.ClientID]struct{})
	for k := range a {
		keys[k] = struct{}{}
	}
	for k := range b {
		keys[k] = struct{}{}
	}
	
	for k := range keys {
		av, aok := a[k]
		bv, bok := b[k]
		if !aok {
			av = 0
		}
		if !bok {
			bv = 0
		}
		if av < bv {
			return false // a does not dominate b
		}
	}
	return true
}

func mergeClock(a, b vectorclock.VectorClock) vectorclock.VectorClock {
	return vectorclock.MergeClocks(a, b)
}

// Serialization methods
func (d *Delta) ToJSON() ([]byte, error) {
	return json.Marshal(d)
}

func (d *Delta) FromJSON(data []byte) error {
	return json.Unmarshal(data, d)
}
