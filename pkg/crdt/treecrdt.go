package crdt

import (
	"errors"
	"fmt"
	"sort"

	"github.com/eislab-cps/synctree/internal/crypto"
	"github.com/eislab-cps/synctree/pkg/random"
	log "github.com/sirupsen/logrus"
)

type NodeID string
type NodeType int

const (
	Root NodeType = iota
	Array
	Map
	Literal
)

type NodeCRDT struct {
	tree         *TreeCRDT
	ID           NodeID      `json:"id"`
	ParentID     NodeID      `json:"parentid"`
	Edges        []*EdgeCRDT `json:"edges"`
	Clock        VectorClock `json:"clock"`
	Owner        ClientID    `json:"owner"`
	IsRoot       bool        `json:"isroot"`
	IsMap        bool        `json:"ismap"`
	IsArray      bool        `json:"isarray"`
	IsPromoted   bool        `json:"ispromoted"`
	IsLiteral    bool        `json:"isliteral"`
	LiteralValue interface{} `json:"litteralValue"`
	Nounce       string      `json:"nounce"`
	Signature    string      `json:"signature"`
	IsDeleted    bool        `json:"deleted"`
}

type EdgeCRDT struct {
	From         NodeID `json:"from"`
	To           NodeID `json:"to"`
	Label        string `json:"label"`
	LSEQPosition []int  `json:"lseqposition"`
}

type TreeCRDT struct {
	Root        *NodeCRDT            `json:"root"`
	Nodes       map[NodeID]*NodeCRDT `json:"nodes"`
	ABACPolicy  *ABACPolicy          `json:"abac"`
	Secure      bool                 `json:"secure"`
	subscribers []subscriber
}

func newTreeCRDT() *TreeCRDT {
	rootID := "root"
	root := &NodeCRDT{
		ID:     NodeID(rootID),
		Edges:  make([]*EdgeCRDT, 0),
		IsRoot: true,
	}
	c := &TreeCRDT{
		Root:  root,
		Nodes: make(map[NodeID]*NodeCRDT),
	}
	c.Nodes[c.Root.ID] = c.Root
	root.tree = c
	c.ABACPolicy = nil
	c.Secure = false

	return c
}

func (c *TreeCRDT) CreateAttachedNode(name string, nodeType NodeType, parentID NodeID, clientID ClientID) *NodeCRDT {
	id := generateRandomNodeID(name)
	node := c.getOrCreateNode(id, nodeType, clientID, 1)
	c.AddEdge(parentID, id, "", clientID)
	node.ParentID = parentID

	c.notifySubscribers(node.ID, EventAdded)

	return node
}

func (c *TreeCRDT) CreateNode(name string, nodeType NodeType, clientID ClientID) *NodeCRDT {
	id := generateRandomNodeID(name)
	node := c.getOrCreateNode(id, nodeType, clientID, 1)
	setNodeTypeFlags(node, nodeType)

	// Do not modify since it is not attached to the tree
	return node
}

func newNodeFromID(id NodeID, nodeType NodeType, tree *TreeCRDT) *NodeCRDT {
	node := &NodeCRDT{
		ID:    id,
		Edges: make([]*EdgeCRDT, 0),
		tree:  tree,
	}
	setNodeTypeFlags(node, nodeType)

	return node
}

func (c *TreeCRDT) getOrCreateNode(id NodeID, nodeType NodeType, clientID ClientID, version int) *NodeCRDT {
	if _, ok := c.Nodes[id]; !ok {
		node := newNodeFromID(id, nodeType, c)
		c.Nodes[id] = node
		node.Clock = make(VectorClock)
		node.Clock[clientID] = version
		node.Owner = clientID
	}
	return c.Nodes[id]
}

func (c *TreeCRDT) GetNode(id NodeID) (*NodeCRDT, bool) {
	node, ok := c.Nodes[id]
	if !ok {
		return nil, false
	}
	return node, true
}

func generateRandomNodeID(label string) NodeID {
	id := random.GenerateRandomID()
	id = label + "-" + id
	return NodeID(id)
}

