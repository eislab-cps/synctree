package crdt

import (
	"errors"
	"fmt"
	"sort"

	"github.com/eislab-cps/synctree/internal/crypto"
	"github.com/eislab-cps/synctree/pkg/abac"
	"github.com/eislab-cps/synctree/pkg/core"
	"github.com/eislab-cps/synctree/pkg/lseq"
	"github.com/eislab-cps/synctree/pkg/random"
	"github.com/eislab-cps/synctree/pkg/vectorclock"
	log "github.com/sirupsen/logrus"
)

const (
	Root core.NodeType = iota
	Array
	Map
	Literal
)

type NodeCRDT struct {
	tree         *TreeCRDT
	ID           core.NodeID             `json:"id"`
	ParentID     core.NodeID             `json:"parentid"`
	Edges        []*EdgeCRDT             `json:"edges"`
	Clock        vectorclock.VectorClock `json:"clock"`
	Owner        core.ClientID           `json:"owner"`
	IsRoot       bool                    `json:"isroot"`
	IsMap        bool                    `json:"ismap"`
	IsArray      bool                    `json:"isarray"`
	IsPromoted   bool                    `json:"ispromoted"`
	IsLiteral    bool                    `json:"isliteral"`
	LiteralValue interface{}             `json:"literalValue"`
	Nonce        string                  `json:"nonce"`
	Signature    string                  `json:"signature"`
	IsDeleted    bool                    `json:"deleted"`
	
	// Array-specific metadata for B-tree array implementation
	IsArrayRoot    bool   `json:"isarrayroot"`    // This node is root of an array B-tree
	IsArrayElement bool   `json:"isarrayelement"` // This node is element within array B-tree
	ArrayIndex     int    `json:"arrayindex"`     // Logical array position (0, 1, 2...)
	BTreeKey       string `json:"btreekey"`       // B-tree ordering key (LSEQ-based)
}

type EdgeCRDT struct {
	From         core.NodeID `json:"from"`
	To           core.NodeID `json:"to"`
	Label        string      `json:"label"`
	LSEQPosition []int       `json:"lseqposition"`
}

type TreeCRDT struct {
	Root        *NodeCRDT                 `json:"root"`
	Nodes       map[core.NodeID]*NodeCRDT `json:"nodes"`
	ABACPolicy  *abac.ABACPolicy          `json:"abac"`
	Secure      bool                      `json:"secure"`
	subscribers []subscriber
}

func NewTreeCRDT() *TreeCRDT {
	rootID := "root"
	root := &NodeCRDT{
		ID:     core.NodeID(rootID),
		Edges:  make([]*EdgeCRDT, 0),
		IsRoot: true,
	}
	c := &TreeCRDT{
		Root:  root,
		Nodes: make(map[core.NodeID]*NodeCRDT),
	}
	c.Nodes[c.Root.ID] = c.Root
	root.tree = c
	c.ABACPolicy = nil
	c.Secure = false

	return c
}

func (c *TreeCRDT) CreateAttachedNode(name string, nodeType core.NodeType, parentID core.NodeID, clientID core.ClientID) *NodeCRDT {
	id := generateRandomNodeID(name)
	node := c.getOrCreateNode(id, nodeType, clientID, 1)
	c.AddEdge(parentID, id, "", clientID)
	node.ParentID = parentID

	c.notifySubscribers(node.ID, EventAdded)

	return node
}

func (c *TreeCRDT) CreateNode(name string, nodeType core.NodeType, clientID core.ClientID) *NodeCRDT {
	id := generateRandomNodeID(name)
	node := c.getOrCreateNode(id, nodeType, clientID, 1)
	setNodeTypeFlags(node, nodeType)

	// Do not modify since it is not attached to the tree
	return node
}

func newNodeFromID(id core.NodeID, nodeType core.NodeType, tree *TreeCRDT) *NodeCRDT {
	node := &NodeCRDT{
		ID:    id,
		Edges: make([]*EdgeCRDT, 0),
		tree:  tree,
	}
	setNodeTypeFlags(node, nodeType)

	return node
}

func (c *TreeCRDT) getOrCreateNode(id core.NodeID, nodeType core.NodeType, clientID core.ClientID, version int) *NodeCRDT {
	if _, ok := c.Nodes[id]; !ok {
		node := newNodeFromID(id, nodeType, c)
		c.Nodes[id] = node
		node.Clock = make(vectorclock.VectorClock)
		node.Clock[clientID] = version
		node.Owner = clientID

	}
	return c.Nodes[id]
}

func (c *TreeCRDT) GetNode(id core.NodeID) (*NodeCRDT, bool) {
	node, ok := c.Nodes[id]
	if !ok {
		return nil, false
	}
	return node, true
}

func generateRandomNodeID(label string) core.NodeID {
	id := random.GenerateRandomID()
	id = label + "-" + id
	return core.NodeID(id)
}

func (n *NodeCRDT) Tree() *TreeCRDT {
	return n.tree
}

// This functions only appends a new node to the tree, no need for conflict resolution
func (n *NodeCRDT) CreateMapNode(clientID core.ClientID) (*NodeCRDT, error) {
	mapNode := n.tree.CreateNode("map", Map, clientID)
	mapNode.ParentID = n.ID
	if err := n.tree.AddEdge(n.ID, mapNode.ID, "", clientID); err != nil {
		return nil, fmt.Errorf("SetKeyValue: failed to attach map node: %w", err)
	}

	n.tree.notifySubscribers(mapNode.ID, EventAdded)

	return mapNode, nil
}

func (n *NodeCRDT) GetNodeForKey(key string) (*NodeCRDT, bool, error) {
	if !n.IsMap {
		return nil, false, fmt.Errorf("GetNodeForKey: node %s is not a map node", n.ID)
	}

	// Search for the key in the edges
	for _, edge := range n.Edges {
		if edge.Label == key {
			valueNodeID := edge.To
			valueNode, exists := n.tree.Nodes[valueNodeID]
			if !exists {
				return nil, false, fmt.Errorf("GetNodeForKey: missing node %s", valueNodeID)
			}
			return valueNode, true, nil
		}
	}
	return nil, false, nil
}

