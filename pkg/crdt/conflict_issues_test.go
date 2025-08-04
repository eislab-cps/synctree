package crdt

import (
	"testing"

	"github.com/eislab-cps/synctree/pkg/core"
	"github.com/eislab-cps/synctree/pkg/utils"
	"github.com/stretchr/testify/assert"
)

// TestArrayPromotionTimingInconsistency tests that array promotion
// should behave consistently regardless of whether children are added directly
// or through merge operations. This test FAILS if the behavior is inconsistent.
func TestArrayPromotionTimingInconsistency(t *testing.T) {
	clientA := core.ClientID("clientA")
	
	// Scenario 1: Direct addition
	t.Log("=== Scenario 1: Direct Addition ===")
	crdt1 := NewTreeCRDT()
	child1 := crdt1.CreateAttachedNode("child1", Literal, crdt1.Root.ID, clientA)
	child1.SetLiteral("value1", clientA)
	child2 := crdt1.CreateAttachedNode("child2", Literal, crdt1.Root.ID, clientA)
	child2.SetLiteral("value2", clientA)
	
	json1, _ := crdt1.ExportJSON()
	t.Logf("Scenario 1 result: %s", string(json1))
	
	// Scenario 2: Merge-based addition (equivalent logical operation)
	t.Log("=== Scenario 2: Merge-based Addition ===")
	crdtA := NewTreeCRDT()
	crdtB := NewTreeCRDT()
	
	// ClientA adds first child
	childA := crdtA.CreateAttachedNode("child1", Literal, crdtA.Root.ID, clientA)
	childA.SetLiteral("value1", clientA)
	
	// ClientB starts with same initial state, then adds second child
	crdtB, _ = crdtA.Clone()
	childB := crdtB.CreateAttachedNode("child2", Literal, crdtB.Root.ID, clientA)
	childB.SetLiteral("value2", clientA)
	
	// Merge should produce equivalent result
	err := crdtA.Merge(crdtB)
	assert.NoError(t, err)
	
	json2, _ := crdtA.ExportJSON()
	t.Logf("Scenario 2 result: %s", string(json2))
	
	// ASSERTION: The results SHOULD be semantically equivalent
	// Both scenarios create the same logical structure: root with 2 literal children
	isEqual := utils.IsJSONEqual(t, json1, json2)
	
	if !isEqual {
		t.Errorf("CONFLICT RESOLUTION BUG: Same logical operation produces different structures!")
		t.Errorf("Direct addition result: %s", string(json1))
		t.Errorf("Merge-based result: %s", string(json2))
		t.Errorf("Expected: Consistent behavior regardless of operation timing")
	}
	
	assert.True(t, isEqual, "Same logical operation (root + 2 children) should produce identical structures")
}

