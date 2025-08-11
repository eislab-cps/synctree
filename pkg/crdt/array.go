package crdt

import (
	"fmt"
	"math/rand"
	"sort"
	"strings"

	"github.com/eislab-cps/synctree/pkg/core"
	"github.com/eislab-cps/synctree/pkg/vectorclock"
)

// ====================
// Array B-Tree Operations
// ====================

// AddArrayElement adds an element to an array B-tree at the specified index
// The array root must have IsArrayRoot = true
func (c *TreeCRDT) AddArrayElement(arrayRootID, elementID core.NodeID, index int, clientID core.ClientID) error {
	arrayRoot := c.Nodes[arrayRootID]
	if arrayRoot == nil {
		return fmt.Errorf("array root node %s not found", arrayRootID)
	}
	if !arrayRoot.IsArrayRoot {
		return fmt.Errorf("node %s is not an array root", arrayRootID)
	}
	
	element := c.Nodes[elementID]
	if element == nil {
		return fmt.Errorf("element node %s not found", elementID)
	}
	
	// Set up array element metadata
	element.IsArrayElement = true
	element.ArrayIndex = index
	element.ParentID = arrayRootID
	element.BTreeKey = c.generateBTreeKey(arrayRootID, index)
	
	// Update vector clock
	element.Clock[clientID] = element.Clock[clientID] + 1
	
	return nil
}

// MoveArrayElement atomically moves an element within an array to a new position
// Uses LWW conflict resolution for concurrent moves of the same element
func (c *TreeCRDT) MoveArrayElement(elementID, arrayRootID core.NodeID, newIndex int, clientID core.ClientID) error {
	element := c.Nodes[elementID]
	if element == nil {
		return fmt.Errorf("element node %s not found", elementID)
	}
	if !element.IsArrayElement {
		return fmt.Errorf("node %s is not an array element", elementID)
	}
	
	arrayRoot := c.Nodes[arrayRootID]
	if arrayRoot == nil {
		return fmt.Errorf("array root node %s not found", arrayRootID)
	}
	if !arrayRoot.IsArrayRoot {
		return fmt.Errorf("node %s is not an array root", arrayRootID)
	}
	
	// Create move operation with current vector clock for LWW resolution
	newClock := vectorclock.CopyClock(element.Clock)
	newClock[clientID] = newClock[clientID] + 1
	
	// Check for concurrent move conflicts using LWW - use ResolveConflict
	winningClock, winningOwner := vectorclock.ResolveConflict(element.Clock, newClock, element.Owner, clientID, false)
	
	if vectorclock.ClocksEqual(winningClock, newClock) && winningOwner == clientID {
		// Update element metadata atomically
		element.ArrayIndex = newIndex
		element.ParentID = arrayRootID
		element.BTreeKey = c.generateBTreeKey(arrayRootID, newIndex)
		element.Clock = winningClock
		// Note: Rebalancing will be handled during merge if needed
	}
	
	return nil // Move was rejected due to LWW conflict resolution
}

// GetArrayElements returns all elements in an array ordered by their ArrayIndex
func (c *TreeCRDT) GetArrayElements(arrayRootID core.NodeID) []*NodeCRDT {
	arrayRoot := c.Nodes[arrayRootID]
	if arrayRoot == nil || !arrayRoot.IsArrayRoot {
		return nil
	}
	
	var elements []*NodeCRDT
	for _, node := range c.Nodes {
		if node.IsArrayElement && node.ParentID == arrayRootID {
			elements = append(elements, node)
		}
	}
	
	// Sort by ArrayIndex for consistent ordering
	sort.Slice(elements, func(i, j int) bool {
		return elements[i].ArrayIndex < elements[j].ArrayIndex
	})
	
	return elements
}

// generateBTreeKey generates a B-tree key using LSEQ algorithm
func (c *TreeCRDT) generateBTreeKey(arrayRootID core.NodeID, index int) string {
	// Simple LSEQ implementation for B-tree ordering
	// In a full implementation, this would use fractional indexing
	
	// Use a simple scheme: lseq_<index>_<random>
	random := rand.Intn(10000)
	return fmt.Sprintf("lseq_%d_%d", index, random)
}

// rebalanceArrayBTree rebalances the B-tree structure after element moves
// Uses deterministic conflict resolution to ensure all replicas converge to the same state
func (c *TreeCRDT) rebalanceArrayBTree(arrayRootID core.NodeID) error {
	elements := c.GetArrayElements(arrayRootID)
	if len(elements) == 0 {
		return nil
	}
	
	// Step 1: Sort all elements by their final precedence (deterministic global ordering)
	sort.Slice(elements, func(i, j int) bool {
		return c.compareElementPrecedence(elements[i], elements[j]) < 0
	})
	
	// Step 2: Assign positions deterministically based on precedence
	// This ensures all replicas assign the same final positions
	for newIndex, elem := range elements {
		elem.ArrayIndex = newIndex
		elem.BTreeKey = c.generateBTreeKey(arrayRootID, newIndex)
	}
	
	return nil
}

