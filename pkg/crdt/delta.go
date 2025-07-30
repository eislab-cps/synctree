package crdt

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/eislab-cps/synctree/internal/crypto"
	"github.com/eislab-cps/synctree/pkg/core"
	"github.com/eislab-cps/synctree/pkg/vectorclock"
	log "github.com/sirupsen/logrus"
)

// clockHappensBefore checks if clock a happens before clock b
func clockHappensBefore(a, b vectorclock.VectorClock) bool {
	// Use the proper vector clock comparison API
	return vectorclock.CompareClock(a, b) == vectorclock.ClockIsDominated
}

// DeltaMutation represents a single mutation with its causal context
type DeltaMutation struct {
	NodeID       core.NodeID             `json:"nodeid"`
	Op           Operation               `json:"op"`
	Key          string                  `json:"key,omitempty"`
	Value        interface{}             `json:"value,omitempty"`
	ClientID     core.ClientID           `json:"clientid"`
	Version      int                     `json:"version"`
	Clock        vectorclock.VectorClock `json:"clock"`
	Signature    string                  `json:"signature,omitempty"`
	FromNodeID   core.NodeID             `json:"from,omitempty"`     // For edge operations
	ToNodeID     core.NodeID             `json:"to,omitempty"`       // For edge operations
	Label        string                  `json:"label,omitempty"`    // For edge operations
	LSEQPosition []int                   `json:"lseq,omitempty"`     // For array operations
	NodeType     core.NodeType           `json:"nodetype,omitempty"` // For create operations
}

// DeltaCRDT represents a set of mutations that form a delta
type DeltaCRDT struct {
	Mutations []DeltaMutation         `json:"mutations"`
	Clock     vectorclock.VectorClock `json:"clock"` // Resulting clock after applying all mutations
}

// MutationLog tracks all mutations for delta generation
type MutationLog struct {
	mutations []DeltaMutation
	// Index for efficient lookup by client
	clientIndex map[core.ClientID][]int
}

// NewMutationLog creates a new mutation log
func NewMutationLog() *MutationLog {
	return &MutationLog{
		mutations:   make([]DeltaMutation, 0),
		clientIndex: make(map[core.ClientID][]int),
	}
}

// AddMutation adds a mutation to the log
func (ml *MutationLog) AddMutation(mut DeltaMutation) {
	index := len(ml.mutations)
	ml.mutations = append(ml.mutations, mut)

	// Update client index
	if _, exists := ml.clientIndex[mut.ClientID]; !exists {
		ml.clientIndex[mut.ClientID] = make([]int, 0)
	}
	ml.clientIndex[mut.ClientID] = append(ml.clientIndex[mut.ClientID], index)
}

// GetMutationsSince returns all mutations that happened after the given vector clock
func (ml *MutationLog) GetMutationsSince(since vectorclock.VectorClock) []DeltaMutation {
	result := make([]DeltaMutation, 0)

	for _, mut := range ml.mutations {
		// Use proper vector clock comparison
		switch vectorclock.CompareClock(since, mut.Clock) {
		case vectorclock.ClockIsDominated:
			// since < mut.Clock - include this mutation
			result = append(result, mut)
		case vectorclock.ClockConcurrent:
			// Concurrent mutations should be included for safety
			// This ensures no updates are lost in distributed scenarios
			result = append(result, mut)
		case vectorclock.ClockDominates, vectorclock.ClockEqual:
			// since >= mut.Clock - skip this mutation
			continue
		}
	}

	return result
}

// GenerateDelta creates a delta containing all mutations since the given clock
func (c *TreeCRDT) GenerateDelta(since vectorclock.VectorClock) *DeltaCRDT {
	if c.mutationLog == nil {
		return &DeltaCRDT{
			Mutations: []DeltaMutation{},
			Clock:     c.GetVectorClock(),
		}
	}

	mutations := c.mutationLog.GetMutationsSince(since)

	return &DeltaCRDT{
		Mutations: mutations,
		Clock:     c.GetVectorClock(),
	}
}