func (n *NodeCRDT) SetKeyValue(key string, value interface{}, clientID core.ClientID) (Mutation, core.NodeID, error) {
	mut := Mutation{}

	if !n.IsMap {
		return mut, "", fmt.Errorf("SetKeyValue: node %s is not a map node", n.ID)
	}

	// Check if key already exists
	for _, edge := range n.Edges {
		if edge.Label == key {
			valueNodeID := edge.To
			valueNode, exists := n.tree.Nodes[valueNodeID]
			if !exists {
				return mut, "", fmt.Errorf("SetKeyValue: missing node %s", valueNodeID)
			}
			maxVersion := 0
			for _, v := range valueNode.Clock {
				if v > maxVersion {
					maxVersion = v
				}
			}
			version := maxVersion + 1

			err := valueNode.setLiteralWithVersion(value, clientID, version)
			if err != nil {
				log.WithFields(log.Fields{
					"NodeID":         valueNodeID,
					"AttemptedValue": value,
					"ClientID":       clientID,
					"Error":          err,
				}).Error("SetLiteral failed")
				return mut, "", fmt.Errorf("SetKeyValue: failed to set value for key %s: %w", key, err)
			}

			valueNode.ParentID = n.ID // Ensure parent link is set
			return mut, valueNodeID, nil
		}
	}

	// Create new value node
	valueNodeID := generateRandomNodeID("val")
	valueNode := n.tree.getOrCreateNode(valueNodeID, Literal, clientID, 1)
	// setLiteralWithVersion will notify subscribers, when the values is updated
	err := valueNode.setLiteralWithVersion(value, clientID, 1)
	if err != nil {
		return mut, "", err
	}

	// Link to map node with key label
	if err := n.tree.AddEdge(n.ID, valueNodeID, key, clientID); err != nil {
		return mut, "", err
	}

	n.tree.notifySubscribers(valueNodeID, EventAdded)

	return mut, valueNodeID, nil
}

func (n *NodeCRDT) RemoveKeyValue(key string, clientID core.ClientID) error {
	if !n.IsMap {
		return fmt.Errorf("RemoveKeyValue: node %s is not a map node", n.ID)
	}

	for _, edge := range n.Edges {
		if edge.Label == key {
			// Simply unlink the key node by removing the edge
			return n.tree.RemoveEdge(n.ID, edge.To, clientID)
		}
	}

	return fmt.Errorf("RemoveKeyValue: key %s not found", key)
}

func (c *TreeCRDT) addEdgeWithVersion(from, to core.NodeID, label string, clientID core.ClientID, newVersion int) error {
	fromNode, ok := c.Nodes[from]
	if !ok {
		return errors.New("Cannot add edge, from node not found: " + string(from))
	}

	toNode, ok := c.Nodes[to]
	if !ok {
		return errors.New("Cannot add edge, to node not found: " + string(from))
	}

	// Prepare the new clock
	newClock := vectorclock.CopyClock(fromNode.Clock)
	newClock[clientID] = newVersion

	// Resolve clock conflict
	winningClock, winningOwner := vectorclock.ResolveConflict(fromNode.Clock, newClock, fromNode.Owner, clientID, false)

	if vectorclock.ClocksEqual(winningClock, newClock) && (clientID == winningOwner) {
		edge := &EdgeCRDT{From: from, To: to, Label: label, LSEQPosition: make([]int, 0)}
		fromNode.Edges = append(fromNode.Edges, edge)
		fromNode.Clock = newClock
		fromNode.Owner = clientID
		toNode.ParentID = from

		c.notifySubscribers(fromNode.ID, EventAdded)

		log.WithFields(log.Fields{"NodeID": from, "To": to, "Label": label, "Version": newVersion}).Debug("Edge added")
	} else {
		log.WithFields(log.Fields{"NodeID": from, "To": to, "Label": label, "Version": newVersion}).Debug("Edge add ignored due to conflict")
	}

	return nil
}

func (c *TreeCRDT) AddEdge(from, to core.NodeID, label string, clientID core.ClientID) error {
	// Check for immediate self-cycle
	if from == to {
		return fmt.Errorf("cannot attach node %s to itself", from)
	}

	// For LWW semantics, check if this operation can resolve conflicts
	if err := c.tryAddEdgeWithLWW(from, to, label, clientID); err != nil {
		return err
	}

	return nil
}

// tryAddEdgeWithLWW attempts to add an edge using LWW conflict resolution
func (c *TreeCRDT) tryAddEdgeWithLWW(from, to core.NodeID, label string, clientID core.ClientID) error {
	// Check if the target node already has a parent
	existingParent := c.findParentNode(to)
	if existingParent != "" && existingParent != from {
		// Node already has a parent - check if this is a valid move operation
		existingParentNode, exists := c.GetNode(existingParent)
		if !exists {
			return fmt.Errorf("existing parent node %s not found", existingParent)
		}

		fromNode, exists := c.GetNode(from)
		if !exists {
			return fmt.Errorf("from node %s not found", from)
		}

		// For same client, check if this is a valid move (later timestamp)
		// Different clients use LWW resolution
		if clientID == existingParentNode.Owner {
			// Same client - only allow if this is a later operation (move)

			newClock := fromNode.Clock[clientID] + 1
			existingClock := existingParentNode.Clock[existingParentNode.Owner]

			log.WithFields(log.Fields{
				"From": from, "To": to, "ClientID": clientID,
				"NewClock": newClock, "ExistingClock": existingClock,
			}).Debug("Same client move operation")

			if newClock >= existingClock {
				// Same or later operation by same client - allow as move
				log.WithFields(log.Fields{"From": from, "To": to}).Debug("Same client move allowed")
				err := c.RemoveEdge(existingParent, to, clientID)
				if err != nil {
					return fmt.Errorf("failed to remove existing edge for same-client move: %w", err)
				}
			} else {
				// Earlier or same timestamp - reject to prevent multiple parents
				return fmt.Errorf("adding edge would create multiple parents for node %s", to)
			}
		} else {
			// Different clients - use LWW resolution
			// Compare vector clocks for different clients
			newClock := fromNode.Clock[clientID] + 1
			existingClock := existingParentNode.Clock[existingParentNode.Owner]

			log.WithFields(log.Fields{
				"From": from, "To": to, "ClientID": clientID,
				"ExistingParent": existingParent, "ExistingOwner": existingParentNode.Owner,
				"NewClock": newClock, "ExistingClock": existingClock,
			}).Debug("LWW comparison for different clients")

			// Use standard LWW resolution for different clients
			canWin := newClock > existingClock || (newClock == existingClock && clientID < existingParentNode.Owner)

			if canWin {
				// New operation wins - remove existing edge first
				log.WithFields(log.Fields{"From": from, "To": to}).Debug("New edge wins LWW")
				err := c.RemoveEdge(existingParent, to, clientID)
				if err != nil {
					return fmt.Errorf("failed to remove existing edge for LWW resolution: %w", err)
				}
			} else {
				// Existing edge wins - reject new operation
				log.WithFields(log.Fields{"From": from, "To": to}).Debug("Existing edge wins LWW")
				return fmt.Errorf("existing edge wins LWW conflict resolution")
			}
		}
	}

	// Check for cycle after potential edge removal
	if c.validAttachment(from, to) != nil {
		return fmt.Errorf("adding edge would create a cycle: %s -> %s", from, to)
	}

	// Proceed with normal edge addition
	fromNode, ok := c.Nodes[from]
	if !ok {
		return fmt.Errorf("from node %s not found", from)
	}

	_, ok = c.Nodes[to]
	if !ok {
		return fmt.Errorf("to node %s not found", to)
	}

	// Check if we should promote the parent to an array before adding edge
	if c.shouldPromoteToArray(fromNode) && !fromNode.IsRoot {
		log.WithFields(log.Fields{
			"ParentID": from,
			"ChildID":  to,
			"Label":    label,
		}).Debug("Triggering array promotion during edge addition")

		err := c.promoteNodeToArray(fromNode, to, label, clientID)
		if err != nil {
			return err
		}
		return nil
	}

	// Standard edge addition
	latestVersion := fromNode.Clock[clientID]
	newVersion := latestVersion + 1
	return c.addEdgeWithVersion(from, to, label, clientID, newVersion)
}

