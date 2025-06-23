package crdt

type Operation int

const (
	OPSetLiteral Operation = iota
	OPCreateNode
	OPAddEdge
)

type Mutation struct {
	NodeID   NodeID      `json:"nodeid"`
	Op       Operation   `json:"op"`
	Key      string      `json:"key,omitempty"`
	Value    interface{} `json:"value,omitempty"`
	ClientID ClientID    `json:"clientid"`
	Version  int         `json:"version"`
}