// GetVectorClock computes the current vector clock of the tree
func (c *TreeCRDT) GetVectorClock() vectorclock.VectorClock {
	clock := make(vectorclock.VectorClock)

	// Aggregate clocks from all nodes
	for _, node := range c.Nodes {
		clock = vectorclock.MergeClocks(clock, node.Clock)
	}

	return clock
}

// MergeDelta applies a delta to the current CRDT state
func (c *TreeCRDT) MergeDelta(delta *DeltaCRDT) error {
	return c.mergeDelta(delta, false, "")
}

// MergeDeltaLenient merges a delta with lenient dependency validation for backward compatibility
func (c *TreeCRDT) MergeDeltaLenient(delta *DeltaCRDT) error {
	return c.mergeDeltaWithValidation(delta, false, "", false)
}

// SecureMergeDelta applies a delta with signature verification
func (c *TreeCRDT) SecureMergeDelta(delta *DeltaCRDT, prvKey string) error {
	return c.mergeDelta(delta, true, prvKey)
}

// sortMutationsByCausalOrder performs a topological sort on mutations based on vector clock dependencies
func sortMutationsByCausalOrder(mutations []DeltaMutation) []DeltaMutation {
	if len(mutations) <= 1 {
		return mutations
	}

	// Build dependency graph
	// dependencies[i] contains indices of mutations that must be applied before mutation i
	dependencies := make(map[int][]int)
	for i := range mutations {
		dependencies[i] = []int{}
	}

	// Find dependencies based on vector clocks
	for i, mut1 := range mutations {
		for j, mut2 := range mutations {
			if i != j && clockHappensBefore(mut2.Clock, mut1.Clock) {
				// mut2 happens before mut1, so mut2 is a dependency of mut1
				dependencies[i] = append(dependencies[i], j)
			}
		}
	}

	// Topological sort using DFS
	result := make([]DeltaMutation, 0, len(mutations))
	visited := make(map[int]bool)
	visiting := make(map[int]bool) // for cycle detection

	var visit func(int) error
	visit = func(idx int) error {
		if visited[idx] {
			return nil
		}

		if visiting[idx] {
			// Cycle detected - this shouldn't happen with valid vector clocks
			return fmt.Errorf("cycle detected in causal dependencies at mutation %d", idx)
		}

		visiting[idx] = true

		// Visit all dependencies first
		for _, dep := range dependencies[idx] {
			if err := visit(dep); err != nil {
				return err
			}
		}

		visiting[idx] = false
		visited[idx] = true
		result = append(result, mutations[idx])

		return nil
	}

	// Process all mutations
	for i := range mutations {
		if err := visit(i); err != nil {
			// If cycle detected (shouldn't happen), fall back to original order
			return mutations
		}
	}

	return result
}

// computeMutationSignature creates a unique signature for a mutation to detect duplicates
func computeMutationSignature(mut DeltaMutation) string {
	h := sha256.New()

	// Include all identifying fields
	h.Write([]byte(mut.NodeID))
	h.Write([]byte(fmt.Sprintf("%d", mut.Op)))
	h.Write([]byte(mut.ClientID))
	h.Write([]byte(fmt.Sprintf("%d", mut.Version)))

	// Include operation-specific fields
	switch mut.Op {
	case OPCreateNode:
		h.Write([]byte(fmt.Sprintf("%d", mut.NodeType)))
	case OPAddEdge, OPRemoveEdge:
		h.Write([]byte(mut.FromNodeID))
		h.Write([]byte(mut.ToNodeID))
		h.Write([]byte(mut.Label))
	case OPSetLiteral:
		h.Write([]byte(fmt.Sprintf("%v", mut.Value)))
	}

	// Include vector clock
	for clientID, version := range mut.Clock {
		h.Write([]byte(clientID))
		h.Write([]byte(fmt.Sprintf("%d", version)))
	}

	return hex.EncodeToString(h.Sum(nil))
}

// mergeDelta applies delta mutations in causal order
func (c *TreeCRDT) mergeDelta(delta *DeltaCRDT, secure bool, prvKey string) error {
	return c.mergeDeltaWithValidation(delta, secure, prvKey, true)
}

