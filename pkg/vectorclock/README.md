# Vector Clocks
## Last-Write-Wins (LWW) Register with Vector Clocks
A **Last-Write-Wins (LWW) Register** is a CRDT (Conflict-free Replicated Data Type) that stores a single value, where concurrent writes are resolved by selecting the latest write.

**Vector clocks** are used instead of physical timestamps to tract **causal history** and ensure deterministic conflict resolution.

Each register contains:
- A `value`: the current value
- A `clock`: a vector clock (`map[replicaID]int`)
- A `replica`: a globally unique ID of the replica that performed the last write
