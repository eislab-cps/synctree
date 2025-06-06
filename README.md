![SyncTree logo](./logo.png)

# Introduction
**SyncTree** is a **CRDT-based tree** with built-in:

- **Conflict-free merge** across distributed replicas  
- **ECDSA crypto** for node signatures  
- **ABAC (Attribute-Based Access Control)** for fine-grained permissions  

**Key Properties:**
- **Conflict-free:** No need for manual conflict resolution
- **Strong eventual consistency:** All replicas converge automatically
- **Offline-capable:** Changes can be made locally and merged later
- **Deterministic merge:** The merge process always produces the same result
- **Self-sovereign identity & self-verifiability** The entire CRDT tree — including identities, structure, and data — is cryptographically self-verifiable and controlled by the users, with no reliance on centralized authorities

**Key Features:**
- **Serialization** to/from **JSON**
- **Secure Import/Export** with signature verification
- **Tree-structured CRDT**: Nodes can be `Map`, `Array`, or `Literal`
- **Built-in cryptographic signatures** (ECDSA / SHA3)
- **Per-node ABAC policy** with recursive inheritance
- **Offline-capable & mergeable**: supports **merge** & **replay of deltas** of divergent replicas
- **JSONPath support** Supports querying the CRDT tree using [JSONPath](https://datatracker.ietf.org/doc/html/draft-ietf-jsonpath-base) expressions
- **Event-driven programming** Subscribe to changes in the CRDT tree and trigger actions when updates occur — enabling reactive applications and real-time integrations.

## What is a CRDT?
A [**CRDT** (Conflict-free Replicated Data Type)](https://en.wikipedia.org/wiki/Conflict-free_replicated_data_type) is a data structure designed for distributed systems, allowing multiple replicas to be updated independently and concurrently without coordination.

CRDTs guarantee that all replicas will eventually **converge to the same state**, regardless of the order of updates or network delays.

The CRDT in SyncTree is based on the following algorithms:
- **Last-Writer-Wins Register** — implemented using vector clocks  
- **LSEQ** — To handle merge of ordered sequences, originally designed for efficient collaborative editing. Reference: [LSEQ — An adaptive structure for sequences in distributed collaborative editing](https://hal.inria.fr/hal-00921633/document)_

## Potential Applications
- **Collaborative editing**  
  Real-time editing of documents, code, or data structures
- **State Reconsolidation in Edge-Cloud Computing Continuums**  
  Reconciling state across distributed systems, such as Edge-Cloud Computing continuums
- **Edge Computing on Satellites**
  Satellites and industrial systems often operate with intermittent or delayed connectivity — CRDTs enable safe local updates and later synchronization
- **Digital Asset Management**  
  E.g. **Digital Product Passport**, with fine-grained access control
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