// mergeDeltaWithValidation allows control over dependency validation strictness
func (c *TreeCRDT) mergeDeltaWithValidation(delta *DeltaCRDT, secure bool, prvKey string, strictDependencies bool) error {
	// Sort mutations by causal order FIRST
	sortedMutations := sortMutationsByCausalOrder(delta.Mutations)

	// Apply mutations in causal order
	for _, mut := range sortedMutations {
		if err := c.applyDeltaMutationWithValidation(mut, secure, strictDependencies); err != nil {
			return fmt.Errorf("failed to apply delta mutation: %w", err)
		}
	}

	return nil
}

// applyDeltaMutation applies a single delta mutation to the tree
func (c *TreeCRDT) applyDeltaMutation(mut DeltaMutation, secure bool) error {
	return c.applyDeltaMutationWithValidation(mut, secure, true)
}

// applyDeltaMutationWithValidation applies a single delta mutation with configurable validation
func (c *TreeCRDT) applyDeltaMutationWithValidation(mut DeltaMutation, secure bool, strictDependencies bool) error {
	// Security validation if secure mode is enabled
	if secure {
		if mut.Signature == "" {
			log.WithFields(log.Fields{
				"ClientID": mut.ClientID,
				"NodeID":   mut.NodeID,
				"Op":       mut.Op,
			}).Error("Mutation has no signature in secure mode")
			return fmt.Errorf("mutation has no signature in secure mode")
		}

		// Verify signature
		recoveredID, err := VerifyDeltaMutation(mut)
		if err != nil {
			log.WithFields(log.Fields{
				"ClientID": mut.ClientID,
				"NodeID":   mut.NodeID,
				"Op":       mut.Op,
				"Error":    err,
			}).Error("Mutation signature verification failed")
			return fmt.Errorf("mutation signature verification failed: %w", err)
		}

		// Verify that the recovered ID matches the claimed ClientID
		expectedID := string(mut.ClientID)
		if recoveredID != expectedID {
			log.WithFields(log.Fields{
				"ClientID":    mut.ClientID,
				"RecoveredID": recoveredID,
				"ExpectedID":  expectedID,
				"NodeID":      mut.NodeID,
				"Op":          mut.Op,
			}).Error("Mutation signature does not match claimed client ID")
			return fmt.Errorf("signature verification failed: recovered ID %s does not match claimed client ID %s", recoveredID, expectedID)
		}

		log.WithFields(log.Fields{
			"ClientID":    mut.ClientID,
			"RecoveredID": recoveredID,
			"NodeID":      mut.NodeID,
			"Op":          mut.Op,
		}).Debug("Mutation signature verified successfully")
	}

	// Check for duplicates
	mutSig := computeMutationSignature(mut)
	if c.appliedMutations == nil {
		c.appliedMutations = make(map[string]bool)
	}

	if c.appliedMutations[mutSig] {
		// Already applied, skip silently (idempotent)
		return nil
	}

	// Store original mutation before modifications for logging
	originalMut := mut

	// Validate causal dependencies if strict validation is enabled
	if strictDependencies {
		if err := c.validateCausalDependencies(mut); err != nil {
			log.WithFields(log.Fields{
				"ClientID": mut.ClientID,
				"NodeID":   mut.NodeID,
				"Op":       mut.Op,
				"Version":  mut.Version,
				"Error":    err,
			}).Error("Causal dependency validation failed")
			return fmt.Errorf("causal dependency validation failed: %w", err)
		}
	}

	switch mut.Op {
	case OPCreateNode:
		// Check if node already exists
		if _, exists := c.Nodes[mut.NodeID]; !exists {
			node := newNodeFromID(mut.NodeID, mut.NodeType, c)
			c.Nodes[mut.NodeID] = node
			node.Clock = vectorclock.CopyClock(mut.Clock)
			node.Owner = mut.ClientID

			// Add to mutation log (use original mutation)
			if c.mutationLog != nil {
				c.mutationLog.AddMutation(originalMut)
			}
		}

	case OPAddEdge:
		// Validate FROM node exists
		fromNode, ok := c.Nodes[mut.FromNodeID]
		if !ok {
			return fmt.Errorf("cannot add edge, from node not found: %s", mut.FromNodeID)
		}

		// Validate TO node exists
		_, ok = c.Nodes[mut.ToNodeID]
		if !ok {
			return fmt.Errorf("cannot add edge, to node not found: %s", mut.ToNodeID)
		}

		// Apply the mutation if it's newer or concurrent with current state
		cmp := vectorclock.CompareClock(fromNode.Clock, mut.Clock)
		if cmp == vectorclock.ClockIsDominated || cmp == vectorclock.ClockConcurrent {
			// Apply the edge
			edge := &EdgeCRDT{
				From:         mut.FromNodeID,
				To:           mut.ToNodeID,
				Label:        mut.Label,
				LSEQPosition: mut.LSEQPosition,
			}

			// Check if edge already exists
			edgeExists := false
			for _, e := range fromNode.Edges {
				if e.To == mut.ToNodeID && e.Label == mut.Label {
					edgeExists = true
					break
				}
			}

			if !edgeExists {
				fromNode.Edges = append(fromNode.Edges, edge)
				fromNode.Clock = vectorclock.MergeClocks(fromNode.Clock, mut.Clock)

				// Update parent reference
				if toNode, ok := c.Nodes[mut.ToNodeID]; ok {
					toNode.ParentID = mut.FromNodeID
				}

				// Add to mutation log
				if c.mutationLog != nil {
					c.mutationLog.AddMutation(mut)
				}
			}
		}

	case OPSetLiteral:
		node, ok := c.Nodes[mut.NodeID]
		if !ok {
			return fmt.Errorf("cannot set literal, node not found: %s", mut.NodeID)
		}

		// Apply mutation using existing conflict resolution
		winningClock, winningOwner := vectorclock.ResolveConflict(node.Clock, mut.Clock, node.Owner, mut.ClientID, false)

		if vectorclock.ClocksEqual(winningClock, mut.Clock) && winningOwner == mut.ClientID {
			node.IsLiteral = true
			node.LiteralValue = mut.Value
			node.Clock = vectorclock.CopyClock(mut.Clock)
			node.Owner = mut.ClientID

			// Add to mutation log (use original mutation)
			if c.mutationLog != nil {
				c.mutationLog.AddMutation(originalMut)
			}
		}

	case OPRemoveEdge:
		fromNode, ok := c.Nodes[mut.FromNodeID]
		if !ok {
			return fmt.Errorf("cannot remove edge, from node not found: %s", mut.FromNodeID)
		}

		// Check if we should apply this mutation based on vector clock
		winningClock, _ := vectorclock.ResolveConflict(fromNode.Clock, mut.Clock, fromNode.Owner, mut.ClientID, false)

		if vectorclock.ClocksEqual(winningClock, mut.Clock) {
			// Remove the edge
			newEdges := make([]*EdgeCRDT, 0)
			for _, edge := range fromNode.Edges {
				if edge.To != mut.ToNodeID {
					newEdges = append(newEdges, edge)
				}
			}
			fromNode.Edges = newEdges
			fromNode.Clock = vectorclock.CopyClock(mut.Clock)
			fromNode.Owner = mut.ClientID

			// Update parent reference
			if toNode, ok := c.Nodes[mut.ToNodeID]; ok {
				toNode.ParentID = ""
			}

			// Add to mutation log (use original mutation)
			if c.mutationLog != nil {
				c.mutationLog.AddMutation(originalMut)
			}
		}

	default:
		return fmt.Errorf("unknown operation: %v", mut.Op)
	}

	// Mark mutation as applied (only after successful application)
	c.appliedMutations[mutSig] = true

	return nil
}