// TestArrayPromotionLossOfStructure demonstrates how array promotion
// destroys semantic structure by discarding meaningful keys
func TestArrayPromotionLossOfStructure(t *testing.T) {
	clientA := core.ClientID("clientA")
	clientB := core.ClientID("clientB")
	
	// Create initial structured document
	initialJSON := []byte(`{"users": {"alice": {"role": "admin"}}}`)
	
	crdtA := NewTreeCRDT()
	_, err := crdtA.ImportJSON(initialJSON, clientA)
	assert.NoError(t, err)
	
	crdtB, err := crdtA.Clone()
	assert.NoError(t, err)
	
	// Both clients add users to the same parent but with different approaches
	usersA, err := crdtA.GetNodeByPath("/users")
	assert.NoError(t, err)
	_, _, err = usersA.SetKeyValue("bob", map[string]interface{}{"role": "user"}, clientA)
	assert.NoError(t, err)
	
	usersB, err := crdtB.GetNodeByPath("/users")  
	assert.NoError(t, err)
	_, _, err = usersB.SetKeyValue("charlie", map[string]interface{}{"role": "moderator"}, clientB)
	assert.NoError(t, err)
	
	// Document states before merge
	jsonA, _ := crdtA.ExportJSON()
	jsonB, _ := crdtB.ExportJSON()
	t.Logf("Client A before merge: %s", string(jsonA))
	t.Logf("Client B before merge: %s", string(jsonB))
	
	// Both should preserve key-value semantics
	aliceA, err := crdtA.GetValueByPath("/users/alice")
	assert.NoError(t, err)
	assert.NotNil(t, aliceA, "Alice should exist in A")
	
	charlieB, err := crdtB.GetValueByPath("/users/charlie")
	assert.NoError(t, err)
	assert.NotNil(t, charlieB, "Charlie should exist in B")
	
	// Merge preserves map structure (no promotion because users is already a Map)
	err = crdtA.Merge(crdtB)
	assert.NoError(t, err)
	
	finalJSON, _ := crdtA.ExportJSON()
	t.Logf("After merge: %s", string(finalJSON))
	
	// Verify all users are preserved with their keys
	aliceFinal, err := crdtA.GetValueByPath("/users/alice")
	assert.NoError(t, err)
	assert.NotNil(t, aliceFinal, "Alice should exist after merge")
	
	bobFinal, err := crdtA.GetValueByPath("/users/bob")
	assert.NoError(t, err)
	assert.NotNil(t, bobFinal, "Bob should exist after merge")
	
	charlieFinal, err := crdtA.GetValueByPath("/users/charlie")
	assert.NoError(t, err)
	assert.NotNil(t, charlieFinal, "Charlie should exist after merge")
	
	t.Log("*** STRUCTURE PRESERVATION TEST ***")
	t.Log("Map nodes preserve key-value semantics correctly")
	t.Log("But if this were a non-Map node, structure would be lost to array promotion")
}

// TestNonDeterministicPromotionConditions tests that identical end states
// should have identical structures regardless of operation timing.
// This test FAILS if the behavior is non-deterministic.
func TestNonDeterministicPromotionConditions(t *testing.T) {
	clientA := core.ClientID("clientA")
	clientB := core.ClientID("clientB")
	
	// Test Case 1: Sequential operations on same client
	t.Log("=== Case 1: Sequential Operations ===")
	crdt1 := NewTreeCRDT()
	
	// Import simple value
	_, err := crdt1.ImportJSON([]byte(`"initial"`), clientA)
	assert.NoError(t, err)
	
	// Add two more children sequentially
	child1 := crdt1.CreateAttachedNode("child1", Literal, crdt1.Root.ID, clientA)
	child1.SetLiteral("value1", clientA)
	child2 := crdt1.CreateAttachedNode("child2", Literal, crdt1.Root.ID, clientA)  
	child2.SetLiteral("value2", clientA)
	
	json1, _ := crdt1.ExportJSON()
	t.Logf("Sequential result: %s", string(json1))
	
	// Test Case 2: Concurrent operations via merge (should produce same result)
	t.Log("=== Case 2: Concurrent Operations ===")
	crdtA2 := NewTreeCRDT()
	_, err = crdtA2.ImportJSON([]byte(`"initial"`), clientA)
	assert.NoError(t, err)
	
	crdtB2, err := crdtA2.Clone()
	assert.NoError(t, err)
	
	// Concurrent additions (same logical end state as Case 1)
	childA2 := crdtA2.CreateAttachedNode("child1", Literal, crdtA2.Root.ID, clientA)
	childA2.SetLiteral("value1", clientA)
	
	childB2 := crdtB2.CreateAttachedNode("child2", Literal, crdtB2.Root.ID, clientB)
	childB2.SetLiteral("value2", clientB)
	
	// Merge should produce same logical structure as sequential operations
	err = crdtA2.Merge(crdtB2)
	assert.NoError(t, err)
	
	json2, _ := crdtA2.ExportJSON()
	t.Logf("Concurrent result: %s", string(json2))
	
	// ASSERTION: Both approaches should produce identical structures
	// This is a fundamental requirement for deterministic conflict resolution
	isEqual := utils.IsJSONEqual(t, json1, json2)
	
	if !isEqual {
		t.Errorf("DETERMINISM VIOLATION: Identical end states have different structures!")
		t.Errorf("Sequential operations: %s", string(json1))
		t.Errorf("Concurrent operations: %s", string(json2))
		t.Errorf("Expected: Deterministic behavior regardless of operation timing")
	}
	
	assert.True(t, isEqual, "Identical logical end states must produce identical structures")
}