// compareElementPrecedence provides deterministic global ordering for array elements
// This ensures all replicas apply the same final ordering after merge
func (c *TreeCRDT) compareElementPrecedence(a, b *NodeCRDT) int {
	// 1. Compare by vector clock (LWW semantics)
	clockCmp := c.compareVectorClocks(a.Clock, b.Clock)
	if clockCmp != 0 {
		return clockCmp
	}
	
	// 2. If clocks are equal, compare by owner (deterministic tie-breaking)
	if a.Owner < b.Owner {
		return -1
	} else if a.Owner > b.Owner {
		return 1
	}
	
	// 3. If owners are equal, compare by NodeID (ultimate deterministic tie-breaker)
	if a.ID < b.ID {
		return -1
	} else if a.ID > b.ID {
		return 1
	}
	
	return 0
}

// findFreeArrayPosition finds a free position near the desired position
func (c *TreeCRDT) findFreeArrayPosition(elements []*NodeCRDT, preferredPos int) int {
	occupiedPositions := make(map[int]bool)
	for _, elem := range elements {
		occupiedPositions[elem.ArrayIndex] = true
	}
	
	// Try positions near the preferred position
	for offset := 0; offset < len(elements)+10; offset++ {
		// Try positive offset first
		if offset > 0 {
			pos := preferredPos + offset
			if !occupiedPositions[pos] {
				return pos
			}
		}
		
		// Try negative offset
		if offset > 0 {
			pos := preferredPos - offset
			if pos >= 0 && !occupiedPositions[pos] {
				return pos
			}
		}
		
		// Try the exact position if offset is 0
		if offset == 0 && !occupiedPositions[preferredPos] {
			return preferredPos
		}
	}
	
	// Fallback: find first available position from 0
	for i := 0; ; i++ {
		if !occupiedPositions[i] {
			return i
		}
	}
}

// compareVectorClocks compares two vector clocks for deterministic ordering
func (c *TreeCRDT) compareVectorClocks(clock1, clock2 map[core.ClientID]int) int {
	// Convert clocks to sorted string representation for comparison
	str1 := c.vectorClockToString(clock1)
	str2 := c.vectorClockToString(clock2)
	
	if str1 < str2 {
		return -1
	} else if str1 > str2 {
		return 1
	}
	return 0
}

// vectorClockToString converts a vector clock to a deterministic string
func (c *TreeCRDT) vectorClockToString(clock map[core.ClientID]int) string {
	var parts []string
	
	// Sort client IDs for deterministic ordering
	var clientIDs []string
	for clientID := range clock {
		clientIDs = append(clientIDs, string(clientID))
	}
	sort.Strings(clientIDs)
	
	// Build string representation
	for _, clientIDStr := range clientIDs {
		clientID := core.ClientID(clientIDStr)
		if count := clock[clientID]; count > 0 {
			parts = append(parts, fmt.Sprintf("%s:%d", clientIDStr, count))
		}
	}
	
	return strings.Join(parts, ",")
}

// mergeArrayElementMetadata merges array-specific metadata between local and remote nodes
// Uses LWW resolution based on vector clocks for array position conflicts
func (c *TreeCRDT) mergeArrayElementMetadata(local, remote *NodeCRDT) {
	// Copy array root flag
	if remote.IsArrayRoot {
		local.IsArrayRoot = remote.IsArrayRoot
	}
	
	// Handle array element metadata with LWW resolution
	if remote.IsArrayElement {
		local.IsArrayElement = remote.IsArrayElement
		local.ParentID = remote.ParentID
		
		// Use LWW to resolve array position conflicts
		winningClock, winningOwner := vectorclock.ResolveConflict(
			local.Clock, remote.Clock, 
			local.Owner, remote.Owner, 
			false)
		
		// If remote wins, update array position
		if vectorclock.ClocksEqual(winningClock, remote.Clock) && winningOwner == remote.Owner {
			local.ArrayIndex = remote.ArrayIndex
			local.BTreeKey = remote.BTreeKey
		}
		// If local wins, keep current position - no change needed
	}
}

// mergeArrayElements ensures all array elements in a tree are properly synchronized
// This should be called after all nodes have been merged to resolve any remaining conflicts
func (c *TreeCRDT) mergeArrayElements() {
	// Find all array roots
	arrayRoots := make(map[core.NodeID]*NodeCRDT)
	for _, node := range c.Nodes {
		if node.IsArrayRoot {
			arrayRoots[node.ID] = node
		}
	}
	
	// Only rebalance arrays that have position conflicts
	for arrayRootID := range arrayRoots {
		if c.hasArrayPositionConflicts(arrayRootID) {
			c.rebalanceArrayBTree(arrayRootID)
		}
	}
}

// hasArrayPositionConflicts checks if an array has elements with conflicting positions
func (c *TreeCRDT) hasArrayPositionConflicts(arrayRootID core.NodeID) bool {
	elements := c.GetArrayElements(arrayRootID)
	positionCount := make(map[int]int)
	
	for _, elem := range elements {
		positionCount[elem.ArrayIndex]++
	}
	
	// Check for positions with more than one element
	for _, count := range positionCount {
		if count > 1 {
			return true
		}
	}
	
	return false
}