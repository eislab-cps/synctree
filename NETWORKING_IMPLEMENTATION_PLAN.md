# Delta-State CRDT Networking Layer Implementation Plan

## Overview

This plan outlines the implementation of a modular networking layer for the delta-state CRDT system. The goal is to provide a flexible, pluggable architecture that allows developers to choose their preferred transport protocols and delivery guarantees while maintaining the core delta-state CRDT semantics.

## Architecture Goals

1. **Modular Design**: Transport, delivery, and discovery layers are pluggable
2. **Multiple Communication Models**: Support causal, FIFO, best-effort delivery
3. **Protocol Agnostic**: Work with HTTP, WebSocket, gRPC, message queues, P2P
4. **Production Ready**: Handle failures, partitions, reconnections gracefully
5. **Developer Friendly**: Simple APIs with sensible defaults

## Core Abstractions

### 1. Transport Layer Interface

```go
// pkg/network/transport.go
package network

import (
    "context"
    "github.com/eislab-cps/synctree/pkg/crdt"
    "github.com/eislab-cps/synctree/pkg/core"
    "github.com/eislab-cps/synctree/pkg/vectorclock"
)

// DeltaTransport handles the actual network communication
type DeltaTransport interface {
    // Send delta to specific peer
    SendDelta(ctx context.Context, peer PeerID, delta *crdt.TreeCRDT) error
    
    // Broadcast delta to all known peers
    BroadcastDelta(ctx context.Context, delta *crdt.TreeCRDT) error
    
    // Register handler for incoming deltas
    OnDeltaReceived(handler DeltaHandler)
    
    // Peer management
    GetPeers() []PeerID
    AddPeer(peer PeerInfo) error
    RemovePeer(peer PeerID) error
    
    // Lifecycle
    Start(ctx context.Context) error
    Stop() error
    
    // Health checking
    PingPeer(ctx context.Context, peer PeerID) error
    GetPeerStatus(peer PeerID) PeerStatus
}

// PeerID uniquely identifies a peer
type PeerID string

// PeerInfo contains peer connection details
type PeerInfo struct {
    ID       PeerID
    Address  string
    Metadata map[string]interface{}
}

// PeerStatus indicates peer health
type PeerStatus int
const (
    PeerOnline PeerStatus = iota
    PeerOffline
    PeerUnknown
)

// DeltaHandler processes incoming deltas
type DeltaHandler func(from PeerID, delta *crdt.TreeCRDT) error
```

### 2. Delivery Guarantee Interface

```go
// pkg/network/delivery.go
package network

// DeliveryGuarantee controls ordering and reliability
type DeliveryGuarantee interface {
    // Deliver delta with specified ordering guarantees
    DeliverDelta(delta *crdt.TreeCRDT, version vectorclock.VectorClock, from PeerID) error
    
    // Set handler for successfully ordered deltas
    OnOrderedDelta(handler OrderedDeltaHandler)
    
    // Configure delivery parameters
    Configure(config DeliveryConfig) error
}

// OrderedDeltaHandler receives deltas in proper order
type OrderedDeltaHandler func(delta *crdt.TreeCRDT, from PeerID) error

// DeliveryConfig customizes delivery behavior
type DeliveryConfig struct {
    MaxQueueSize  int
    Timeout       time.Duration
    RetryAttempts int
    BufferSize    int
}
```

### 3. Peer Discovery Interface

```go
// pkg/network/discovery.go
package network

// PeerDiscovery finds and manages peers
type PeerDiscovery interface {
    // Discover peers in the network
    DiscoverPeers(ctx context.Context) ([]PeerInfo, error)
    
    // Announce our presence
    Announce(ctx context.Context, info PeerInfo) error
    
    // Subscribe to peer events
    OnPeerJoined(handler PeerEventHandler)
    OnPeerLeft(handler PeerEventHandler)
    
    // Start/stop discovery
    Start(ctx context.Context) error
    Stop() error
}

// PeerEventHandler handles peer lifecycle events
type PeerEventHandler func(peer PeerInfo)
```

## Implementation Components

### 1. Delivery Guarantee Implementations