// SignDeltaMutation signs a delta mutation using the provided identity
func SignDeltaMutation(mut *DeltaMutation, identity *crypto.Idendity) error {
	// Create a deterministic digest of the mutation
	digest, err := computeMutationDigest(*mut)
	if err != nil {
		log.WithFields(log.Fields{
			"ClientID": mut.ClientID,
			"NodeID":   mut.NodeID,
			"Op":       mut.Op,
			"Error":    err,
		}).Error("Failed to compute mutation digest")
		return fmt.Errorf("failed to compute mutation digest: %w", err)
	}

	// Sign the digest
	signature, err := crypto.Sign(digest, identity.PrivateKey())
	if err != nil {
		log.WithFields(log.Fields{
			"ClientID": mut.ClientID,
			"NodeID":   mut.NodeID,
			"Op":       mut.Op,
			"Error":    err,
		}).Error("Failed to sign mutation")
		return fmt.Errorf("failed to sign mutation: %w", err)
	}

	// Store signature as hex string
	mut.Signature = hex.EncodeToString(signature)

	return nil
}

// VerifyDeltaMutation verifies the signature of a delta mutation
func VerifyDeltaMutation(mut DeltaMutation) (string, error) {
	if mut.Signature == "" {
		return "", fmt.Errorf("mutation has no signature")
	}

	// Compute digest for verification
	digest, err := computeMutationDigest(mut)
	if err != nil {
		log.WithFields(log.Fields{
			"ClientID": mut.ClientID,
			"NodeID":   mut.NodeID,
			"Op":       mut.Op,
			"Error":    err,
		}).Error("Failed to compute mutation digest for verification")
		return "", fmt.Errorf("failed to compute mutation digest: %w", err)
	}

	// Decode signature
	signatureBytes, err := hex.DecodeString(mut.Signature)
	if err != nil {
		log.WithFields(log.Fields{
			"ClientID":  mut.ClientID,
			"NodeID":    mut.NodeID,
			"Signature": mut.Signature,
			"Error":     err,
		}).Error("Failed to decode mutation signature")
		return "", fmt.Errorf("failed to decode signature: %w", err)
	}

	// Recover public key and verify signature
	recoveredPublicKey, err := crypto.RecoverPublicKey(digest, signatureBytes)
	if err != nil {
		log.WithFields(log.Fields{
			"ClientID": mut.ClientID,
			"NodeID":   mut.NodeID,
			"Error":    err,
		}).Error("Failed to recover public key from mutation signature")
		return "", fmt.Errorf("failed to recover public key from signature: %w", err)
	}

	// Verify signature
	valid, err := crypto.Verify(recoveredPublicKey, digest, signatureBytes)
	if err != nil {
		log.WithFields(log.Fields{
			"ClientID": mut.ClientID,
			"NodeID":   mut.NodeID,
			"Error":    err,
		}).Error("Failed to verify mutation signature")
		return "", fmt.Errorf("failed to verify signature: %w", err)
	}

	if !valid {
		log.WithFields(log.Fields{
			"ClientID":  mut.ClientID,
			"NodeID":    mut.NodeID,
			"Signature": mut.Signature,
		}).Error("Mutation signature verification failed")
		return "", fmt.Errorf("signature verification failed for mutation")
	}

	// Get recovered client ID
	recoveredID, err := crypto.RecoveredID(digest, signatureBytes)
	if err != nil {
		log.WithFields(log.Fields{
			"ClientID": mut.ClientID,
			"NodeID":   mut.NodeID,
			"Error":    err,
		}).Error("Failed to recover client ID from mutation signature")
		return "", fmt.Errorf("failed to recover client ID: %w", err)
	}

	return recoveredID, nil
}