func (c *TreeCRDT) AppendEdge(from, to core.NodeID, label string, clientID core.ClientID) error {
	return c.appendEdge(from, to, label, clientID, false)
}

func (c *TreeCRDT) appendEdge(from, to core.NodeID, label string, clientID core.ClientID, ignoreConflicts bool) error {
	if c.validAttachment(from, to) != nil {
		return fmt.Errorf("adding edge would create a cycle: %s -> %s or multiple parents", from, to)
	}

	fromNode, ok := c.Nodes[from]
	if !ok {
		return fmt.Errorf("AppendEdge: from parent node %s not found", from)
	}

	var lastSibling core.NodeID
	if len(fromNode.Edges) > 0 {
		// Use the last edge as anchor for right-side insert
		last := fromNode.Edges[len(fromNode.Edges)-1]
		lastSibling = last.To
	} else {
		// No siblings yet, insert at the beginning
		lastSibling = ""
	}

	newVersion := fromNode.Clock[clientID] + 1
	return c.insertEdgeWithVersion(from, to, label, lastSibling, false, clientID, newVersion)
}

func (c *TreeCRDT) PrependEdge(from, to core.NodeID, label string, clientID core.ClientID) error {
	if c.validAttachment(from, to) != nil {
		return fmt.Errorf("adding edge would create a cycle: %s -> %s or multiple parents", from, to)
	}

	node, ok := c.Nodes[from]
	if !ok {
		return fmt.Errorf("PrependEdge: parent node %s not found", from)
	}

	var firstSibling core.NodeID
	if len(node.Edges) > 0 {
		// Use the first edge as anchor for left-side insert
		first := node.Edges[0]
		firstSibling = first.To
	} else {
		// No siblings yet, insert at the beginning
		firstSibling = ""
	}

	newVersion := node.Clock[clientID] + 1
	return c.insertEdgeWithVersion(from, to, label, firstSibling, true /* left */, clientID, newVersion)
}

func (c *TreeCRDT) InsertEdgeLeft(from, to core.NodeID, label string, sibling core.NodeID, clientID core.ClientID) error {
	if c.validAttachment(from, to) != nil {
		return fmt.Errorf("adding edge would create a cycle: %s -> %s or multiple parents", from, to)
	}

	node, ok := c.Nodes[from]
	if !ok {
		return fmt.Errorf("InsertEdge: parent node %s not found", from)
	}
	latestVersion := node.Clock[clientID]
	newVersion := latestVersion + 1

	return c.insertEdgeWithVersion(from, to, label, sibling, true, clientID, newVersion)
}

func (c *TreeCRDT) InsertEdgeRight(from, to core.NodeID, label string, sibling core.NodeID, clientID core.ClientID) error {
	if c.validAttachment(from, to) != nil {
		return fmt.Errorf("adding edge would create a cycle: %s -> %s or multiple parents", from, to)
	}

	node, ok := c.Nodes[from]
	if !ok {
		return fmt.Errorf("InsertEdge: parent node %s not found", from)
	}
	latestVersion := node.Clock[clientID]
	newVersion := latestVersion + 1

	return c.insertEdgeWithVersion(from, to, label, sibling, false, clientID, newVersion)
}

func (c *TreeCRDT) insertEdgeWithVersion(from, to core.NodeID, label string, sibling core.NodeID, left bool, clientID core.ClientID, newVersion int) error {
	node, ok := c.Nodes[from]
	if !ok {
		return fmt.Errorf("insertWithVersion: parent node %s not found", from)
	}

	newClock := vectorclock.CopyClock(node.Clock)
	newClock[clientID] = newVersion

	// Sort edges for position lookup
	sorted := make([]*EdgeCRDT, len(node.Edges))
	copy(sorted, node.Edges)
	sortEdgesByLSEQ(sorted)

	var leftPos, rightPos lseq.Position
	found := false

	if sibling == "" || len(sorted) == 0 {
		// Insert at beginning
		leftPos = []int{}
		rightPos = []int{lseq.Base}
	} else {
		for i, e := range sorted {
			if e.To == sibling {
				found = true
				if left {
					// Insert to the left of sibling
					if i > 0 {
						leftPos = sorted[i-1].LSEQPosition
					} else {
						leftPos = []int{}
					}
					rightPos = e.LSEQPosition
				} else {
					// Insert to the right of sibling
					leftPos = e.LSEQPosition
					if i+1 < len(sorted) {
						rightPos = sorted[i+1].LSEQPosition
					} else {
						rightPos = []int{lseq.Base}
					}
				}
				break
			}
		}
		if !found {
			leftPos = []int{}
			rightPos = []int{lseq.Base}
		}
	}

	newPos := lseq.GeneratePositionBetweenLSEQ(leftPos, rightPos)

	edge := &EdgeCRDT{
		From:         from,
		To:           to,
		Label:        label,
		LSEQPosition: newPos,
	}
	node.Edges = append(node.Edges, edge)
	sortEdgesByLSEQ(node.Edges)

	node.Clock = newClock
	node.Owner = clientID

	child := c.Nodes[to]
	if child == nil {
		return fmt.Errorf("cannot add edge, child node %s not found", to)
	}
	child.ParentID = from

	c.notifySubscribers(from, EventAdded)

	log.WithFields(log.Fields{
		"NodeID":       from,
		"To":           to,
		"Sibling":      sibling,
		"Left":         left,
		"LSEQPosition": newPos,
		"Version":      newVersion,
	}).Debug("InsertEdge succeeded")

	return nil
}

func (c *TreeCRDT) GetSibling(parentNodeID core.NodeID, index int) (*NodeCRDT, error) {
	node, ok := c.Nodes[parentNodeID]
	if !ok {
		return nil, fmt.Errorf("cannot find node: %s", parentNodeID)
	}

	if len(node.Edges) == 0 {
		return nil, fmt.Errorf("cannot find sibling node, no edges")
	}

	// Sort edges by LSEQ
	sorted := make([]*EdgeCRDT, len(node.Edges))
	copy(sorted, node.Edges)
	sortEdgesByLSEQ(sorted)

	if index < 0 || index >= len(sorted) {
		return nil, fmt.Errorf("sibling index %d out of bounds", index)
	}

	siblingID := sorted[index].To
	sibling, exists := c.Nodes[siblingID]
	if !exists {
		return nil, fmt.Errorf("sibling node %s not found in CRDT tree", siblingID)
	}

	return sibling, nil
}