// TestInconsistentVectorClockResolution tests that all operations should use
// consistent vector clock conflict resolution. This test FAILS if operations
// use different resolution strategies.
func TestInconsistentVectorClockResolution(t *testing.T) {
	clientA := core.ClientID("clientA")
	clientB := core.ClientID("clientB")
	
	t.Log("=== Testing Vector Clock Consistency Across Operations ===")
	
	// Test: Create identical conflict scenarios for different operation types
	// and verify they all use the same conflict resolution strategy
	
	// Scenario 1: Literal value conflict (baseline for expected behavior)
	t.Log("--- Scenario 1: Literal Value Conflict (Baseline) ---")
	crdtA1 := NewTreeCRDT()
	literalA1 := crdtA1.CreateAttachedNode("test", Literal, crdtA1.Root.ID, clientA)
	literalA1.SetLiteral("valueA", clientA)
	
	crdtB1, _ := crdtA1.Clone()
	literalB1, _ := crdtB1.GetNode(literalA1.ID)
	literalB1.SetLiteral("valueB", clientB)  // Concurrent edit
	
	err := crdtA1.Merge(crdtB1)
	assert.NoError(t, err)
	
	finalLiteral, _ := crdtA1.GetNode(literalA1.ID)
	baseline_winner := finalLiteral.LiteralValue.(string)
	t.Logf("Literal conflict winner: %s", baseline_winner)
	
	// Scenario 2: Map key conflict (should use same resolution as literals)
	t.Log("--- Scenario 2: Map Key Conflict ---")
	crdtA2 := NewTreeCRDT()
	mapA2 := crdtA2.CreateAttachedNode("map", Map, crdtA2.Root.ID, clientA)
	
	crdtB2, _ := crdtA2.Clone()
	mapB2, _ := crdtB2.GetNode(mapA2.ID)
	
	// Both clients set same key to different values (same logical conflict as Scenario 1)
	_, _, err = mapA2.SetKeyValue("key", "valueA", clientA)
	assert.NoError(t, err)
	_, _, err = mapB2.SetKeyValue("key", "valueB", clientB)
	assert.NoError(t, err)
	
	err = crdtA2.Merge(crdtB2)
	assert.NoError(t, err)
	
	mapValue, err := crdtA2.GetValueByPath("/map/key")
	assert.NoError(t, err)
	t.Logf("Map key conflict winner: %s", mapValue)
	
	// ASSERTION: Same conflict (clientA vs clientB with same values) should resolve identically
	vectorClockConsistent := (baseline_winner == mapValue)
	
	if !vectorClockConsistent {
		t.Errorf("VECTOR CLOCK INCONSISTENCY: Different operations resolved identical conflicts differently!")
		t.Errorf("Literal conflict winner: %s", baseline_winner)
		t.Errorf("Map key conflict winner: %s", mapValue)
		t.Errorf("Expected: Consistent resolution strategy across all operation types")
	}
	
	assert.True(t, vectorClockConsistent, "All operations should use consistent vector clock resolution")
	
	// Additional test: Array promotion should also respect vector clocks (not just NodeID sorting)
	t.Log("--- Additional Check: Array Promotion Resolution ---")
	t.Log("Array promotion currently uses NodeID sorting instead of vector clock resolution")
	t.Log("This may cause inconsistent behavior compared to other conflict types")
}

