[![codecov](https://codecov.io/gh/eislab-cps/synctree/branch/main/graph/badge.svg)](https://codecov.io/gh/eislab-cps/synctree)
[![Go](https://github.com/eislab-cps/synctree/actions/workflows/go.yml/badge.svg)](https://github.com/eislab-cps/synctree/actions/workflows/go.yml)

![SyncTree logo](./logo.png)

# Introduction
**SyncTree** is a **CRDT-based tree** with built-in:

- **Conflict-free merge** across distributed replicas  
- **ECDSA crypto** for node signatures and identities (Web3 identities) 
- **ABAC (Attribute-Based Access Control)** for fine-grained permissions  

**Key Properties:**
- **Conflict-free:** No need for manual conflict resolution
- **Strong eventual consistency:** All replicas converge automatically
- **Offline-capable:** Changes can be made locally and merged later
- **Deterministic merge:** The merge process always produces the same result
- **Self-sovereign identity (Web3) & self-verifiability** The entire CRDT tree — including identities, structure, and data — is cryptographically self-verifiable and controlled by the users, with no reliance on centralized authorities

**Key Features:**
- **Serialization** to/from **JSON**
- **Secure Import/Export** with signature verification
- **Tree-structured CRDT**: Nodes can be `Map`, `Array`, or `Literal`
- **Built-in cryptographic signatures** (ECDSA / SHA3)
- **Per-node ABAC policy** with recursive inheritance
- **Offline-capable & mergeable**: supports **merge** & **replay of deltas** of divergent replicas
- **JSON Pointer support** Supports querying the CRDT tree using JSON Pointers expressions [RFC 6901](https://datatracker.ietf.org/doc/html/rfc6901)
- **Event-driven programming** Subscribe to changes in the CRDT tree and trigger actions when updates occur — enabling reactive applications and real-time integrations

## What is a CRDT?
A [**CRDT** (Conflict-free Replicated Data Type)](https://en.wikipedia.org/wiki/Conflict-free_replicated_data_type) is a data structure designed for distributed systems, allowing multiple replicas to be updated independently and concurrently without coordination.

CRDTs guarantee that all replicas will eventually **converge to the same state**, regardless of the order of updates or network delays.

The CRDT in SyncTree is based on the following algorithms:
- **Last-Writer-Wins Register** — implemented using vector clocks  
- **LSEQ** — To handle merge of ordered sequences, originally designed for efficient collaborative editing. Reference: [LSEQ — An adaptive structure for sequences in distributed collaborative editing](https://hal.inria.fr/hal-00921633/document)_

## Potential Applications
- **Collaborative editing**  
  Real-time editing of documents, code, or data structures.
- **State Reconsolidation in Edge-Cloud Computing Continuums**  
  Reconciling state across distributed systems, such as Edge-Cloud Computing continuums.
- **Edge Computing on Satellites**
  Satellites and industrial systems often operate with intermittent or delayed connectivity — CRDTs enable safe local updates and later synchronization.
- **Digital Asset Management**  
  E.g. **Digital Product Passport**, with fine-grained access control.
- **Decentralized Applications (DApps)**  
  Peer-to-peer Applications with conflict-free data structures.
- **Decentralized Service Registries**  
  Decentralized Service Registry in SOA or Microservices architectures (e.g. [Eclipse Arrowhead](https://arrowhead.eu/eclipse-arrowhead-2)).
- **Agentic AI Systems**  
  AI agents with shared state and fine-grained access control.

# Getting started
## Web3 Identity in SyncTree
In **SyncTree**, an **identity** is based on **ECDSA cryptographic keys** — the same primitives used in many **Web3 ecosystems** (e.g. Ethereum, Polkadot, Cosmos).

- The **private key** is a securely generated ECDSA key.
- The **identity (ID)** is simply the **SHA3 hash** of the public key (derived from the private key).

This means:
Your **identity is self-sovereign** — owned and controlled by you.  
It is **portable** — can be used across decentralized applications (DApps), blockchains, and SyncTree instances.  
It is **verifiable** — other peers can verify your node signatures using your public key.  
It requires **no centralized authority** — no login, no passwords, no central server.  

In short — **SyncTree uses Web3-style identities** to ensure that:
- Every change to the tree is **signed** by a user identity.
- The entire tree is **cryptographically verifiable** (who made which change, when, and whether it is authorized).
- Fine-grained **access control (ABAC)** can be enforced per node, based on identities.

### Generate a ECDSA Private key
```console
synctree key generate
```

```console
INFO[0000] Generated new private key                     Id=aad2acc278f5ae57515b188ac3185b1da6153f177c7ee892cb18b6c2b7f802e4 PrvKey=bcbf7cd574b226ac1fa3e69591b11ab9d449d6e3e6446d452698b00d4af884e5
```

## Import JSON to CRDT SyncTree
```console
synctree import --json ./viewer/example.json --crdt tree.json --prvkey bcbf7cd574b226ac1fa3e69591b11ab9d449d6e3e6446d452698b00d4af884e5 --print
```

## Export back to JSON
```console
synctree export --json ./j1.json --crdt tree.json --prvkey bcbf7cd574b226ac1fa3e69591b11ab9d449d6e3e6446d452698b00d4af884e5 --print
```

## CRDT Viwer
**CRDT Viewer** is a tool for visualizing CRDT tree structures.  

To use the viewer:

1. Open the `viewer.html` file located in the `viewer` directory.
2. Drag and drop a CRDT file (e.g., `example_crdt.json`) into the browser window.

![CRDT Tree Viewer](./viewer.png)
