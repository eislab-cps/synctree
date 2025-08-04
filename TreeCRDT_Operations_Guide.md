# TreeCRDT Operations and Conflict Resolution Guide

## Overview

This document describes the operations, rules, and conflict resolution mechanisms in the TreeCRDT implementation. The TreeCRDT is a conflict-free replicated data type that maintains a tree structure while allowing concurrent modifications across multiple clients.

## Core Concepts

### Node Types
- **Root**: The single root node of the tree
- **Map**: Container nodes that store key-value pairs  
- **Array**: Ordered container nodes (can be promoted from other types)
- **Literal**: Leaf nodes containing actual values

### Key Properties
- **Vector Clocks**: Each node maintains a vector clock for conflict resolution
- **Client Ownership**: Each node has an owner (clientID)
- **Tree Structure**: Maintained through parent-child relationships via edges
- **Edge Labels**: Edges can have labels (used for map keys)
- **LSEQ Positioning**: Arrays use LSEQ algorithm for ordering

## Node Operations

### 1. Node Addition

#### Creating New Nodes

**Function**: `CreateAttachedNode(name, nodeType, parentID, clientID)`

**Process**:
1. Generate random NodeID based on name
2. Create node with specified type
3. Set initial vector clock `{clientID: 1}`
4. Attach to parent via `AddEdge`
5. Notify subscribers

**Scenarios**:
- ✅ **Success**: Parent exists, no conflicts
- ❌ **Cycle Detection**: Would create parent-child cycle
- ❌ **Multiple Parents**: Target node already has a parent

**Example**:
```go
parent := crdt.CreateAttachedNode("parent", Map, crdt.Root.ID, clientA)
child := crdt.CreateAttachedNode("child", Literal, parent.ID, clientA)
```

#### Adding Edges Between Existing Nodes

**Function**: `AddEdge(from, to, label, clientID)`

**Validation Rules**:
1. **Self-Reference Check**: `from != to`
2. **Cycle Detection**: DFS traversal from `to` node cannot reach `from`
3. **Single Parent Rule**: `to` node cannot already have a parent
4. **Node Existence**: Both nodes must exist in the CRDT

**Conflict Resolution**:
- Uses vector clock comparison
- Lower clientID wins ties
- Failed additions are silently ignored (no error)

### 2. Node Removal

#### Soft Deletion

**Function**: `MarkDeleted(clientID)`

**Process**:
1. Calculate next version: `max(vectorClock) + 1`
2. Create new vector clock with updated version
3. Resolve conflicts using vector clock + clientID
4. Mark `IsDeleted = true` if operation wins
5. Notify subscribers

**Scenarios**:
- ✅ **Success**: Higher vector clock version
- ❌ **Rejected**: Lower vector clock version or losing clientID in tie

#### Edge Removal

**Function**: `RemoveEdge(from, to, clientID)`

**Process**:
1. Increment sender's vector clock
2. Apply conflict resolution
3. Remove edge if operation wins
4. Unlink child's parent reference
5. Notify subscribers

**Effects**:
- Child node becomes orphaned (but not deleted)
- `Tidy()` operation can later remove unreferenced nodes

### 3. Node Editing

#### Literal Value Changes

**Function**: `SetLiteral(value, clientID)` or `setLiteralWithVersion(value, clientID, version)`

**Conflict Resolution**:
1. **Vector Clock Comparison**: Higher version wins
2. **Tie Breaking**: Lower clientID wins
3. **Last Writer Wins**: Among concurrent operations

**Example Conflict**:
```
ClientA: node.SetLiteral("valueA", clientA) // version 1
ClientB: node.SetLiteral("valueB", clientB) // version 1 (concurrent)
// Result: Determined by clientID comparison (A < B ? "valueA" : "valueB")
```

#### Map Operations

**Function**: `SetKeyValue(key, value, clientID)`

**Process**:
1. **Existing Key**: Update literal value using conflict resolution
2. **New Key**: Create new literal node and edge with label
3. **Version Management**: Increment vector clock appropriately

**Scenarios**:
- ✅ **New Key**: Always succeeds
- 🔄 **Existing Key**: Subject to vector clock conflicts
- ❌ **Non-Map Node**: Operation fails with error

### 4. Array Operations

#### Ordered Insertions

**Functions**:
- `PrependEdge(from, to, label, clientID)`: Insert at beginning
- `AppendEdge(from, to, label, clientID)`: Insert at end  
- `InsertEdgeLeft(from, to, label, sibling, clientID)`: Insert before sibling
- `InsertEdgeRight(from, to, label, sibling, clientID)`: Insert after sibling

**LSEQ Algorithm**:
- Generates position between left and right boundaries
- Ensures total ordering across concurrent insertions
- Positions are deterministic and comparable

