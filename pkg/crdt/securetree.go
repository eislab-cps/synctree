package crdt

type SecureNode interface {
	// General operations
	ID() NodeID

	// Literal operations
	SetLiteral(value interface{}, prvKey string) (*Delta, error)
	GetLiteral() (interface{}, error)

	// Map operations
	CreateMapNode(prvKey string) (*Delta, SecureNode, error)
	SetKeyValue(key string, value interface{}, prvKey string) (*Delta, NodeID, error)
	GetNodeForKey(key string) (SecureNode, bool, error)
	RemoveKeyValue(key string, prvKey string) (*Delta, error)
}

type SecureTree interface {
	// ABAC (Attribute Based Access Control)
	ABAC() *ABACPolicy

	// Subscription
	Subscribe(path string, ch chan NodeEvent)

	// Node operations
	CreateAttachedNode(name string, nodeType NodeType, parentID NodeID, prvKey string) (*Delta, SecureNode, error)
	CreateNode(name string, nodeType NodeType, prvKey string) (*Delta, SecureNode, error)
	GetNode(id NodeID) (SecureNode, bool)
	GetSibling(parentNodeID NodeID, index int) (SecureNode, error)
	GetValueByPath(path string) (interface{}, error)
	GetNodeByPath(path string) (SecureNode, error)
	GetStringValueByPath(path string) (string, error)

	// Edge operations
	AddEdge(from, to NodeID, label string, prvKey string) (*Delta, error)
	RemoveEdge(from, to NodeID, prvKey string) (*Delta, error)

	// List operations
	AppendEdge(from, to NodeID, label string, prvKey string) (*Delta, error)
	PrependEdge(from, to NodeID, label string, prvKey string) (*Delta, error)
	InsertEdgeLeft(from, to NodeID, label string, sibling NodeID, prvKey string) (*Delta, error)
	InsertEdgeRight(from, to NodeID, label string, sibling NodeID, prvKey string) (*Delta, error)

	// Merge operations
	Merge(c2 SecureTree, prvKey string) error

	// Serialization
	ImportJSON(rawJSON []byte, prvKey string) (NodeID, error)
	ImportJSONToMap(rawJSON []byte, parentID NodeID, key string, prvKey string) (NodeID, error)
	ImportJSONToArray(rawJSON []byte, parentID NodeID, prvKey string) (NodeID, error)
	ExportJSON() ([]byte, error)
	Load(data []byte) error
	Save() ([]byte, error)
	Clone() (SecureTree, error)

	// Utility functions
	Tidy()
	VerifyTree() error
}