func (c *TreeCRDT) removeEdgeWithVersion(from, to core.NodeID, clientID core.ClientID, newVersion int, ignoreConflicts bool) error {
	fromNode, ok := c.Nodes[from]
	if !ok {
		return fmt.Errorf("cannot remove edge, from node %s not found", from)
	}
	toNode, ok := c.Nodes[from]
	if !ok {
		return fmt.Errorf("cannot remove edge, to node %s not found", from)
	}

	// Prepare the new clock
	newClock := vectorclock.CopyClock(fromNode.Clock)
	newClock[clientID] = newVersion

	// Resolve clock conflict
	winningClock, _ := vectorclock.ResolveConflict(fromNode.Clock, newClock, fromNode.Owner, clientID, false)

	if vectorclock.ClocksEqual(winningClock, newClock) || ignoreConflicts {
		// New clock wins -> allow edge removal
		newEdges := []*EdgeCRDT{}
		for _, edge := range fromNode.Edges {
			if !(edge.To == to) {
				newEdges = append(newEdges, edge)
			}
		}
		fromNode.Edges = newEdges
		fromNode.Clock = newClock
		fromNode.Owner = clientID

		toNode.ParentID = "" // Unlink child node from parent

		c.notifySubscribers(fromNode.ID, EventRemoved)

		log.WithFields(log.Fields{
			"NodeID":  from,
			"To":      to,
			"Version": newVersion}).Debug("Edge removed")
	} else {
		log.WithFields(log.Fields{
			"NodeID":        from,
			"To":            to,
			"FromNodeClock": fromNode.Clock,
			"NewClock":      newClock,
			"Version":       newVersion}).Error("Edge remove ignored due to conflict")
		return fmt.Errorf("cannot remove edge, conflict detected: %s", from)
	}

	return nil
}

func (c *TreeCRDT) RemoveEdge(from, to core.NodeID, clientID core.ClientID) error {
	fromNode, ok := c.Nodes[from]
	if !ok {
		return fmt.Errorf("cannot remove edge, from node %s not found", from)
	}
	latestVersion := fromNode.Clock[clientID]
	newVersion := latestVersion + 1

	return c.removeEdgeWithVersion(from, to, clientID, newVersion, false)
}

func (n *NodeCRDT) GetLiteral() (interface{}, error) {
	if !n.IsLiteral {
		return nil, fmt.Errorf("getLiteral: node %s is not a literal", n.ID)
	}
	return n.LiteralValue, nil
}

func (n *NodeCRDT) MarkDeleted(clientID core.ClientID) error {
	// Find max version for this client
	maxVersion := 0
	for _, v := range n.Clock {
		if v > maxVersion {
			maxVersion = v
		}
	}
	version := maxVersion + 1

	return n.markDeletedWithVersion(clientID, version)
}

func (n *NodeCRDT) markDeletedWithVersion(clientID core.ClientID, version int) error {
	currentClock := n.Clock
	newClock := make(vectorclock.VectorClock)
	newClock[clientID] = version

	winningClock, winningOwner := vectorclock.ResolveConflict(currentClock, newClock, n.Owner, clientID, false)

	if vectorclock.ClocksEqual(winningClock, newClock) && winningOwner == clientID {
		n.IsLiteral = true
		n.Clock = newClock
		n.Owner = clientID
		n.IsDeleted = true
		log.WithFields(log.Fields{
			"NodeID":               n.ID,
			"NodeClock":            currentClock,
			"NewClock":             newClock,
			"WinningClock":         winningClock,
			"WinningOwner":         winningOwner,
			"AttemptedDeleteValue": true,
			"ClientID":             clientID}).Debug("Set deleted flag")

		n.tree.notifySubscribers(n.ID, EventUpdated)
	} else {
		log.WithFields(log.Fields{
			"NodeID":               n.ID,
			"AttemptedDeleteValue": true,
			"ClientID":             clientID,
			"NodeClock":            currentClock,
			"NewClock":             newClock,
			"WinningClock":         winningClock,
			"ExistingOwner":        n.Owner,
			"WinningOwner":         winningOwner}).Debug("Delete set ignored due to conflict")
		return fmt.Errorf("cannot set deleted flag, conflict detected: %s", n.ID)
	}

	return nil
}

// Tidy removes all nodes that are not referenced by any edges.
//
// WARNING:
// - This function should NOT be called automatically after every change.
// - In CRDTs, a node that looks "orphaned" now may be referenced later by concurrent operations.
//
// Recommended usage:
//   - Call Tidy() manually after a batch of operations is complete,
//     when the CRDT tree is known to be stable.
//   - Optionally call Tidy() periodically (e.g., background maintenance) or before persisting to disk.
//
// This helps keep the CRDT tree compact without risking consistency.
// shouldPromoteToArray checks if a node should be promoted to an array
// when adding a new child. This is the key fix for consistent array promotion.
func (c *TreeCRDT) shouldPromoteToArray(parent *NodeCRDT) bool {
	// Promotion conditions:
	// 1. Parent has exactly 1 child (adding a second triggers promotion)
	// 2. Parent is not already a Map or Array
	// 3. Parent is not the root (root should always accept multiple children)

	if parent.IsRoot {
		return false // Root never gets promoted
	}

	if parent.IsMap || parent.IsArray {
		return false // Already a container type
	}

	if len(parent.Edges) == 1 {
		return true // Has one child, adding second should trigger promotion
	}

	return false
}

// promoteNodeToArray promotes a node to an array, preserving its existing children
// This ensures consistent behavior regardless of operation timing
func (c *TreeCRDT) promoteNodeToArray(parent *NodeCRDT, newChildID core.NodeID, edgeLabel string, clientID core.ClientID) error {
	if len(parent.Edges) != 1 {
		return nil // No promotion needed
	}

	existingEdge := parent.Edges[0]
	existingChild := c.Nodes[existingEdge.To]

	// Create the new child node if it doesn't exist
	newChild, ok := c.Nodes[newChildID]
	if !ok {
		// The new child should have been created before calling this function
		return fmt.Errorf("new child node %s not found", newChildID)
	}

	// Create promoted array node
	arrayNode := c.CreateNode("arr", Array, parent.Owner)
	arrayNode.IsArray = true
	arrayNode.IsPromoted = true

	// Find parent of the node being promoted
	var grandParent *NodeCRDT
	var parentEdgeLabel string
	for _, node := range c.Nodes {
		for _, edge := range node.Edges {
			if edge.To == parent.ID {
				grandParent = node
				parentEdgeLabel = edge.Label
				break
			}
		}
		if grandParent != nil {
			break
		}
	}

	if grandParent == nil {
		log.WithField("NodeID", parent.ID).Error("Cannot find parent for promotion")
		return nil
	}

	// Remove existing edge from grandparent to parent
	err := c.removeEdgeWithVersion(grandParent.ID, parent.ID, parent.Owner, parent.Clock[parent.Owner], true)
	if err != nil {
		log.WithError(err).Error("Failed to remove edge during promotion")
		return err
	}

	// Add edge from grandparent to promoted array
	// Use addEdgeWithVersion directly to avoid recursive promotion checks
	err = c.addEdgeWithVersion(grandParent.ID, arrayNode.ID, parentEdgeLabel, clientID, grandParent.Clock[clientID]+1)
	if err != nil {
		log.WithError(err).Error("Failed to add edge to promoted array")
		return err
	}

	// Sort children by NodeID for deterministic ordering (for now)
	// TODO: Use vector clock resolution for ordering
	children := []*NodeCRDT{existingChild, newChild}
	sort.Slice(children, func(i, j int) bool {
		return children[i].ID < children[j].ID
	})

	// Add both children to the promoted array
	for _, child := range children {
		if child.ID == existingChild.ID {
			// Re-add existing child
			err = c.AppendEdge(arrayNode.ID, child.ID, "", parent.Owner)
		} else {
			// Add new child
			err = c.AppendEdge(arrayNode.ID, child.ID, edgeLabel, clientID)
		}
		if err != nil {
			log.WithError(err).Error("Failed to add child to promoted array")
			return err
		}
	}

	return nil
}