#### Causal Delivery
```go
// pkg/network/delivery/causal.go
package delivery

type CausalDelivery struct {
    pendingDeltas map[vectorclock.VectorClock]*PendingDelta
    deliveredClock vectorclock.VectorClock
    handler       OrderedDeltaHandler
    mutex         sync.RWMutex
}

type PendingDelta struct {
    Delta     *crdt.TreeCRDT
    Version   vectorclock.VectorClock
    From      PeerID
    Timestamp time.Time
}

func (cd *CausalDelivery) DeliverDelta(delta *crdt.TreeCRDT, version vectorclock.VectorClock, from PeerID) error {
    cd.mutex.Lock()
    defer cd.mutex.Unlock()
    
    // Check if we can deliver immediately
    if vectorclock.DominatesOrEqual(cd.deliveredClock, version) {
        // Already delivered or causally ready
        return cd.deliverImmediately(delta, from)
    }
    
    // Queue for later delivery
    cd.pendingDeltas[version] = &PendingDelta{
        Delta:     delta,
        Version:   version,
        From:      from,
        Timestamp: time.Now(),
    }
    
    // Try to deliver any now-ready deltas
    return cd.deliverReadyDeltas()
}

func (cd *CausalDelivery) deliverReadyDeltas() error {
    changed := true
    for changed {
        changed = false
        
        for version, pending := range cd.pendingDeltas {
            if cd.canDeliver(version) {
                if err := cd.deliverImmediately(pending.Delta, pending.From); err != nil {
                    return err
                }
                
                // Update delivered clock
                cd.deliveredClock = vectorclock.MergeClocks(cd.deliveredClock, version)
                delete(cd.pendingDeltas, version)
                changed = true
            }
        }
    }
    return nil
}
```

#### FIFO Delivery
```go
// pkg/network/delivery/fifo.go
package delivery

type FIFODelivery struct {
    queues  map[PeerID]*DeltaQueue
    handler OrderedDeltaHandler
    mutex   sync.RWMutex
}

type DeltaQueue struct {
    deltas      []*PendingDelta
    nextExpected uint64
    mutex       sync.Mutex
}

func (fd *FIFODelivery) DeliverDelta(delta *crdt.TreeCRDT, version vectorclock.VectorClock, from PeerID) error {
    fd.mutex.Lock()
    queue, exists := fd.queues[from]
    if !exists {
        queue = &DeltaQueue{nextExpected: 1}
        fd.queues[from] = queue
    }
    fd.mutex.Unlock()
    
    return queue.enqueue(&PendingDelta{
        Delta:   delta,
        Version: version,
        From:    from,
    })
}
```

#### Best Effort Delivery
```go
// pkg/network/delivery/besteffort.go
package delivery

type BestEffortDelivery struct {
    handler OrderedDeltaHandler
}

func (bed *BestEffortDelivery) DeliverDelta(delta *crdt.TreeCRDT, version vectorclock.VectorClock, from PeerID) error {
    // Deliver immediately without ordering guarantees
    return bed.handler(delta, from)
}
```

### 2. Transport Implementations

#### HTTP Transport
```go
// pkg/network/transport/http.go
package transport

type HTTPTransport struct {
    server   *http.Server
    client   *http.Client
    peers    map[PeerID]PeerInfo
    handler  DeltaHandler
    config   HTTPConfig
    mutex    sync.RWMutex
}

type HTTPConfig struct {
    ListenAddr    string
    ReadTimeout   time.Duration
    WriteTimeout  time.Duration
    MaxDeltaSize  int64
    Compression   bool
}

func (ht *HTTPTransport) SendDelta(ctx context.Context, peer PeerID, delta *crdt.TreeCRDT) error {
    peerInfo := ht.peers[peer]
    if peerInfo.Address == "" {
        return fmt.Errorf("peer %s not found", peer)
    }
    
    // Serialize delta
    data, err := ht.serializeDelta(delta)
    if err != nil {
        return fmt.Errorf("failed to serialize delta: %w", err)
    }
    
    // Compress if enabled
    if ht.config.Compression {
        data, err = ht.compress(data)
        if err != nil {
            return fmt.Errorf("failed to compress delta: %w", err)
        }
    }
    
    // Send HTTP POST
    req, err := http.NewRequestWithContext(ctx, "POST", peerInfo.Address+"/delta", bytes.NewReader(data))
    if err != nil {
        return err
    }
    
    req.Header.Set("Content-Type", "application/octet-stream")
    if ht.config.Compression {
        req.Header.Set("Content-Encoding", "gzip")
    }
    
    resp, err := ht.client.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()
    
    if resp.StatusCode != http.StatusOK {
        return fmt.Errorf("peer rejected delta: %s", resp.Status)
    }
    
    return nil
}

func (ht *HTTPTransport) Start(ctx context.Context) error {
    mux := http.NewServeMux()
    mux.HandleFunc("/delta", ht.handleIncomingDelta)
    mux.HandleFunc("/ping", ht.handlePing)
    
    ht.server = &http.Server{
        Addr:         ht.config.ListenAddr,
        Handler:      mux,
        ReadTimeout:  ht.config.ReadTimeout,
        WriteTimeout: ht.config.WriteTimeout,
    }
    
    go func() {
        if err := ht.server.ListenAndServe(); err != http.ErrServerClosed {
            // Log error
        }
    }()
    
    return nil
}
```