// computeMutationDigest creates a cryptographic hash of a mutation for signing
func computeMutationDigest(mut DeltaMutation) (*crypto.Hash, error) {
	// Create a deterministic representation of the mutation (excluding signature)
	digestMut := struct {
		NodeID       core.NodeID             `json:"nodeid"`
		Op           Operation               `json:"op"`
		Key          string                  `json:"key,omitempty"`
		Value        interface{}             `json:"value,omitempty"`
		ClientID     core.ClientID           `json:"clientid"`
		Version      int                     `json:"version"`
		Clock        vectorclock.VectorClock `json:"clock"`
		FromNodeID   core.NodeID             `json:"from,omitempty"`
		ToNodeID     core.NodeID             `json:"to,omitempty"`
		Label        string                  `json:"label,omitempty"`
		LSEQPosition []int                   `json:"lseq,omitempty"`
		NodeType     core.NodeType           `json:"nodetype,omitempty"`
	}{
		NodeID:       mut.NodeID,
		Op:           mut.Op,
		Key:          mut.Key,
		Value:        mut.Value,
		ClientID:     mut.ClientID,
		Version:      mut.Version,
		Clock:        mut.Clock,
		FromNodeID:   mut.FromNodeID,
		ToNodeID:     mut.ToNodeID,
		Label:        mut.Label,
		LSEQPosition: mut.LSEQPosition,
		NodeType:     mut.NodeType,
	}

	// Serialize to JSON for deterministic hashing
	jsonBytes, err := json.Marshal(digestMut)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal mutation for digest: %w", err)
	}

	// Compute hash
	return crypto.GenerateHashFromString(string(jsonBytes)), nil
}