func (c *TreeCRDT) Tidy() {
	referenced := make(map[core.NodeID]bool)

	// Mark all referenced nodes (target of edges)
	for _, node := range c.Nodes {
		for _, edge := range node.Edges {
			referenced[edge.To] = true
		}
	}

	// Always preserve the root node
	referenced[c.Root.ID] = true

	// Now delete all nodes that are unreferenced
	for id := range c.Nodes {
		if !referenced[id] {
			delete(c.Nodes, id)
			log.WithFields(log.Fields{"NodeID": id}).Debug("Purged unreferenced node")
		}
	}

	// Unlink deleted nodes from their parents
	for _, node := range c.Nodes {
		// Check if any child has the deleted flag set
		newEdges := make([]*EdgeCRDT, 0)
		for _, edge := range node.Edges {
			child, exists := c.Nodes[edge.To]
			if exists && !child.IsDeleted {
				newEdges = append(newEdges, edge)
			} else {
				log.WithFields(log.Fields{
					"NodeID":  node.ID,
					"ChildID": edge.To,
					"Deleted": child.IsDeleted,
				}).Debug("Unlinking deleted child node")
			}
		}
		node.Edges = newEdges
	}

	// Delete all deleted nodes
	for id, node := range c.Nodes {
		if node.IsDeleted {
			delete(c.Nodes, id)
			log.WithFields(log.Fields{"NodeID": id}).Debug("Purged deleted node")
		}
	}

}

func (c *TreeCRDT) Merge(c2 *TreeCRDT) error {
	return c.merge(c2, false, "")
}

func (c *TreeCRDT) SecureMerge(c2 *TreeCRDT, prvKey string) error {
	// Step 1: Clone local tree for pre-validation
	c1Copy, err := c.Clone()
	if err != nil {
		log.WithFields(log.Fields{
			"Error": err,
		}).Error("Failed to clone CRDT tree for merge")
		return fmt.Errorf("failed to clone CRDT tree for merge: %w", err)
	}

	// Step 2: Simulate merge on the clone
	err = c1Copy.merge(c2, true, prvKey)
	if err != nil {
		log.WithFields(log.Fields{
			"Error": err,
		}).Error("Failed to merge CRDT trees")
		return fmt.Errorf("failed to merge CRDT trees: %w", err)
	}

	// Step 3: Verify tree FIRST — before ABACPolicy merge!
	err = c1Copy.VerifyTree()
	if err != nil {
		log.WithFields(log.Fields{
			"Error": err,
		}).Error("Failed to verify remote CRDT tree BEFORE ABACPolicy merge")
		return fmt.Errorf("failed to verify remote CRDT tree BEFORE ABACPolicy merge: %w", err)
	}

	// Step 4: Now safe to merge ABACPolicy
	err = c1Copy.ABACPolicy.Merge(c2.ABACPolicy)
	if err != nil {
		return fmt.Errorf("failed to merge ABACPolicy in clone: %w", err)
	}

	// Step 5: Verify merged tree + ABAC
	err = c1Copy.VerifyTree()
	if err != nil {
		log.WithFields(log.Fields{
			"Error": err,
		}).Error("Failed to verify remote CRDT tree before merge")
		return fmt.Errorf("failed to verify remote CRDT tree before merge: %w", err)
	}

	// Step 6: Apply merge to live tree
	err = c.merge(c2, true, prvKey)
	if err != nil {
		log.WithFields(log.Fields{
			"Error": err,
		}).Error("Failed to apply merge to live CRDT tree")
		return fmt.Errorf("failed to apply merge to live CRDT tree: %w", err)
	}

	// Step 7: Apply ABACPolicy merge to live tree
	err = c.ABACPolicy.Merge(c2.ABACPolicy)
	if err != nil {
		log.WithFields(log.Fields{
			"Error": err,
		}).Error("Failed to merge ABACPolicy to live tree")
		return fmt.Errorf("failed to merge ABACPolicy to live tree: %w", err)
	}

	return nil
}