// This functions only appends a new node to the tree, no need for conflict resolution
func (n *NodeCRDT) CreateMapNode(clientID ClientID) (*NodeCRDT, error) {
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

func (n *NodeCRDT) SetKeyValue(key string, value interface{}, clientID ClientID) (NodeID, error) {
	if !n.IsMap {
		return "", fmt.Errorf("SetKeyValue: node %s is not a map node", n.ID)
	}

	// Check if key already exists
	for _, edge := range n.Edges {
		if edge.Label == key {
			valueNodeID := edge.To
			valueNode, exists := n.tree.Nodes[valueNodeID]
			if !exists {
				return "", fmt.Errorf("SetKeyValue: missing node %s", valueNodeID)
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
			}

			valueNode.ParentID = n.ID // Ensure parent link is set

			return valueNodeID, err
		}
	}

	// Create new value node
	valueNodeID := generateRandomNodeID("val")
	valueNode := n.tree.getOrCreateNode(valueNodeID, Literal, clientID, 1)
	// setLiteralWithVersion will notify subscribers, when the values is updated
	if err := valueNode.setLiteralWithVersion(value, clientID, 1); err != nil {
		return "", err
	}

	// Link to map node with key label
	if err := n.tree.AddEdge(n.ID, valueNodeID, key, clientID); err != nil {
		return "", err
	}

	n.tree.notifySubscribers(valueNodeID, EventAdded)

	return valueNodeID, nil
}

func (n *NodeCRDT) RemoveKeyValue(key string, clientID ClientID) error {
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

func (c *TreeCRDT) addEdgeWithVersion(from, to NodeID, label string, clientID ClientID, newVersion int) error {
	fromNode, ok := c.Nodes[from]
	if !ok {
		return errors.New("Cannot add edge, from node not found: " + string(from))
	}

	toNode, ok := c.Nodes[to]
	if !ok {
		return errors.New("Cannot add edge, to node not found: " + string(from))
	}

	// Prepare the new clock
	newClock := copyClock(fromNode.Clock)
	newClock[clientID] = newVersion

	// Resolve clock conflict
	winningClock, winningOwner := resolveConflict(fromNode.Clock, newClock, fromNode.Owner, clientID, false)

	if clocksEqual(winningClock, newClock) && (clientID == winningOwner) {
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

func (c *TreeCRDT) AddEdge(from, to NodeID, label string, clientID ClientID) error {
	if c.validAttachment(from, to) != nil {
		return fmt.Errorf("Adding edge would create a cycle: %s -> %s or multiple parents", from, to)
	}

	fromNode, ok := c.Nodes[from]
	if !ok {
		return errors.New("Cannot add edge, from node not found: " + string(from))
	}

	latestVersion := fromNode.Clock[clientID]
	newVersion := latestVersion + 1

	return c.addEdgeWithVersion(from, to, label, clientID, newVersion)
}

func (c *TreeCRDT) AppendEdge(from, to NodeID, label string, clientID ClientID) error {
	return c.appendEdge(from, to, label, clientID, false)
}

func (c *TreeCRDT) appendEdge(from, to NodeID, label string, clientID ClientID, ignoreConflicts bool) error {
	if c.validAttachment(from, to) != nil {
		return fmt.Errorf("Adding edge would create a cycle: %s -> %s or multiple parents", from, to)
	}

	fromNode, ok := c.Nodes[from]
	if !ok {
		return fmt.Errorf("AppendEdge: from parent node %s not found", from)
	}

	var lastSibling NodeID
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

func (c *TreeCRDT) PrependEdge(from, to NodeID, label string, clientID ClientID) error {
	if c.validAttachment(from, to) != nil {
		return fmt.Errorf("Adding edge would create a cycle: %s -> %s or multiple parents", from, to)
	}

	node, ok := c.Nodes[from]
	if !ok {
		return fmt.Errorf("PrependEdge: parent node %s not found", from)
	}

	var firstSibling NodeID
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

func (c *TreeCRDT) InsertEdgeLeft(from, to NodeID, label string, sibling NodeID, clientID ClientID) error {
	if c.validAttachment(from, to) != nil {
		return fmt.Errorf("Adding edge would create a cycle: %s -> %s or multiple parents", from, to)
	}

	node, ok := c.Nodes[from]
	if !ok {
		return fmt.Errorf("InsertEdge: parent node %s not found", from)
	}
	latestVersion := node.Clock[clientID]
	newVersion := latestVersion + 1

	return c.insertEdgeWithVersion(from, to, label, sibling, true, clientID, newVersion)
}

func (c *TreeCRDT) InsertEdgeRight(from, to NodeID, label string, sibling NodeID, clientID ClientID) error {
	if c.validAttachment(from, to) != nil {
		return fmt.Errorf("Adding edge would create a cycle: %s -> %s or multiple parents", from, to)
	}

	node, ok := c.Nodes[from]
	if !ok {
		return fmt.Errorf("InsertEdge: parent node %s not found", from)
	}
	latestVersion := node.Clock[clientID]
	newVersion := latestVersion + 1

	return c.insertEdgeWithVersion(from, to, label, sibling, false, clientID, newVersion)
}

func (c *TreeCRDT) insertEdgeWithVersion(from, to NodeID, label string, sibling NodeID, left bool, clientID ClientID, newVersion int) error {
	node, ok := c.Nodes[from]
	if !ok {
		return fmt.Errorf("insertWithVersion: parent node %s not found", from)
	}

	newClock := copyClock(node.Clock)
	newClock[clientID] = newVersion

	// Sort edges for position lookup
	sorted := make([]*EdgeCRDT, len(node.Edges))
	copy(sorted, node.Edges)
	sortEdgesByLSEQ(sorted)

	var leftPos, rightPos Position
	found := false

	if sibling == "" || len(sorted) == 0 {
		// Insert at beginning
		leftPos = []int{}
		rightPos = []int{Base}
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
						rightPos = []int{Base}
					}
				}
				break
			}
		}
		if !found {
			leftPos = []int{}
			rightPos = []int{Base}
		}
	}

	newPos := generatePositionBetweenLSEQ(leftPos, rightPos)

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
		return fmt.Errorf("Cannot add edge, child node %s not found", to)
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

func (c *TreeCRDT) GetSibling(parentNodeID NodeID, index int) (*NodeCRDT, error) {
	node, ok := c.Nodes[parentNodeID]
	if !ok {
		return nil, fmt.Errorf("Cannot find node: %s", parentNodeID)
	}

	if len(node.Edges) == 0 {
		return nil, fmt.Errorf("Cannot find sibling node, no edges")
	}

	// Sort edges by LSEQ
	sorted := make([]*EdgeCRDT, len(node.Edges))
	copy(sorted, node.Edges)
	sortEdgesByLSEQ(sorted)

	if index < 0 || index >= len(sorted) {
		return nil, fmt.Errorf("Sibling index %d out of bounds", index)
	}

	siblingID := sorted[index].To
	sibling, exists := c.Nodes[siblingID]
	if !exists {
		return nil, fmt.Errorf("Sibling node %s not found in CRDT tree", siblingID)
	}

	return sibling, nil
}

func (c *TreeCRDT) removeEdgeWithVersion(from, to NodeID, clientID ClientID, newVersion int, ignoreConflicts bool) error {
	fromNode, ok := c.Nodes[from]
	if !ok {
		return fmt.Errorf("Cannot remove edge, from node %s not found", from)
	}
	toNode, ok := c.Nodes[from]
	if !ok {
		return fmt.Errorf("Cannot remove edge, to node %s not found", from)
	}

	// Prepare the new clock
	newClock := copyClock(fromNode.Clock)
	newClock[clientID] = newVersion

	// Resolve clock conflict
	winningClock, _ := resolveConflict(fromNode.Clock, newClock, fromNode.Owner, clientID, false)

	if clocksEqual(winningClock, newClock) || ignoreConflicts {
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
		return fmt.Errorf("Cannot remove edge, conflict detected: %s", from)
	}

	return nil
}

func (c *TreeCRDT) RemoveEdge(from, to NodeID, clientID ClientID) error {
	fromNode, ok := c.Nodes[from]
	if !ok {
		return fmt.Errorf("Cannot remove edge, from node %s not found", from)
	}
	latestVersion := fromNode.Clock[clientID]
	newVersion := latestVersion + 1

	return c.removeEdgeWithVersion(from, to, clientID, newVersion, false)
}

func (n *NodeCRDT) GetLiteral() (interface{}, error) {
	if !n.IsLiteral {
		return nil, fmt.Errorf("GetLiteral: node %s is not a literal", n.ID)
	}
	return n.LiteralValue, nil
}

func (n *NodeCRDT) SetLiteral(value interface{}, clientID ClientID) error {
	// Find max version for this client
	maxVersion := 0
	for _, v := range n.Clock {
		if v > maxVersion {
			maxVersion = v
		}
	}
	version := maxVersion + 1

	return n.setLiteralWithVersion(value, clientID, version)
}

func (n *NodeCRDT) setLiteralWithVersion(value interface{}, clientID ClientID, version int) error {
	value = normalizeNumber(value) // If value is a number, normalize it to float64 since JS uses float64 for all numbers
	currentClock := n.Clock
	newClock := make(VectorClock)
	newClock[clientID] = version

	winningClock, winningOwner := resolveConflict(currentClock, newClock, n.Owner, clientID, false)

	if clocksEqual(winningClock, newClock) && winningOwner == clientID {
		n.IsLiteral = true
		n.LiteralValue = value
		n.Clock = newClock
		n.Owner = clientID
		log.WithFields(log.Fields{
			"NodeID":       n.ID,
			"NodeClock":    currentClock,
			"NewClock":     newClock,
			"WinningClock": winningClock,
			"WinningOwner": winningOwner,
			"ClientID":     clientID,
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
			"ClientID":              clientID,
			"NodeClock":             currentClock,
			"NewClock":              newClock,
			"WinningClock":          winningClock,
			"ExistingOwner":         n.Owner,
			"WinningOwner":          winningOwner}).Debug("Literal set ignored due to conflict")
		return fmt.Errorf("Cannot set literal value, conflict detected: %s", n.ID)
	}

	return nil
}

func (n *NodeCRDT) MarkDeleted(clientID ClientID) error {
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

func (n *NodeCRDT) markDeletedWithVersion(clientID ClientID, version int) error {
	currentClock := n.Clock
	newClock := make(VectorClock)
	newClock[clientID] = version

	winningClock, winningOwner := resolveConflict(currentClock, newClock, n.Owner, clientID, false)

	if clocksEqual(winningClock, newClock) && winningOwner == clientID {
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
		return fmt.Errorf("Cannot set deleted flag, conflict detected: %s", n.ID)
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
func (c *TreeCRDT) Tidy() {
	referenced := make(map[NodeID]bool)

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
		return fmt.Errorf("Failed to clone CRDT tree for merge: %w", err)
	}

	// Step 2: Simulate merge on the clone
	err = c1Copy.merge(c2, true, prvKey)
	if err != nil {
		log.WithFields(log.Fields{
			"Error": err,
		}).Error("Failed to merge CRDT trees")
		return fmt.Errorf("Failed to merge CRDT trees: %w", err)
	}

	// Step 3: Verify tree FIRST — before ABACPolicy merge!
	err = c1Copy.VerifyTree()
	if err != nil {
		log.WithFields(log.Fields{
			"Error": err,
		}).Error("Failed to verify remote CRDT tree BEFORE ABACPolicy merge")
		return fmt.Errorf("Failed to verify remote CRDT tree BEFORE ABACPolicy merge: %w", err)
	}

	// Step 4: Now safe to merge ABACPolicy
	err = c1Copy.ABACPolicy.Merge(c2.ABACPolicy)
	if err != nil {
		return fmt.Errorf("Failed to merge ABACPolicy in clone: %w", err)
	}

	// Step 5: Verify merged tree + ABAC
	err = c1Copy.VerifyTree()
	if err != nil {
		log.WithFields(log.Fields{
			"Error": err,
		}).Error("Failed to verify remote CRDT tree before merge")
		return fmt.Errorf("Failed to verify remote CRDT tree before merge: %w", err)
	}

	// Step 6: Apply merge to live tree
	err = c.merge(c2, true, prvKey)
	if err != nil {
		log.WithFields(log.Fields{
			"Error": err,
		}).Error("Failed to apply merge to live CRDT tree")
		return fmt.Errorf("Failed to apply merge to live CRDT tree: %w", err)
	}

	// Step 7: Apply ABACPolicy merge to live tree
	err = c.ABACPolicy.Merge(c2.ABACPolicy)
	if err != nil {
		log.WithFields(log.Fields{
			"Error": err,
		}).Error("Failed to merge ABACPolicy to live tree")
		return fmt.Errorf("Failed to merge ABACPolicy to live tree: %w", err)
	}

	return nil
}

func (c *TreeCRDT) merge(c2 *TreeCRDT, secure bool, prvKey string) error {
	force := false
	promotions := make(map[NodeID]NodeID) // fromNodeID -> arrayNodeID

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
			cloned.Clock = copyClock(remote.Clock)
			cloned.Owner = remote.Owner
			cloned.IsDeleted = remote.IsDeleted
			cloned.IsRoot = remote.IsRoot
			cloned.Nounce = remote.Nounce
			cloned.Signature = remote.Signature
			c.Nodes[id] = cloned
			local = cloned
		}

		mergedClock := mergeClocks(local.Clock, remote.Clock)
		mergedOwner := lowestClientID(local.Owner, remote.Owner)

		if remote.IsLiteral {
			err := local.setLiteralWithVersion(remote.LiteralValue, remote.Owner, remote.Clock[remote.Owner])
			local.Nounce = remote.Nounce
			local.Signature = remote.Signature
			if err != nil {
				log.WithFields(log.Fields{
					"NodeID": remote.ID,
					"Error":  err,
				}).Warning("Failed to set literal value during merge")
				continue
			}
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
					identity, err := crypto.CreateIdendityFromString(prvKey)
					if err != nil {
						log.WithFields(log.Fields{
							"NodeID": fromNode.ID,
							"Error":  err,
						}).Error("Failed to create identity for signing")
						return fmt.Errorf("Failed to create identity for signing: %w", err)
					}
					err = arrayNode.Sign(identity)
					if err != nil {
						log.WithFields(log.Fields{
							"NodeID": fromNode.ID,
							"Error":  err,
						}).Error("Failed to sign promoted array node")
						return fmt.Errorf("Failed to sign promoted array node: %w", err)
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
						var leftSiblingID NodeID
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

				var siblingID NodeID
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

	c.normalize()
	return nil
}

func (c *TreeCRDT) cloneNodeFromRemote(c2 *TreeCRDT, id NodeID) {
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
	cloned.Clock = copyClock(remote.Clock)
	cloned.Owner = remote.Owner
	cloned.IsDeleted = remote.IsDeleted
	cloned.IsRoot = remote.IsRoot
	cloned.ParentID = remote.ParentID
	cloned.Nounce = remote.Nounce
	cloned.Signature = remote.Signature
	c.Nodes[id] = cloned
}

func (c *TreeCRDT) edgeExists(node *NodeCRDT, to NodeID) bool {
	for _, e := range node.Edges {
		if e.To == to {
			return true
		}
	}
	return false
}

func cloneNodeWithoutEdges(n *NodeCRDT, crdt *TreeCRDT) *NodeCRDT {
	nodeType := Literal
	if n.IsArray {
		nodeType = Array
	} else if n.IsMap {
		nodeType = Map
	}
	cloned := newNodeFromID(n.ID, nodeType, crdt)
	cloned.IsLiteral = n.IsLiteral
	cloned.LiteralValue = n.LiteralValue
	cloned.Clock = copyClock(n.Clock)
	cloned.Owner = n.Owner
	return cloned
}

func (c *TreeCRDT) normalize() {
	log.Debug("Normalizing CRDT tree")
	sortEdgesByLSEQ(c.Root.Edges)
	for _, node := range c.Nodes {
		sortEdgesByLSEQ(node.Edges)
	}
}

func (c *TreeCRDT) validAttachment(from, to NodeID) error {
	if from == to {
		return fmt.Errorf("cannot attach node %s to itself", from)
	}

	// 1. Check for cycle
	visited := make(map[NodeID]bool)
	var dfs func(NodeID) bool
	dfs = func(id NodeID) bool {
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

func (c *TreeCRDT) ValidateTree() error {
	if c.Root == nil {
		return fmt.Errorf("Tree must have a root node")
	}

	parentMap := make(map[NodeID]NodeID)
	visited := make(map[NodeID]bool)

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
			return fmt.Errorf("Node %s must have exactly one type: Map, Array, or Literal", node.ID)
		}
		return nil
	}

	var dfs func(current NodeID, ancestors map[NodeID]bool) error
	dfs = func(current NodeID, ancestors map[NodeID]bool) error {
		if ancestors[current] {
			log.WithField("NodeID", current).Debug("Cycle detected")
			return fmt.Errorf("Cycle detected at node %s", current)
		}
		if visited[current] {
			return nil
		}
		visited[current] = true

		node, exists := c.Nodes[current]
		if !exists {
			log.WithField("NodeID", current).Debug("Node not found")
			return fmt.Errorf("Node %s not found in tree", current)
		}

		// Validate type (non-root nodes only)
		if err := validateNodeType(node); err != nil {
			return err
		}

		// Literals must not have children
		if node.IsLiteral && len(node.Edges) > 0 {
			log.WithField("NodeID", current).Debug("Literal node has children")
			return fmt.Errorf("Literal node %s must not have children", current)
		}

		ancestors[current] = true
		for _, edge := range node.Edges {
			childID := edge.To

			childNode, ok := c.Nodes[childID]
			if !ok {
				log.WithField("ChildID", childID).Debug("Edge to non-existent node")
				return fmt.Errorf("Edge to non-existent node: %s", childID)
			}

			// Root must not have a parent
			if childNode.IsRoot {
				log.WithField("ParentNodeID", current).Debug("Root node has a parent")
				return fmt.Errorf("Root node must not have a parent")
			}

			if existingParent, ok := parentMap[childID]; ok && existingParent != current {
				log.WithFields(log.Fields{
					"ChildID":        childID,
					"ExistingParent": existingParent,
					"CurrentParent":  current,
				}).Debug("Multiple parents detected")
				return fmt.Errorf("Node %s has multiple parents: %s and %s", childID, existingParent, current)
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
	if err := dfs(c.Root.ID, make(map[NodeID]bool)); err != nil {
		return err
	}

	// Ensure all nodes were visited (i.e. reachable from root)
	for id := range c.Nodes {
		if !visited[id] {
			log.WithField("NodeID", id).Debug("Unreachable node detected")
			return fmt.Errorf("Unreachable node found: %s", id)
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
		if !c.ABACPolicy.IsAllowed(recoveredID, ActionModify, id) {
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

func (t *TreeCRDT) isDescendant(root NodeID, target NodeID) bool {
	if root == target {
		return true
	}
	visited := make(map[NodeID]bool)
	var dfs func(NodeID) bool
	dfs = func(n NodeID) bool {
		if visited[n] {
			return false
		}
		visited[n] = true
		node, ok := t.Nodes[n]
		if !ok {
			return false
		}
		for _, edge := range node.Edges {
			if edge.To == target || dfs(edge.To) {
				return true
			}
		}
		return false
	}
	return dfs(root)
}
