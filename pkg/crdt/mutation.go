package crdt

import "github.com/eislab-cps/synctree/pkg/core"

type Operation int

const (
	OPSetLiteral Operation = iota
	OPCreateNode
	OPAddEdge
)

type Mutation struct {
	NodeID   core.NodeID   `json:"nodeid"`
	Op       Operation     `json:"op"`
	Key      string        `json:"key,omitempty"`
	Value    interface{}   `json:"value,omitempty"`
	ClientID core.ClientID `json:"clientid"`
	Version  int           `json:"version"`
}