func (c *TreeCRDT) merge(c2 *TreeCRDT, secure bool, prvKey string) error {
	force := false
	promotions := make(map[core.NodeID]core.NodeID) // fromNodeID -> arrayNodeID

	for id, remote := range c2.Nodes {
		local, exists := c.Nodes[id]
		if !exists {
			nodeType := Literal
			if remote.IsArray {
				nodeType = Array
			} else if remote.IsMap {
				nodeType = Map
			}

			// TODO: this code is duplicated in cloneNodeFromRemote
			cloned := newNodeFromID(id, nodeType, c)
			cloned.IsLiteral = remote.IsLiteral
			cloned.IsMap = remote.IsMap
			cloned.ParentID = remote.ParentID
			cloned.IsArray = remote.IsArray
			cloned.IsPromoted = remote.IsPromoted
			cloned.LiteralValue = remote.LiteralValue
			cloned.Clock = vectorclock.CopyClock(remote.Clock)
			cloned.Owner = remote.Owner
			cloned.IsDeleted = remote.IsDeleted
			cloned.IsRoot = remote.IsRoot
			cloned.Nonce = remote.Nonce
			cloned.Signature = remote.Signature
			// Copy array-specific metadata
			cloned.IsArrayRoot = remote.IsArrayRoot
			cloned.IsArrayElement = remote.IsArrayElement
			cloned.ArrayIndex = remote.ArrayIndex
			cloned.BTreeKey = remote.BTreeKey
			c.Nodes[id] = cloned
			local = cloned
		}

		mergedClock := vectorclock.MergeClocks(local.Clock, remote.Clock)
		mergedOwner := lowestClientID(local.Owner, remote.Owner)

		if remote.IsLiteral {
			err := local.setLiteralWithVersion(remote.LiteralValue, remote.Owner, remote.Clock[remote.Owner])
			local.Nonce = remote.Nonce
			local.Signature = remote.Signature
			if err != nil {
				log.WithFields(log.Fields{
					"NodeID": remote.ID,
					"Error":  err,
				}).Warning("Failed to set literal value during merge")
				continue
			}
		}

		// Handle array element metadata merging
		if remote.IsArrayElement || remote.IsArrayRoot {
			c.mergeArrayElementMetadata(local, remote)
		}

		for _, re := range remote.Edges {
			if _, exists := c.Nodes[re.From]; !exists {
				c.cloneNodeFromRemote(c2, re.From)
			}
			if _, exists := c.Nodes[re.To]; !exists {
				c.cloneNodeFromRemote(c2, re.To)
			}

			fromNode := c.Nodes[re.From]
			toNode := c.Nodes[re.To]

			if c.edgeExists(fromNode, re.To) {
				continue
			}

			// Promote to array if single child and not already array or map
			if len(fromNode.Edges) == 1 && !fromNode.IsArray && !fromNode.IsMap {
				existingEdge := fromNode.Edges[0]
				existingChild := c.Nodes[existingEdge.To]

				arrayNode := c.CreateNode("arr", Array, fromNode.Owner)
				arrayNode.IsArray = true
				arrayNode.IsPromoted = true

				err := c.AddEdge(fromNode.ID, arrayNode.ID, "", fromNode.Owner)
				if err != nil {
					log.WithFields(log.Fields{
						"NodeID": fromNode.ID,
						"To":     arrayNode.ID,
						"Label":  "",
						"Error":  err,
					}).Error("AddEdge failed during promotion")
				}
				if secure {
					identity, err := crypto.CreateIdentityFromString(prvKey)
					if err != nil {
						log.WithFields(log.Fields{
							"NodeID": fromNode.ID,
							"Error":  err,
						}).Error("Failed to create identity for signing")
						return fmt.Errorf("failed to create identity for signing: %w", err)
					}
					err = arrayNode.Sign(identity)
					if err != nil {
						log.WithFields(log.Fields{
							"NodeID": fromNode.ID,
							"Error":  err,
						}).Error("Failed to sign promoted array node")
						return fmt.Errorf("failed to sign promoted array node: %w", err)
					}

				}
				_ = c.removeEdgeWithVersion(fromNode.ID, existingChild.ID, existingChild.Owner, existingChild.Clock[existingChild.Owner], true)

				// Insert both existing and new child sorted by NodeID
				children := []*NodeCRDT{existingChild, toNode}
				sort.Slice(children, func(i, j int) bool {
					return children[i].ID < children[j].ID
				})
				for _, child := range children {
					_ = c.AppendEdge(arrayNode.ID, child.ID, "", fromNode.Owner)
				}

				promotions[fromNode.ID] = arrayNode.ID
				continue
			}

			if arrayNodeID, promoted := promotions[re.From]; promoted {
				// Prevent duplicate
				if c.edgeExists(c.Nodes[arrayNodeID], re.To) {
					continue
				}

				// Ensure deterministic order using NodeID
				arrayNode := c.Nodes[arrayNodeID]
				existingChildren := make([]*EdgeCRDT, len(arrayNode.Edges))
				copy(existingChildren, arrayNode.Edges)
				sort.SliceStable(existingChildren, func(i, j int) bool {
					return existingChildren[i].To < existingChildren[j].To
				})

				inserted := false
				for i, edge := range existingChildren {
					if re.To < edge.To {
						var leftSiblingID core.NodeID
						if i > 0 {
							leftSiblingID = existingChildren[i-1].To
							_ = c.InsertEdgeRight(arrayNodeID, re.To, re.Label, leftSiblingID, remote.Owner)
						} else {
							_ = c.PrependEdge(arrayNodeID, re.To, re.Label, remote.Owner)
						}
						inserted = true
						break
					}
				}
				if !inserted {
					err := c.AppendEdge(arrayNodeID, re.To, re.Label, remote.Owner)
					if err != nil {
						log.WithFields(log.Fields{
							"NodeID": re.From,
							"To":     re.To,
							"Label":  re.Label,
							"Error":  err,
						}).Error("AppendEdge failed")
						if !force {
							return fmt.Errorf("AppendEdge failed: %w", err)
						}
					}
				}
				continue
			}

			if fromNode.IsArray {
				// Sort remote parent's edges to find left sibling
				remoteParent := c2.Nodes[re.From]
				sortEdgesByLSEQ(remoteParent.Edges)

				var siblingID core.NodeID
				var sibling *NodeCRDT = nil

				for i, edge := range remoteParent.Edges {
					if edge.To == re.To && i > 0 {
						siblingID = remoteParent.Edges[i-1].To
						break
					}
				}

				if siblingID != "" {
					var exists bool
					sibling, exists = c.Nodes[siblingID]
					if !exists {
						sibling = nil
					}
				}

				if sibling == nil {
					log.WithFields(log.Fields{
						"From":     re.From,
						"To":       re.To,
						"Label":    re.Label,
						"ClientID": remote.Owner,
					}).Debug("Appending edge to array (no left sibling found in local CRDT tree)")
					err := c.PrependEdge(re.From, re.To, re.Label, remote.Owner)
					if err != nil {
						log.WithFields(log.Fields{
							"NodeID": re.From,
							"To":     re.To,
							"Label":  re.Label,
							"Error":  err,
						}).Error("AppendEdge failed 2")
						if !force {
							return fmt.Errorf("AppendEdge failed 2: %w", err)
						}
					}
				} else {
					log.WithFields(log.Fields{
						"From":      re.From,
						"To":        re.To,
						"Label":     re.Label,
						"SiblingID": sibling.ID,
						"ClientID":  remote.Owner,
					}).Debug("Inserting edge to array (right of sibling from remote CRDT tree)")
					err := c.InsertEdgeRight(re.From, re.To, re.Label, sibling.ID, remote.Owner)
					if err != nil {
						log.WithFields(log.Fields{
							"NodeID": re.From,
							"To":     re.To,
							"Label":  re.Label,
							"Error":  err,
						}).Error("InsertEdgeLeft failed")
						if !force {
							return fmt.Errorf("InsertEdgeRight failed: %w", err)
						}
					}
				}

			} else {
				if !c.edgeExists(fromNode, re.To) {
					version := fromNode.Clock[remote.Owner] + 1
					err := c.addEdgeWithVersion(fromNode.ID, re.To, re.Label, remote.Owner, version)
					if err != nil {
						log.WithFields(log.Fields{
							"NodeID": re.From,
							"To":     re.To,
							"Label":  re.Label,
							"Error":  err,
						}).Error("AddEdgeWithVersion failed")
						if !force {
							return fmt.Errorf("AddEdgeWithVersion failed: %w", err)
						}
						continue
					}
				} else {
					log.WithFields(log.Fields{
						"From":     re.From,
						"To":       re.To,
						"Label":    re.Label,
						"ClientID": remote.Owner,
					}).Debug("Edge already exists, skipping")
					continue
				}
				_ = c.AddEdge(fromNode.ID, re.To, re.Label, remote.Owner)
			}
		}

		local.Clock = mergedClock
		local.Owner = mergedOwner
	}

	// Rebalance all arrays after merge to ensure proper convergence
	c.mergeArrayElements()
	
	c.normalize()
	return nil
}

