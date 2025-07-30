package securecrdt

import (
	"github.com/eislab-cps/synctree/pkg/abac"
	"github.com/eislab-cps/synctree/pkg/core"
	"github.com/eislab-cps/synctree/pkg/crdt"
)

type SecureNodeCRDT interface {
	// General operations
	ID() core.NodeID

	// Literal operations
	SetLiteral(value interface{}, prvKey string) (crdt.Mutation, error)
	GetLiteral() (interface{}, error)

	// Map operations
	CreateMapNode(prvKey string) (crdt.Mutation, SecureNodeCRDT, error)
	SetKeyValue(key string, value interface{}, prvKey string) (crdt.Mutation, core.NodeID, error)
	GetNodeForKey(key string) (SecureNodeCRDT, bool, error)
	RemoveKeyValue(key string, prvKey string) (crdt.Mutation, error)
}

type SecureTreeCRDT interface {
	// ABAC (Attribute Based Access Control)
	ABAC() *abac.ABACPolicy

	// Subscription
	Subscribe(path string, ch chan crdt.NodeEvent)

	// Node operations
	CreateAttachedNode(name string, nodeType core.NodeType, parentID core.NodeID, prvKey string) (crdt.Mutation, SecureNodeCRDT, error)
	CreateNode(name string, nodeType core.NodeType, prvKey string) (crdt.Mutation, SecureNodeCRDT, error)
	GetNode(id core.NodeID) (SecureNodeCRDT, bool)
	GetSibling(parentNodeID core.NodeID, index int) (SecureNodeCRDT, error)
	GetValueByPath(path string) (interface{}, error)
	GetNodeByPath(path string) (SecureNodeCRDT, error)
	GetStringValueByPath(path string) (string, error)

	// Edge operations
	AddEdge(from, to core.NodeID, label string, prvKey string) (crdt.Mutation, error)
	RemoveEdge(from, to core.NodeID, prvKey string) (crdt.Mutation, error)

	// List operations
	AppendEdge(from, to core.NodeID, label string, prvKey string) (crdt.Mutation, error)
	PrependEdge(from, to core.NodeID, label string, prvKey string) (crdt.Mutation, error)
	InsertEdgeLeft(from, to core.NodeID, label string, sibling core.NodeID, prvKey string) (crdt.Mutation, error)
	InsertEdgeRight(from, to core.NodeID, label string, sibling core.NodeID, prvKey string) (crdt.Mutation, error)

	// Merge operations
	Merge(c2 SecureTreeCRDT, prvKey string) error
	ApplyMutation(mut crdt.Mutation, prvKey string) error

	// Serialization
	ImportJSON(rawJSON []byte, prvKey string) (core.NodeID, error)
	ImportJSONToMap(rawJSON []byte, parentID core.NodeID, key string, prvKey string) (core.NodeID, error)
	ImportJSONToArray(rawJSON []byte, parentID core.NodeID, prvKey string) (core.NodeID, error)
	ExportJSON() ([]byte, error)
	Load(data []byte) error
	Save() ([]byte, error)
	Clone() (SecureTreeCRDT, error)

	// Utility functions
	Tidy()
	VerifyTree() error
}