#### WebSocket Transport
```go
// pkg/network/transport/websocket.go
package transport

type WebSocketTransport struct {
    connections map[PeerID]*websocket.Conn
    server      *http.Server
    handler     DeltaHandler
    config      WebSocketConfig
    mutex       sync.RWMutex
}

type WebSocketConfig struct {
    ListenAddr      string
    MaxMessageSize  int64
    PingInterval    time.Duration
    PongTimeout     time.Duration
    BufferSize      int
}

// Real-time bidirectional communication
func (wst *WebSocketTransport) handleConnection(conn *websocket.Conn, peerID PeerID) {
    defer conn.Close()
    
    conn.SetReadLimit(wst.config.MaxMessageSize)
    conn.SetPongHandler(func(string) error {
        return conn.SetReadDeadline(time.Now().Add(wst.config.PongTimeout))
    })
    
    // Start ping ticker
    ticker := time.NewTicker(wst.config.PingInterval)
    defer ticker.Stop()
    
    for {
        select {
        case <-ticker.C:
            if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
                return // Connection closed
            }
            
        default:
            messageType, data, err := conn.ReadMessage()
            if err != nil {
                return // Connection closed
            }
            
            if messageType == websocket.BinaryMessage {
                delta, err := wst.deserializeDelta(data)
                if err != nil {
                    continue // Skip malformed messages
                }
                
                wst.handler(peerID, delta)
            }
        }
    }
}
```

### 3. Discovery Implementations

#### Static Discovery
```go
// pkg/network/discovery/static.go
package discovery

type StaticDiscovery struct {
    peers   []PeerInfo
    joinedHandler  PeerEventHandler
    leftHandler    PeerEventHandler
}

func NewStaticDiscovery(peers []PeerInfo) *StaticDiscovery {
    return &StaticDiscovery{peers: peers}
}

func (sd *StaticDiscovery) DiscoverPeers(ctx context.Context) ([]PeerInfo, error) {
    return sd.peers, nil
}
```

#### mDNS Discovery
```go
// pkg/network/discovery/mdns.go
package discovery

type MDNSDiscovery struct {
    serviceName string
    port        int
    server      *mdns.Server
    resolver    *mdns.Resolver
    peers       map[string]PeerInfo
    mutex       sync.RWMutex
}

func (md *MDNSDiscovery) Announce(ctx context.Context, info PeerInfo) error {
    host, err := os.Hostname()
    if err != nil {
        return err
    }
    
    service, err := mdns.NewMDNSService(
        string(info.ID),
        md.serviceName,
        "",
        host,
        md.port,
        nil,
        []string{"txtv=1", fmt.Sprintf("id=%s", info.ID)},
    )
    if err != nil {
        return err
    }
    
    md.server, err = mdns.NewServer(&mdns.Config{Zone: service})
    return err
}
```

## Main Network Coordinator

```go
// pkg/network/coordinator.go
package network

type DeltaNetwork struct {
    transport  DeltaTransport
    delivery   DeliveryGuarantee
    discovery  PeerDiscovery
    deltaSync  *crdt.DeltaSync
    
    // State
    localPeerID PeerID
    peers       map[PeerID]PeerInfo
    metrics     *NetworkMetrics
    
    // Configuration
    config NetworkConfig
    
    // Lifecycle
    ctx    context.Context
    cancel context.CancelFunc
    wg     sync.WaitGroup
}

type NetworkConfig struct {
    LocalPeerID      PeerID
    SyncInterval     time.Duration
    HealthCheckInterval time.Duration
    MaxConcurrentSyncs  int
    RetryAttempts       int
}

func NewDeltaNetwork(transport DeltaTransport, delivery DeliveryGuarantee, discovery PeerDiscovery) *DeltaNetwork {
    return &DeltaNetwork{
        transport: transport,
        delivery:  delivery,
        discovery: discovery,
        peers:     make(map[PeerID]PeerInfo),
        metrics:   NewNetworkMetrics(),
    }
}

func (dn *DeltaNetwork) RegisterDeltaSync(deltaSync *crdt.DeltaSync) {
    dn.deltaSync = deltaSync
    
    // Set up handlers
    dn.transport.OnDeltaReceived(dn.handleIncomingDelta)
    dn.delivery.OnOrderedDelta(dn.handleOrderedDelta)
    dn.discovery.OnPeerJoined(dn.handlePeerJoined)
    dn.discovery.OnPeerLeft(dn.handlePeerLeft)
}

func (dn *DeltaNetwork) Start(ctx context.Context) error {
    dn.ctx, dn.cancel = context.WithCancel(ctx)
    
    // Start components
    if err := dn.transport.Start(dn.ctx); err != nil {
        return fmt.Errorf("failed to start transport: %w", err)
    }
    
    if err := dn.discovery.Start(dn.ctx); err != nil {
        return fmt.Errorf("failed to start discovery: %w", err)
    }
    
    // Start background routines
    dn.wg.Add(2)
    go dn.syncLoop()
    go dn.healthCheckLoop()
    
    return nil
}

func (dn *DeltaNetwork) syncLoop() {
    defer dn.wg.Done()
    
    ticker := time.NewTicker(dn.config.SyncInterval)
    defer ticker.Stop()
    
    for {
        select {
        case <-dn.ctx.Done():
            return
        case <-ticker.C:
            dn.performPeriodicSync()
        }
    }
}

func (dn *DeltaNetwork) performPeriodicSync() {
    peers := dn.transport.GetPeers()
    
    // Limit concurrent syncs
    semaphore := make(chan struct{}, dn.config.MaxConcurrentSyncs)
    
    for _, peerID := range peers {
        select {
        case semaphore <- struct{}{}:
            go func(peer PeerID) {
                defer func() { <-semaphore }()
                dn.syncWithPeer(peer)
            }(peerID)
        case <-dn.ctx.Done():
            return
        }
    }
}

func (dn *DeltaNetwork) syncWithPeer(peerID PeerID) error {
    // Get peer's last known state
    lastKnownClock := dn.getLastKnownClock(peerID)
    
    // Generate delta since last sync
    delta := dn.deltaSync.GenerateDeltaState(lastKnownClock)
    
    // Send delta
    if err := dn.transport.SendDelta(dn.ctx, peerID, delta); err != nil {
        dn.metrics.RecordSyncError(peerID, err)
        return err
    }
    
    dn.metrics.RecordSyncSuccess(peerID, len(delta.Nodes))
    return nil
}
```