func (c *TreeCRDT) cloneNodeFromRemote(c2 *TreeCRDT, id core.NodeID) {
	remote := c2.Nodes[id]
	nodeType := Literal
	if remote.IsArray {
		nodeType = Array
	} else if remote.IsMap {
		nodeType = Map
	}
	cloned := newNodeFromID(id, nodeType, c)
	cloned.IsLiteral = remote.IsLiteral
	cloned.IsMap = remote.IsMap
	cloned.IsArray = remote.IsArray
	cloned.IsPromoted = remote.IsPromoted
	cloned.LiteralValue = remote.LiteralValue
	cloned.Clock = vectorclock.CopyClock(remote.Clock)
	cloned.Owner = remote.Owner
	cloned.IsDeleted = remote.IsDeleted
	cloned.IsRoot = remote.IsRoot
	cloned.ParentID = remote.ParentID
	cloned.Nonce = remote.Nonce
	cloned.Signature = remote.Signature
	// Copy array-specific metadata
	cloned.IsArrayRoot = remote.IsArrayRoot
	cloned.IsArrayElement = remote.IsArrayElement
	cloned.ArrayIndex = remote.ArrayIndex
	cloned.BTreeKey = remote.BTreeKey
	c.Nodes[id] = cloned
}

func (c *TreeCRDT) edgeExists(node *NodeCRDT, to core.NodeID) bool {
	for _, e := range node.Edges {
		if e.To == to {
			return true
		}
	}
	return false
}

func (c *TreeCRDT) normalize() {
	log.Debug("Normalizing CRDT tree")
	sortEdgesByLSEQ(c.Root.Edges)
	for _, node := range c.Nodes {
		sortEdgesByLSEQ(node.Edges)
	}
}

func (c *TreeCRDT) validAttachment(from, to core.NodeID) error {
	if from == to {
		return fmt.Errorf("cannot attach node %s to itself", from)
	}

	// 1. Check for cycle
	visited := make(map[core.NodeID]bool)
	var dfs func(core.NodeID) bool
	dfs = func(id core.NodeID) bool {
		if id == from {
			return true
		}
		visited[id] = true
		node := c.Nodes[id]
		for _, edge := range node.Edges {
			if !visited[edge.To] && dfs(edge.To) {
				return true
			}
		}
		return false
	}
	if dfs(to) {
		return fmt.Errorf("adding edge from %s to %s would create a cycle", from, to)
	}

	// 2. Check if `to` already has a parent
	for _, parent := range c.Nodes {
		for _, edge := range parent.Edges {
			if edge.To == to {
				return fmt.Errorf("node %s already has a parent", to)
			}
		}
	}

	return nil
}

// validAttachmentWithLWW performs attachment validation with Last Writer Wins semantics
// It allows operations that would normally create conflicts if they can be resolved by LWW
func (c *TreeCRDT) validAttachmentWithLWW(from, to core.NodeID, clientID core.ClientID) error {
	if from == to {
		return fmt.Errorf("cannot attach node %s to itself", from)
	}

	// Check if `to` already has a parent - this is where LWW resolution can help
	existingParent := core.NodeID("")
	for _, parent := range c.Nodes {
		for _, edge := range parent.Edges {
			if edge.To == to {
				existingParent = parent.ID
				break
			}
		}
		if existingParent != "" {
			break
		}
	}

	// If node has existing parent, we can allow the operation if it can win via LWW
	if existingParent != "" && existingParent != from {
		// Allow the operation - LWW resolution will handle it during merge
		// This enables move operations to succeed individually and be resolved during merge
		return nil
	}

	// Standard cycle detection, but with LWW consideration
	visited := make(map[core.NodeID]bool)
	var dfs func(core.NodeID) bool
	dfs = func(id core.NodeID) bool {
		if id == from {
			return true
		}
		visited[id] = true
		node := c.Nodes[id]
		for _, edge := range node.Edges {
			if !visited[edge.To] && dfs(edge.To) {
				return true
			}
		}
		return false
	}

	// For potential cycles, allow the operation if it could be resolved by LWW
	// The actual resolution will happen during merge
	if dfs(to) {
		// This would create a cycle, but in LWW semantics, we allow it
		// and let the merge process resolve which edges win
		return nil
	}

	return nil
}

// MoveNodeWithLWW implements a move operation with Last Writer Wins semantics
// It removes the node from its current parent and adds it to the new parent
// If conflicts arise, they are resolved using vector clock comparison
func (c *TreeCRDT) MoveNodeWithLWW(nodeID, newParentID core.NodeID, newLabel string, clientID core.ClientID) error {
	node, exists := c.GetNode(nodeID)
	if !exists {
		return fmt.Errorf("node %s not found", nodeID)
	}

	_, exists = c.GetNode(newParentID)
	if !exists {
		return fmt.Errorf("new parent %s not found", newParentID)
	}

	// If node already has this parent, nothing to do
	if node.ParentID == newParentID {
		return nil
	}

	// Check for self-cycle
	if nodeID == newParentID {
		return fmt.Errorf("cannot move node %s to itself", nodeID)
	}

	// Remove from current parent if it has one
	if node.ParentID != "" {
		err := c.RemoveEdge(node.ParentID, nodeID, clientID)
		if err != nil {
			return fmt.Errorf("failed to remove node from current parent: %w", err)
		}
	}

	// Add to new parent - use LWW-aware validation
	err := c.addEdgeWithLWW(newParentID, nodeID, newLabel, clientID)
	if err != nil {
		// If the LWW add fails, try to restore the old parent connection
		if node.ParentID != "" {
			_ = c.AddEdge(node.ParentID, nodeID, "", clientID) // Best effort restore
		}
		return fmt.Errorf("failed to add node to new parent: %w", err)
	}

	return nil
}

// addEdgeWithLWW adds an edge with LWW conflict resolution
func (c *TreeCRDT) addEdgeWithLWW(from, to core.NodeID, label string, clientID core.ClientID) error {
	// Use LWW-aware validation instead of strict validation
	if err := c.validAttachmentWithLWW(from, to, clientID); err != nil {
		return err
	}

	fromNode, ok := c.Nodes[from]
	if !ok {
		return fmt.Errorf("from node %s not found", from)
	}

	_, ok = c.Nodes[to]
	if !ok {
		return fmt.Errorf("to node %s not found", to)
	}

	// Check if adding this edge would resolve a conflict via LWW
	existingParentID := c.findParentNode(to)
	if existingParentID != "" && existingParentID != from {
		// Node already has a parent - remove the old edge first based on LWW
		existingParent, exists := c.GetNode(existingParentID)
		if exists {
			// Compare vector clocks to determine which edge should win
			fromNodeClock := fromNode.Clock[clientID]
			existingParentClock := existingParent.Clock[existingParent.Owner]

			// If the new operation has a higher clock, it wins
			if fromNodeClock > existingParentClock ||
				(fromNodeClock == existingParentClock && clientID < existingParent.Owner) {
				// New edge wins - remove old edge
				err := c.RemoveEdge(existingParentID, to, clientID)
				if err != nil {
					return fmt.Errorf("failed to remove existing edge during LWW resolution: %w", err)
				}
			} else {
				// Existing edge wins - reject new edge
				return fmt.Errorf("existing edge wins LWW resolution")
			}
		}
	}

	// Standard edge addition
	latestVersion := fromNode.Clock[clientID]
	newVersion := latestVersion + 1
	return c.addEdgeWithVersion(from, to, label, clientID, newVersion)
}