**Conflict Resolution**:
- LSEQ positions resolve ordering conflicts
- Vector clocks resolve insertion conflicts
- Duplicate insertions are ignored

## Conflict Resolution Mechanisms

### 1. Vector Clock Resolution

**Algorithm**: `ResolveConflict(localClock, remoteClock, localOwner, remoteOwner)`

**Rules**:
1. **Dominance**: If one clock dominates (all versions ≥), it wins
2. **Tie Breaking**: If clocks are concurrent, lower clientID wins
3. **Equality**: If identical, maintain current state

### 2. Array Promotion

**Trigger Conditions**:
- Node has exactly 1 child
- Node is NOT Map or Array type
- Second child being added during merge

**Process**:
1. Create new Array node with `IsPromoted = true`
2. Move existing child to array
3. Add new child to array
4. Sort children by NodeID for determinism
5. Link parent to promoted array

**Example**:
```
Before: root -> literalChild
During merge: + secondChild
After: root -> promotedArray[IsPromoted=true] -> [child1, child2] (sorted by ID)
```

### 3. Cycle Prevention

**Detection**: DFS traversal during `validAttachment()`

**Prevention Rules**:
1. **Self-Reference**: Node cannot be its own parent
2. **Ancestral Check**: Node cannot become parent of its ancestor
3. **Real-time Validation**: Check performed before every edge addition

**Concurrency Handling**:
- Operations creating cycles are rejected
- No automatic conflict resolution for cycles
- Client must retry with valid operations

### 4. Multiple Parent Prevention

**Rule**: Each node can have at most one parent

**Enforcement**:
- `validAttachment()` checks for existing parents
- Operations attempting multiple parents are rejected
- Existing parent relationship must be removed first

## Concurrency Scenarios

### 1. Concurrent Node Additions

**Scenario**: Two clients add different children to same parent
```
ClientA: parent.AddChild(childA)
ClientB: parent.AddChild(childB)
```

**Resolution**:
- Both children are added (no conflict)
- May trigger array promotion if conditions met
- Order determined by LSEQ positions in arrays

### 2. Concurrent Edits to Same Node

**Scenario**: Two clients edit same literal value
```
ClientA: node.SetLiteral("valueA")  // concurrent
ClientB: node.SetLiteral("valueB")  // concurrent
```

**Resolution**:
- Vector clock + clientID determines winner
- Losing edit is discarded
- Deterministic outcome across all replicas

### 3. Concurrent Move Operations

**Scenario**: Moving nodes that could create cycles
```
ClientA: Move nodeX under nodeY
ClientB: Move nodeY under nodeX  // Creates cycle
```

**Current Resolution**:
- Cycle detection prevents invalid moves
- Operations creating cycles are rejected
- No automatic rearrangement

**⚠️ Note - Desired Behavior**: 
The ideal behavior would be to use **Last Writer Wins (LWW)** semantics when illegal moves are detected. This would require complex logic to:
1. Detect the conflicting move operations
2. Compare their vector clocks/timestamps
3. Apply the "winning" move and reject the "losing" move
4. Maintain tree structure integrity

This approach makes more sense than:
- ❌ Discarding both move operations (current behavior)
- ❌ Attempting to mix/merge moves (would violate tree structure rules - BAD!)

The LWW approach would preserve user intent while maintaining structural consistency, but requires sophisticated conflict resolution logic that considers the temporal ordering of move operations across the distributed system.

### 4. Parent-Child Relationship Conflicts

**Scenario**: Node deletion while children are being added
```
ClientA: parent.MarkDeleted()
ClientB: parent.AddChild(newChild)
```

**Resolution**:
- Vector clock resolution determines order
- If deletion wins: child addition may be rejected
- If addition wins: deletion may be delayed

## List Handling

### Current Implementation

**Array Nodes**: 
- Use LSEQ algorithm for positioning
- Maintain total order across concurrent operations
- Support insert, append, prepend operations

**Edge Sorting**:
- Edges sorted by LSEQ position
- `sortEdgesByLSEQ()` maintains consistent ordering
- Used during export and validation

### Operations on Lists

1. **Insert at Position**: `InsertEdgeLeft/Right()`
2. **Append/Prepend**: `AppendEdge()`, `PrependEdge()`
3. **Remove Item**: `RemoveEdge()` + mark deleted
4. **Reorder**: Not directly supported (would require new positions)

## Tree Structure Validation

### Validation Rules

**Function**: `ValidateTree()`

**Checks**:
1. **Single Root**: Exactly one root node exists
2. **Node Types**: Each node has exactly one type (Map/Array/Literal)
3. **Tree Property**: No cycles, each node has ≤1 parent
4. **Child Constraints**: Literals cannot have children
5. **Reachability**: All nodes reachable from root

### Structural Integrity