## Usage Examples

### Basic HTTP Setup
```go
// Basic HTTP transport with causal delivery
transport := transport.NewHTTPTransport(transport.HTTPConfig{
    ListenAddr: ":8080",
    ReadTimeout: 30 * time.Second,
    WriteTimeout: 30 * time.Second,
    Compression: true,
})

delivery := delivery.NewCausalDelivery()

discovery := discovery.NewStaticDiscovery([]network.PeerInfo{
    {ID: "peer1", Address: "http://localhost:8081"},
    {ID: "peer2", Address: "http://localhost:8082"},
})

network := network.NewDeltaNetwork(transport, delivery, discovery)
network.RegisterDeltaSync(deltaSync)

if err := network.Start(context.Background()); err != nil {
    log.Fatal(err)
}
```

### WebSocket with mDNS Discovery
```go
// Real-time WebSocket with automatic peer discovery
transport := transport.NewWebSocketTransport(transport.WebSocketConfig{
    ListenAddr: ":9090",
    MaxMessageSize: 1024 * 1024, // 1MB
    PingInterval: 30 * time.Second,
})

delivery := delivery.NewFIFODelivery()

discovery := discovery.NewMDNSDiscovery("_crdt._tcp", 9090)

network := network.NewDeltaNetwork(transport, delivery, discovery)
```

## Implementation Phases

### Phase 1: Core Interfaces (Week 1)
- Define transport, delivery, discovery interfaces
- Implement basic HTTP transport
- Implement best-effort delivery
- Implement static discovery
- Basic integration tests

### Phase 2: Advanced Transports (Week 2)
- WebSocket transport implementation
- gRPC transport implementation
- Message queue transport (Kafka/RabbitMQ)
- Transport-specific optimizations

### Phase 3: Delivery Guarantees (Week 3)
- Causal delivery implementation
- FIFO delivery implementation
- Delivery queue management
- Timeout and retry logic

### Phase 4: Discovery Systems (Week 4)
- mDNS discovery implementation
- Consul/etcd discovery implementation
- Peer health monitoring
- Dynamic peer management

### Phase 5: Production Features (Week 5-6)
- Comprehensive error handling
- Metrics and monitoring
- Load balancing and failover
- Security (TLS, authentication)
- Performance optimization

### Phase 6: Testing & Documentation (Week 7)
- Integration tests across all combinations
- Performance benchmarks
- Documentation and examples
- Developer guides

## Success Criteria

1. **Modularity**: Can swap transport/delivery/discovery independently
2. **Reliability**: Handles network failures gracefully
3. **Performance**: Efficient delta distribution with minimal overhead
4. **Usability**: Simple API for common use cases
5. **Compatibility**: Works with existing delta-state CRDT implementation
6. **Scalability**: Supports hundreds of peers with reasonable performance

## Dependencies

- `github.com/gorilla/websocket` for WebSocket transport
- `github.com/hashicorp/mdns` for mDNS discovery  
- `google.golang.org/grpc` for gRPC transport
- `github.com/hashicorp/consul/api` for Consul discovery
- `github.com/Shopify/sarama` for Kafka transport

This networking layer will provide the infrastructure needed to make the delta-state CRDT truly distributed and production-ready.