// findParentNode finds the parent of a given node
func (c *TreeCRDT) findParentNode(nodeID core.NodeID) core.NodeID {
	for _, parent := range c.Nodes {
		for _, edge := range parent.Edges {
			if edge.To == nodeID {
				return parent.ID
			}
		}
	}
	return ""
}

func (c *TreeCRDT) ValidateTree() error {
	if c.Root == nil {
		return fmt.Errorf("Tree must have a root node")
	}

	parentMap := make(map[core.NodeID]core.NodeID)
	visited := make(map[core.NodeID]bool)

	// Ensure exactly one root node
	rootCount := 0
	for _, node := range c.Nodes {
		if node.IsRoot {
			rootCount++
		}
	}
	if rootCount != 1 {
		log.WithField("RootCount", rootCount).Debug("Invalid root node count")
		return fmt.Errorf("Tree must have exactly one root node, found %d", rootCount)
	}

	// Helper: Ensure node has exactly one type (Map, Array, or Literal) — skip root
	validateNodeType := func(node *NodeCRDT) error {
		if node.IsRoot {
			return nil
		}

		types := 0
		if node.IsMap {
			types++
		}
		if node.IsArray {
			types++
		}
		if node.IsLiteral {
			types++
		}
		if types != 1 {
			log.WithFields(log.Fields{
				"NodeID":    node.ID,
				"IsMap":     node.IsMap,
				"IsArray":   node.IsArray,
				"IsLiteral": node.IsLiteral,
			}).Debug("Node has invalid type combination")
			return fmt.Errorf("node %s must have exactly one type: Map, Array, or Literal", node.ID)
		}
		return nil
	}

	var dfs func(current core.NodeID, ancestors map[core.NodeID]bool) error
	dfs = func(current core.NodeID, ancestors map[core.NodeID]bool) error {
		if ancestors[current] {
			log.WithField("NodeID", current).Debug("Cycle detected")
			return fmt.Errorf("cycle detected at node %s", current)
		}
		if visited[current] {
			return nil
		}
		visited[current] = true

		node, exists := c.Nodes[current]
		if !exists {
			log.WithField("NodeID", current).Debug("Node not found")
			return fmt.Errorf("node %s not found in tree", current)
		}

		// Validate type (non-root nodes only)
		if err := validateNodeType(node); err != nil {
			return err
		}

		// Literals must not have children
		if node.IsLiteral && len(node.Edges) > 0 {
			log.WithField("NodeID", current).Debug("Literal node has children")
			return fmt.Errorf("literal node %s must not have children", current)
		}

		ancestors[current] = true
		for _, edge := range node.Edges {
			childID := edge.To

			childNode, ok := c.Nodes[childID]
			if !ok {
				log.WithField("ChildID", childID).Debug("Edge to non-existent node")
				return fmt.Errorf("edge to non-existent node: %s", childID)
			}

			// Root must not have a parent
			if childNode.IsRoot {
				log.WithField("ParentNodeID", current).Debug("Root node has a parent")
				return fmt.Errorf("root node must not have a parent")
			}

			if existingParent, ok := parentMap[childID]; ok && existingParent != current {
				log.WithFields(log.Fields{
					"ChildID":        childID,
					"ExistingParent": existingParent,
					"CurrentParent":  current,
				}).Debug("Multiple parents detected")
				return fmt.Errorf("node %s has multiple parents: %s and %s", childID, existingParent, current)
			}
			parentMap[childID] = current

			if err := dfs(childID, ancestors); err != nil {
				return err
			}
		}
		delete(ancestors, current)
		return nil
	}

	// Start DFS from declared root node
	if err := dfs(c.Root.ID, make(map[core.NodeID]bool)); err != nil {
		return err
	}

	// Ensure all nodes were visited (i.e. reachable from root)
	for id := range c.Nodes {
		if !visited[id] {
			log.WithField("NodeID", id).Debug("Unreachable node detected")
			return fmt.Errorf("unreachable node found: %s", id)
		}
	}

	return nil
}

func (c *TreeCRDT) VerifyTree() error {
	if c.ABACPolicy == nil {
		return fmt.Errorf("VerifyTree: ABACPolicy is not set")
	}

	// Step 1: Verify tree structure (optional but recommended)
	if err := c.ValidateTree(); err != nil {
		return fmt.Errorf("VerifyTree: tree structure invalid: %w", err)
	}

	// Step 2: For each node → verify signature and ABAC
	for id, node := range c.Nodes {
		if node.Signature == "" {
			return fmt.Errorf("VerifyTree: node %s has no signature", id)
		}
		recoveredID, err := node.Verify()
		if err != nil {
			return fmt.Errorf("VerifyTree: signature verification failed for node %s: %w", id, err)
		}

		// 2.2 Check ABACPolicy for ActionModify
		if !c.ABACPolicy.IsAllowed(recoveredID, abac.ActionModify, id) {
			return fmt.Errorf("VerifyTree: ABAC violation: client %s is not allowed to modify node %s", recoveredID, id)
		}
	}

	_, err := c.ABACPolicy.Verify()
	if err != nil {
		log.WithFields(log.Fields{
			"Error": err,
		}).Error("Failed to verify ABAC policy")
		return fmt.Errorf("VerifyTree: failed to compute ABAC policy hash: %w", err)
	}

	return nil
}

// type Mutation struct {
// 	NodeID   core.NodeID   `json:"nodeid"`
// 	Op       Operation     `json:"op"`
// 	Key      string        `json:"key,omitempty"`
// 	Value    interface{}   `json:"value,omitempty"`
// 	ClientID core.ClientID `json:"clientid"`
// 	Version  int           `json:"version"`
// }

func (c *TreeCRDT) ApplyMutation(mut Mutation) error {
	switch mut.Op {
	case OPSetLiteral:
		n := c.Nodes[mut.NodeID]
		if n == nil {
			log.WithFields(log.Fields{
				"NodeID":   mut.NodeID,
				"ClientID": mut.ClientID,
			}).Error("ApplyMutation: node not found for OPSetLiteral")
			return fmt.Errorf("ApplyMutation: node %s not found for OPSetLiteral", mut.NodeID)
		}
		err := n.applySetLiteralMutations(mut)
		if err != nil {
			log.WithFields(log.Fields{
				"NodeID":   mut.NodeID,
				"ClientID": mut.ClientID,
				"Error":    err,
			}).Error("ApplyMutation: failed to apply OPSetLiteral mutation")
			return fmt.Errorf("ApplyMutation: failed to apply OPSetLiteral mutation: %w", err)
		}
	}

	return nil
}

// GetVectorClock returns the current vector clock of the tree
func (c *TreeCRDT) GetVectorClock() vectorclock.VectorClock {
	clock := make(vectorclock.VectorClock)

	// Merge clocks from all nodes
	for _, node := range c.Nodes {
		clock = vectorclock.MergeClocks(clock, node.Clock)
	}

	return clock
}

