package crdt

import (
	"github.com/eislab-cps/synctree/pkg/core"
	"github.com/eislab-cps/synctree/pkg/vectorclock"
)


// DeltaSync provides delta synchronization capabilities for TreeCRDT.
// This is a thin layer over TreeCRDT that enables delta-state CRDT operations.
type DeltaSync struct {
	tree *TreeCRDT
}

// NewDeltaSync creates a new DeltaSync instance
func NewDeltaSync(tree *TreeCRDT) *DeltaSync {
	return &DeltaSync{
		tree: tree,
	}
}


// GenerateDeltaState creates a delta by extracting state newer than fromClock
func (ds *DeltaSync) GenerateDeltaState(fromClock vectorclock.VectorClock) *TreeCRDT {
	// Create a new TreeCRDT to hold the delta state
	delta := NewTreeCRDT()
	
	// Extract nodes that have been modified after fromClock
	for nodeID, node := range ds.tree.Nodes {
		// Check if this node has changes newer than fromClock
		dominates := vectorclock.DominatesOrEqual(fromClock, node.Clock)
		// DEBUG: uncomment the next line for debugging
		// fmt.Printf("Node %s: fromClock=%v, nodeClock=%v, dominates=%v, include=%v\n", nodeID, fromClock, node.Clock, dominates, !dominates)
		if !dominates {
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
	
	// Set up the delta root if it exists in the source tree
	if sourceRoot, exists := ds.tree.Nodes["root"]; exists {
		if deltaRoot, exists := delta.Nodes["root"]; exists {
			// Copy root edges that point to nodes in the delta
			deltaRoot.Edges = make([]*EdgeCRDT, 0)
			for _, edge := range sourceRoot.Edges {
				if _, targetExists := delta.Nodes[edge.To]; targetExists {
					deltaRoot.Edges = append(deltaRoot.Edges, &EdgeCRDT{
						From:         edge.From,
						To:           edge.To,
						Label:        edge.Label,
						LSEQPosition: make([]int, len(edge.LSEQPosition)),
					})
					copy(deltaRoot.Edges[len(deltaRoot.Edges)-1].LSEQPosition, edge.LSEQPosition)
				}
			}
			delta.Root = deltaRoot
		}
	}
	
	return delta
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

// ApplyDeltaState applies a delta state to the current tree using TreeCRDT's existing merge logic.
// This ensures consistency with TreeCRDT's conflict resolution including array promotion.
func (ds *DeltaSync) ApplyDeltaState(deltaState *TreeCRDT) error {
	// Use the existing, battle-tested merge logic
	return ds.tree.Merge(deltaState)
}

// Legacy types for backward compatibility with securetree_adapter.go
// TODO: These should be migrated away from in favor of the new delta-state approach
type OperationRecorder interface {
	RecordOperation(op DeltaOperation)
}

type DeltaOperation struct {
	Type      Operation                       `json:"type"`
	NodeID    core.NodeID                     `json:"node_id,omitempty"`
	ParentID  core.NodeID                     `json:"parent_id,omitempty"`
	EdgeInfo  *EdgeInfo                       `json:"edge_info,omitempty"`
	Value     interface{}                     `json:"value,omitempty"`
	Clock     vectorclock.VectorClock         `json:"clock"`
	ClientID  core.ClientID                   `json:"client_id"`
	Metadata  map[string]interface{}          `json:"metadata,omitempty"`
}

type EdgeInfo struct {
	FromNodeID core.NodeID `json:"from_node_id"`
	ToNodeID   core.NodeID `json:"to_node_id"`
	Label      string      `json:"label"`
	Position   int         `json:"position,omitempty"`
}

type Delta struct {
	Operations []DeltaOperation            `json:"operations"`
	FromClock  vectorclock.VectorClock     `json:"from_clock"`
	ToClock    vectorclock.VectorClock     `json:"to_clock"`
	SourceID   core.ClientID               `json:"source_id"`
}

