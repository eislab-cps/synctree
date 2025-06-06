![SyncTree logo](./logo.png)

# Introduction
**SyncTree** is a **CRDT-based tree** with built-in:

- **Conflict-free merge** across distributed replicas  
- **ECDSA crypto** for node signatures  
- **ABAC (Attribute-Based Access Control)** for fine-grained permissions  

**Key Properties:**
- **Conflict-free:** No need for manual conflict resolution
- **Strong wventually consistency:** All replicas converge automatically
- **Offline-capable:** Changes can be made locally and merged later
- **Deterministic merge:** The merge process always produces the same result

**Key Features:**
- **Serialization** to/from **JSON**
- **Secure Import/Export** with signature verification
- **Tree-structured CRDT**: Nodes can be `Map`, `Array`, or `Literal`
- **Built-in cryptographic signatures** (ECDSA / SHA3)
- **Per-node ABAC policy** with recursive inheritance
- **Offline-capable & mergeable**: supports **merge** & **replay of deltas** of divergent replicas

## What is a CRDT?
A [**CRDT** (Conflict-free Replicated Data Type)](https://en.wikipedia.org/wiki/Conflict-free_replicated_data_type) is a data structure designed for distributed systems, allowing multiple replicas to be updated independently and concurrently without coordination.

CRDTs guarantee that all replicas will eventually **converge to the same state**, regardless of the order of updates or network delays.

## Potential Applications
- **Collaborative editing**  
  Real-time editing of documents, code, or data structures
- **State Reconsolidation in Edge-Cloud Computing Continuums**  
  Reconciling state across distributed systems, such as Edge-Cloud Computing continuums
- **Edge Computing on Satellites**
- Offline-first: satellites and industrial systems often operate with intermittent or delayed connectivity — CRDTs enable safe local updates and later synchronization
- **Digital Asset Management**  
  E.g. **Digital Product Pass**, with fine-grained access control
- **Decentralized Applications (DApps)**  
  Peer-to-peer Applications with conflict-free data structures
- **Decentralized Service Registries**  
  Decentralized Service Registry in SOA or Microservices architectures (e.g. [Eclipse Arrowhead](https://arrowhead.eu/eclipse-arrowhead-2))
- **Agentic AI Systems**  
  AI agents with shared state and fine-grained access control

# Getting started
## CRDT Viwer
**CRDT Viewer** is a tool for visualizing CRDT tree structures.  

To use the viewer:

1. Open the `viewer.html` file located in the `viewer` directory.
2. Drag and drop a CRDT file (e.g., `example_crdt.json`) into the browser window.


![CRDT Tree Viewer](./viewer.png)

## CLI

## Examples