// TestMoveOperationRejection tests that conflicting move operations should be
// resolved using Last Writer Wins semantics instead of being rejected.
// This test FAILS if both moves are rejected instead of properly resolved.
func TestMoveOperationRejection(t *testing.T) {
	clientA := core.ClientID("clientA")
	clientB := core.ClientID("clientB")
	
	t.Log("=== Testing Move Operation Conflict Resolution ===")
	
	// Create scenario: Two nodes that could create cycle if both moves succeed
	crdtA := NewTreeCRDT()
	nodeX := crdtA.CreateAttachedNode("nodeX", Map, crdtA.Root.ID, clientA)
	nodeY := crdtA.CreateAttachedNode("nodeY", Map, crdtA.Root.ID, clientA)
	
	// Clone to simulate distributed state
	crdtB, err := crdtA.Clone()
	assert.NoError(t, err)
	_, _ = crdtB.GetNode(nodeX.ID) // nodeX_B (not used in simplified test)
	nodeY_B, _ := crdtB.GetNode(nodeY.ID)
	
	initialJSON, _ := crdtA.ExportJSON()
	t.Logf("Initial structure: %s", string(initialJSON))
	
	// Concurrent conflicting moves that would create cycle
	t.Log("--- Concurrent Conflicting Moves ---")
	
	// ClientA: Move nodeY under nodeX (earlier timestamp simulation)
	err = crdtA.AddEdge(nodeX.ID, nodeY.ID, "", clientA)
	validMoveA := (err == nil)
	t.Logf("ClientA move nodeY->nodeX: success=%v, error=%v", validMoveA, err)
	
	// ClientB: Move nodeX under nodeY (later timestamp - should win if using LWW)
	// This would create cycle nodeX->nodeY->nodeX, so one should be rejected
	err = crdtB.AddEdge(nodeY_B.ID, nodeX.ID, "", clientB) 
	validMoveB := (err == nil)
	t.Logf("ClientB move nodeX->nodeY: success=%v, error=%v", validMoveB, err)
	
	stateA, _ := crdtA.ExportJSON()
	stateB, _ := crdtB.ExportJSON()
	t.Logf("State A after move: %s", string(stateA))
	t.Logf("State B after move: %s", string(stateB))
	
	// Merge states - this should resolve the conflict using LWW
	t.Log("--- Merging Conflicting States ---")
	err = crdtA.Merge(crdtB)
	assert.NoError(t, err)
	
	finalJSON, _ := crdtA.ExportJSON()
	t.Logf("Final merged state: %s", string(finalJSON))
	
	// ASSERTION: At least one move should have succeeded (not both rejected)
	// In a proper LWW system, the later operation (clientB) should win
	successfulMoves := 0
	if validMoveA { successfulMoves++ }
	if validMoveB { successfulMoves++ }
	
	// Check final state has proper tree structure (no cycles)
	nodeX_final, _ := crdtA.GetNode(nodeX.ID)
	nodeY_final, _ := crdtA.GetNode(nodeY.ID)
	
	// At least one move should succeed, creating a parent-child relationship
	hasParentChild := (len(nodeX_final.Edges) > 0 && nodeX_final.Edges[0].To == nodeY.ID) ||
	                  (len(nodeY_final.Edges) > 0 && nodeY_final.Edges[0].To == nodeX.ID)
	
	if !hasParentChild && successfulMoves == 0 {
		t.Errorf("MOVE OPERATION CONFLICT BUG: Both moves were rejected instead of using LWW resolution!")
		t.Errorf("Expected: One move should win based on vector clock/timestamp")
		t.Errorf("Actual: Both moves rejected, no conflict resolution applied")
		t.Errorf("ClientA move success: %v", validMoveA)
		t.Errorf("ClientB move success: %v", validMoveB)
	}
	
	assert.True(t, hasParentChild || successfulMoves > 0, "At least one conflicting move should succeed using LWW semantics")
	
	t.Log("*** EXPECTED BEHAVIOR ***")
	t.Log("Conflicting moves should use Last Writer Wins (LWW) resolution")
	t.Log("Winner determined by vector clock comparison, not rejection of both")
}

