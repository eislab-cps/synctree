# Introduction

![SyncTree logo](./logo.png)

**SyncTree** is a **CRDT-based tree** with built-in:

- **Conflict-free merge** across distributed replicas  
- **ECDSA crypto** for node signatures  
- **ABAC (Attribute-Based Access Control)** for fine-grained permissions  

**Key properties:**

- **Conflict-free:** No need for manual conflict resolution
- **Eventually consistent:** All replicas converge automatically
- **Offline-capable:** Changes can be made locally and merged later
- **Deterministic merge:** The merge process always produces the same result

## What is a CRDT?
A **CRDT** (Conflict-free Replicated Data Type) is a data structure designed for **distributed systems** where multiple replicas may update concurrently — even **offline** — without coordination.

CRDTs guarantee that all replicas will eventually **converge to the same state**, regardless of the order of updates or network delays.

## Features
- **Serialization** to/from **JSON**
- **Secure Import/Export** with signature verification
- **Tree-structured CRDT**: Nodes can be `Map`, `Array`, or `Literal`
- **Built-in cryptographic signatures** (ECDSA / SHA3)
- **Per-node ABAC policy** with recursive inheritance
- **Offline-capable & mergeable**: supports **merge** & **replay of deltas** of divergent replicas

## Potential Applications
- **Collaborative editing**  
  Real-time editing of documents, code, or data structures
- **State reconsolidation**  
  Reconciling state across distributed systems, such as Edge-Cloud Computing continuums
- **Digital Asset Management**  
  E.g. **Digital Product Pass**, with fine-grained access control
- **Decentralized Applications (DApps)**  
  Peer-to-peer applications with conflict-free data structures
- **Decentralized Service Mesh**  
  Decentralized Service Registry in SOA or Microservices architectures (e.g. Eclipse Arrowhead)
- **Agentic AI Systems**  
  AI agents with shared state and fine-grained access control