**Prevention Mechanisms**:
- `validAttachment()` checks before edge creation
- Vector clock conflicts prevent inconsistent states
- Cycle detection during merge operations

**Recovery Mechanisms**:
- `Tidy()` removes orphaned nodes
- Validation can detect and report corruption
- Manual intervention may be required for cycles

## Questionable Conflict Resolution Behaviors

### 1. Array Promotion Timing Issues

**Problem**: Array promotion only occurs during `Merge()` operations, not during regular `AddEdge()` calls. This creates inconsistent behavior:

```go
// Scenario 1: Direct addition (no promotion)
parent.AddEdge(child1)  // parent has 1 child
parent.AddEdge(child2)  // parent has 2 children - NO promotion

// Scenario 2: Merge-based addition (promotion occurs)
treeA: parent -> child1
treeB: parent -> child2  
treeA.Merge(treeB)  // -> array promotion happens
```

**Issue**: The same logical operation (parent getting 2 children) behaves differently based on how it occurs.

### 2. Array Promotion Loss of Structure

**Problem**: When array promotion occurs, the original semantic structure is lost:

```go
// Before: Structured map with meaningful keys
{
  "user": {"name": "Alice", "role": "admin"}
}

// After concurrent addition + merge: Becomes array with no key semantics
[
  {"name": "Alice", "role": "admin"},
  {"name": "Bob", "role": "user"}  
]
```

**Issue**: Keys and semantic relationships are discarded, converting meaningful structure into ordered list.

### 3. Non-Deterministic Promotion Conditions

**Problem**: Array promotion depends on exact timing and merge order:

```go
// Scenario A: No promotion (additions happen before merge)
clientA: parent.AddChild(child1); parent.AddChild(child2)
clientB: // no changes
merge() // No promotion - parent already has 2 children

// Scenario B: Promotion occurs (additions happen during merge)  
clientA: parent.AddChild(child1)
clientB: parent.AddChild(child2)
merge() // Promotion occurs - conflict detected
```

**Issue**: Identical end states can have different structures based on operation ordering.

### 4. Inconsistent Vector Clock Resolution

**Problem**: Different operations use vector clocks inconsistently:

- **Node values**: Use proper vector clock comparison with tie-breaking
- **Edge additions**: May ignore conflicts silently 
- **Array promotions**: Use deterministic NodeID sorting but bypass vector clocks

**Issue**: No uniform conflict resolution strategy across operation types.

### 5. Move Operation Rejection

**Problem**: As noted earlier, conflicting move operations are rejected rather than resolved:

```go
clientA: Move nodeX under nodeY
clientB: Move nodeY under nodeX  // Would create cycle
// Result: Both operations rejected, no clear resolution
```

**Desired**: Last Writer Wins (LWW) semantics with proper vector clock resolution.

### 6. Delta-State vs Full-State Inconsistencies

**Potential Issue**: Array promotion behavior may differ between delta-state synchronization and full-state merges due to different code paths and context availability.

## Error Handling and Edge Cases

### 1. Invalid Operations

**Response**: Operations that would break tree structure are rejected

**Examples**:
- Creating cycles
- Multiple parents
- Invalid node type operations

### 2. Concurrent Deletions

**Scenario**: Node deleted while being modified
- Deletion may win: modifications ignored
- Modification may win: deletion delayed
- Vector clock resolution provides consistency

### 3. Network Partitions

**Behavior**:
- Operations continue locally
- Conflicts resolved during merge
- Array promotion may occur during reunion
- Some operations may be rejected post-merge

### 4. Malformed Merges

**Handling**:
- Secure merge validates before applying
- Clone-and-validate approach prevents corruption
- Failed merges leave original state intact

## Best Practices

### 1. Operation Ordering

- Batch related operations when possible
- Consider vector clock implications
- Use appropriate clientID naming for deterministic tie-breaking

### 2. Conflict Minimization

- Coordinate major structural changes
- Use unique node names to avoid ID collisions
- Consider operation semantics when designing conflicts

### 3. Performance Considerations

- Call `Tidy()` periodically to clean orphaned nodes
- Monitor tree depth and branching factor
- Use appropriate data structures for expected access patterns

### 4. Error Recovery

- Validate tree structure after major operations
- Handle operation rejections gracefully
- Implement retry logic for transient failures

## Conclusion

The TreeCRDT provides robust conflict resolution for collaborative tree editing through:

- **Vector Clock Ordering**: Ensures consistent operation ordering
- **Structural Validation**: Maintains tree properties under concurrency
- **Array Promotion**: Handles conflicting children elegantly
- **Deterministic Resolution**: Guarantees identical outcomes across replicas

Understanding these mechanisms is crucial for building applications that leverage the TreeCRDT's capabilities while avoiding common pitfalls in distributed tree editing.