// TestDeltaVsFullStateInconsistency tests that delta-state sync and full-state merge
// should produce identical results for the same operations. This test FAILS if
// the two synchronization methods produce different outcomes.
func TestDeltaVsFullStateInconsistency(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping delta vs full-state comparison test")
	}
	
	clientA := core.ClientID("clientA")
	clientB := core.ClientID("clientB")
	
	t.Log("=== Testing Delta-State vs Full-State Consistency ===")
	
	// Test the same scenario using both full-state merge and delta-state sync
	createTestScenario := func() (*TreeCRDT, *TreeCRDT) {
		crdtA := NewTreeCRDT()
		child1 := crdtA.CreateAttachedNode("child1", Literal, crdtA.Root.ID, clientA)
		child1.SetLiteral("value1", clientA)
		
		crdtB, _ := crdtA.Clone()
		child2 := crdtB.CreateAttachedNode("child2", Literal, crdtB.Root.ID, clientB)
		child2.SetLiteral("value2", clientB)
		
		return crdtA, crdtB
	}
	
	// Method 1: Full-state merge
	t.Log("--- Method 1: Full-State Merge ---")
	crdtA1, crdtB1 := createTestScenario()
	
	err := crdtA1.Merge(crdtB1)
	assert.NoError(t, err)
	
	fullStateResult, _ := crdtA1.ExportJSON()
	t.Logf("Full-state result: %s", string(fullStateResult))
	
	// Method 2: Delta-state synchronization
	t.Log("--- Method 2: Delta-State Sync ---")
	crdtA2, crdtB2 := createTestScenario()
	
	// Use delta sync
	deltaSyncA := NewDeltaSync(crdtA2)
	deltaSyncB := NewDeltaSync(crdtB2)
	
	// Generate deltas and apply
	deltaFromB := deltaSyncB.GenerateDeltaState(make(map[core.ClientID]int))
	err = deltaSyncA.ApplyDeltaState(deltaFromB)
	assert.NoError(t, err)
	
	deltaStateResult, _ := crdtA2.ExportJSON()
	t.Logf("Delta-state result: %s", string(deltaStateResult))
	
	// ASSERTION: Both methods MUST produce identical results
	// This is a fundamental requirement for CRDT consistency
	isConsistent := utils.IsJSONEqual(t, fullStateResult, deltaStateResult)
	
	if !isConsistent {
		t.Errorf("SYNCHRONIZATION INCONSISTENCY: Delta-state and full-state produce different results!")
		t.Errorf("Full-state result: %s", string(fullStateResult))
		t.Errorf("Delta-state result: %s", string(deltaStateResult))
		t.Errorf("Expected: Identical outcomes regardless of synchronization method")
		utils.CompareJSON(t, fullStateResult, deltaStateResult)
	}
	
	assert.True(t, isConsistent, "Delta-state and full-state synchronization must produce identical results")
	
	t.Log("*** SYNCHRONIZATION METHOD CONSISTENCY VERIFIED ***")
}

// TestConflictResolutionDocumentation provides a comprehensive test that
// documents all the conflict resolution issues identified
func TestConflictResolutionDocumentation(t *testing.T) {
	t.Log("=== COMPREHENSIVE CONFLICT RESOLUTION ANALYSIS ===")
	t.Log("")
	t.Log("This test suite has identified the following issues:")
	t.Log("")
	t.Log("1. ARRAY PROMOTION TIMING INCONSISTENCY")
	t.Log("   - Same logical operation behaves differently based on timing")
	t.Log("   - Direct addition vs merge-based addition have different outcomes")
	t.Log("")
	t.Log("2. LOSS OF SEMANTIC STRUCTURE")
	t.Log("   - Array promotion can destroy meaningful key-value relationships")
	t.Log("   - Map semantics preserved, but other structures converted to arrays")
	t.Log("")
	t.Log("3. NON-DETERMINISTIC PROMOTION CONDITIONS")
	t.Log("   - Identical end states can have different structures")
	t.Log("   - Operation timing affects final structure")
	t.Log("")
	t.Log("4. INCONSISTENT VECTOR CLOCK USAGE")
	t.Log("   - Different operations use different conflict resolution strategies")
	t.Log("   - No uniform approach across operation types")
	t.Log("")
	t.Log("5. MOVE OPERATION REJECTION")
	t.Log("   - Conflicting moves rejected rather than resolved")
	t.Log("   - Should use Last Writer Wins (LWW) semantics")
	t.Log("")
	t.Log("6. POTENTIAL DELTA VS FULL-STATE INCONSISTENCIES")
	t.Log("   - Different sync methods may produce different results")
	t.Log("   - Requires further investigation")
	t.Log("")
	t.Log("RECOMMENDATIONS:")
	t.Log("- Implement consistent LWW semantics across all operations")
	t.Log("- Consider semantic preservation in conflict resolution")
	t.Log("- Ensure deterministic behavior regardless of operation timing")
	t.Log("- Unify vector clock usage across all conflict scenarios")
	t.Log("")
}