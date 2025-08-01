# Array Promotion in Delta-State CRDTs: Behavior Analysis

## Overview

This document analyzes the current array promotion behavior in TreeCRDT when used with delta-state synchronization, identifies the core issue, and outlines potential solutions.

## Current Behavior

### What Happens Now

When multiple machines generate deltas that contain overlapping tree structures, TreeCRDT's merge algorithm applies array promotion as a conflict resolution mechanism:

```json
// Machine X delta contains: /machines node, /metadata node
// Machine Y delta contains: /machines node, /metadata node  
// Machine Z delta contains: /machines node, /metadata node

// Result after merge: Deep array nesting
[
  [
    [
      {"machines": {"x": {...}}, "metadata": {...}},
      {"machines": {}, "metadata": {}}
    ],
    {"machines": {"y": {...}}, "metadata": {...}}
  ],
  {"metadata": {...}, "machines": {"z": {...}}}
]
```

### Why This Happens

1. **Delta Generation**: Each machine generates a complete TreeCRDT structure containing modified nodes
2. **Structural Overlap**: Multiple deltas contain the same tree paths (`/machines`, `/metadata`)
3. **Conflict Detection**: TreeCRDT.Merge() sees multiple versions of the same nodes
4. **Array Promotion**: TreeCRDT applies its conflict resolution by promoting to arrays

## The Core Issue

### Structural vs. Data Conflicts

There are two types of conflicts:

1. **Data Conflicts** (intended): Multiple clients modify the same data field
   - Example: Client A sets `counter = 5`, Client B sets `counter = 7`
   - Resolution: Last-writer-wins or array promotion with both values

2. **Structural Conflicts** (unintended): Multiple deltas contain the same tree nodes but with different data
   - Example: Client A's delta has `/machines` with `{x: {...}}`, Client B's delta has `/machines` with `{y: {...}}`
   - Current Resolution: Array promotion
   - **This is likely wrong** - these should merge additively //Claude
   - **But** it does make sense if the data represents something in plural, but in that case it should already have been an array right? //Dev

### Test Case Analysis

#### ✅ Working Test: `TestDeltaStateSync`
```go
// TreeA creates nodeA, TreeB creates nodeB
// No structural overlap - different node IDs
// Result: Clean merge ✓
```

#### ❌ Failing Test: `TestDeltaStateOrderIndependence`
```go
// All trees modify /machines and /metadata paths
// Structural overlap despite different keys (x, y, z)
// Result: Array promotion ❌
```

## Expected vs. Actual Behavior

### What Users Expect (Additive Merge)
```json
// Input: 3 deltas with non-conflicting data
{"machines": {"x": {...}}, "metadata": {"last_update_x": "..."}}
{"machines": {"y": {...}}, "metadata": {"last_update_y": "..."}}
{"machines": {"z": {...}}, "metadata": {"last_update_z": "..."}}

// Expected result: Clean additive merge
{
  "machines": {
    "x": {...},
    "y": {...}, 
    "z": {...}
  },
  "metadata": {
    "last_update_x": "...",
    "last_update_y": "...",
    "last_update_z": "..."
  }
}
```

### What Actually Happens (Array Promotion)
```json
// Actual result: Deep nested arrays
[[[{"machines": {"x": {...}}}, {"machines": {}}], {"machines": {"y": {...}}}], {"machines": {"z": {...}}}]
```

## Analysis: Bug or Feature?

### Evidence It's a Bug

1. **User Expectation**: Delta-state CRDTs should enable clean, additive synchronization
2. **Non-conflicting Data**: Tests use different keys (`x`, `y`, `z`) - no actual data conflicts
3. **CRDT Principles**: CRDTs should merge concurrent non-conflicting changes cleanly
4. **User Feedback**: "per node edit access" and "same granularity as treecrdt allows" suggests node-level precision

### Evidence It's Intended Behavior

1. **Consistent with TreeCRDT**: Array promotion is documented TreeCRDT behavior
2. **Conflict Resolution**: Multiple tree structures → conflict → deterministic resolution
3. **Implementation**: Using existing `TreeCRDT.Merge()` as requested by user

## Potential Solutions

### Solution 1: Fix Delta Generation Strategy

**Approach**: Generate minimal, non-overlapping state fragments

```go
// Instead of generating complete tree structures:
Delta1: {"/machines": {...}, "/metadata": {...}}  // OVERLAPS
Delta2: {"/machines": {...}, "/metadata": {...}}  // OVERLAPS

// Generate node-level changes:
Delta1: {"/machines/x": {...}, "/metadata/last_update_x": {...}}  // NO OVERLAP
Delta2: {"/machines/y": {...}, "/metadata/last_update_y": {...}}  // NO OVERLAP
```

**Pros**:
- Eliminates structural conflicts
- Preserves clean additive merge behavior
- Aligns with user expectation of "per node edit access"

**Cons**:
- Major refactoring of delta generation
- May break existing delta serialization
- Requires careful handling of tree consistency

### Solution 2: Accept Array Promotion as Intended

**Approach**: Update tests to expect and verify array promotion behavior

```go
// Update failing tests to verify:
// 1. Array promotion occurs deterministically
// 2. All data is preserved in nested structure
// 3. Order independence holds within array promotion
```

**Pros**:
- No changes to core algorithms
- Consistent with existing TreeCRDT behavior
- Simple fix

**Cons**:
- Poor user experience (deeply nested arrays)
- Doesn't align with typical CRDT expectations
- Makes delta-state CRDTs less useful

### Solution 3: Hybrid Approach

**Approach**: Implement smarter conflict detection

```go
// Only apply array promotion for actual data conflicts
// Use additive merge for structural overlaps with different keys
if (sameTreePath && conflictingKeys) {
    additivemerge()  // machines.x + machines.y = clean merge
} else if (sameTreePath && sameKeys) {
    arrayPromotion() // machines.x + machines.x = array promotion
}
```

## Recommendation

Based on the analysis, **Solution 1 (Fix Delta Generation Strategy)** appears to be the correct approach because:

1. **Aligns with User Intent**: "per node edit access" suggests node-level granularity
2. **Preserves CRDT Principles**: Non-conflicting concurrent changes should merge cleanly
3. **Better User Experience**: Clean JSON structures instead of deeply nested arrays
4. **True Delta-State Behavior**: Delta-state CRDTs should minimize state transfer and conflicts

## Test Strategy

### Keep Failing Tests
The failing tests correctly identify the issue and should be preserved as regression tests.

### Add New Tests
1. **True conflict tests**: Verify array promotion when same keys are modified
2. **Additive merge tests**: Verify clean merging of non-conflicting changes
3. **Edge case tests**: Complex scenarios with mixed conflicts and non-conflicts

## Implementation Priority

1. **High Priority**: Fix delta generation to avoid structural conflicts
2. **Medium Priority**: Implement smarter conflict detection
3. **Low Priority**: Update documentation to clarify intended behavior

---

**Status**: Analysis Complete  
**Decision Needed**: Choose solution approach  
**Next Steps**: Implement chosen solution and update tests accordingly