// validateCausalDependencies ensures that all causal dependencies for a mutation are satisfied
func (c *TreeCRDT) validateCausalDependencies(mut DeltaMutation) error {
	// For each client in the mutation's vector clock, verify that we have seen
	// all previous versions from that client
	for clientID, version := range mut.Clock {
		if version <= 0 {
			continue // Skip invalid versions
		}

		// Find the highest version we've seen from this client
		highestSeen := 0
		for _, node := range c.Nodes {
			if node.Clock != nil {
				if clientVersion, exists := node.Clock[clientID]; exists && clientVersion > highestSeen {
					highestSeen = clientVersion
				}
			}
		}

		// Check mutation log for additional versions
		if c.mutationLog != nil {
			for _, loggedMut := range c.mutationLog.mutations {
				if loggedClientVersion, exists := loggedMut.Clock[clientID]; exists && loggedClientVersion > highestSeen {
					highestSeen = loggedClientVersion
				}
			}
		}

		// The mutation's version should be at most one higher than what we've seen
		if version > highestSeen+1 {
			log.WithFields(log.Fields{
				"ClientID":        clientID,
				"MutationVersion": version,
				"HighestSeen":     highestSeen,
				"Operation":       mut.Op,
				"NodeID":          mut.NodeID,
			}).Error("Missing causal dependency - gap in version sequence")
			return fmt.Errorf("missing causal dependency for client %s: mutation version %d but highest seen is %d",
				clientID, version, highestSeen)
		}
	}

	// Additional validation: for edge operations, ensure referenced nodes exist or will exist
	switch mut.Op {
	case OPAddEdge:
		// FromNode dependency
		if mut.FromNodeID != "" {
			if _, exists := c.Nodes[mut.FromNodeID]; !exists {
				// Check if a create mutation for this node exists in our mutation log with a lower version
				if !c.nodeWillExist(mut.FromNodeID, mut.Clock) {
					return fmt.Errorf("from node %s does not exist and no create mutation found", mut.FromNodeID)
				}
			}
		}

		// ToNode dependency
		if mut.ToNodeID != "" {
			if _, exists := c.Nodes[mut.ToNodeID]; !exists {
				if !c.nodeWillExist(mut.ToNodeID, mut.Clock) {
					return fmt.Errorf("to node %s does not exist and no create mutation found", mut.ToNodeID)
				}
			}
		}

	case OPRemoveEdge:
		// FromNode must exist
		if mut.FromNodeID != "" {
			if _, exists := c.Nodes[mut.FromNodeID]; !exists {
				return fmt.Errorf("cannot remove edge: from node %s does not exist", mut.FromNodeID)
			}
		}

	case OPSetLiteral:
		// Node must exist
		if _, exists := c.Nodes[mut.NodeID]; !exists {
			if !c.nodeWillExist(mut.NodeID, mut.Clock) {
				return fmt.Errorf("cannot set literal: node %s does not exist and no create mutation found", mut.NodeID)
			}
		}
	}

	return nil
}

// nodeWillExist checks if a node will exist based on pending mutations in the log
func (c *TreeCRDT) nodeWillExist(nodeID core.NodeID, beforeClock vectorclock.VectorClock) bool {
	if c.mutationLog == nil {
		return false
	}

	// Look for a create mutation for this node that happens before the given clock
	for _, loggedMut := range c.mutationLog.mutations {
		if loggedMut.Op == OPCreateNode && loggedMut.NodeID == nodeID {
			// Check if this create mutation happens before the given clock
			if clockHappensBefore(loggedMut.Clock, beforeClock) || vectorclock.ClocksEqual(loggedMut.Clock, beforeClock) {
				return true
			}
		}
	}

	return false
}
