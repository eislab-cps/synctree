package crdt

import (
	"fmt"

	"github.com/eislab-cps/synctree/internal/crypto"
	"github.com/eislab-cps/synctree/pkg/abac"
	"github.com/eislab-cps/synctree/pkg/core"
	log "github.com/sirupsen/logrus"
)

// Local interfaces to avoid circular dependencies
type SecureNode interface {
	ID() core.NodeID
	SetLiteral(value interface{}, prvKey string) error
	GetLiteral() (interface{}, error)
	CreateMapNode(prvKey string) (SecureNode, error)
	SetKeyValue(key string, value interface{}, prvKey string) (core.NodeID, error)
	GetNodeForKey(key string) (SecureNode, bool, error)
	RemoveKeyValue(key string, prvKey string) error
}

type SecureTree interface {
	ABAC() *abac.ABACPolicy
	CreateAttachedNode(name string, nodeType core.NodeType, parentID core.NodeID, prvKey string) (SecureNode, error)
	CreateNode(name string, nodeType core.NodeType, prvKey string) (SecureNode, error)
	GetNode(id core.NodeID) (SecureNode, bool)
	GetSibling(parentNodeID core.NodeID, index int) (SecureNode, error)
	GetValueByPath(path string) (interface{}, error)
	GetNodeByPath(path string) (SecureNode, error)
	GetStringValueByPath(path string) (string, error)
	AddEdge(from, to core.NodeID, label string, prvKey string) error
	RemoveEdge(from, to core.NodeID, prvKey string) error
	AppendEdge(from, to core.NodeID, label string, prvKey string) error
	PrependEdge(from, to core.NodeID, label string, prvKey string) error
	InsertEdgeLeft(from, to core.NodeID, label string, sibling core.NodeID, prvKey string) error
	InsertEdgeRight(from, to core.NodeID, label string, sibling core.NodeID, prvKey string) error
	Merge(c2 SecureTree, prvKey string) error
	ImportJSON(rawJSON []byte, prvKey string) (core.NodeID, error)
	ImportJSONToMap(rawJSON []byte, parentID core.NodeID, key string, prvKey string) (core.NodeID, error)
	ImportJSONToArray(rawJSON []byte, parentID core.NodeID, prvKey string) (core.NodeID, error)
	ExportJSON() ([]byte, error)
	Load(data []byte) error
	Save() ([]byte, error)
	Clone() (SecureTree, error)
	Tidy()
	VerifyTree() error
}

type AdapterSecureNodeCRDT struct {
	nodeCrdt *NodeCRDT
}

func performSecureAction(
	accessControl bool,
	prvKey string,
	action abac.ABACAction,
	target core.NodeID,
	abac *abac.ABACPolicy,
	actionFn func(core.ClientID) (*NodeCRDT, error),
) error {
	identity, err := crypto.CreateIdentityFromString(prvKey)
	if err != nil {
		return fmt.Errorf("failed to create identity: %w", err)
	}

	id := identity.ID()

	if accessControl {
		if abac != nil && !abac.IsAllowed(id, action, target) {
			log.WithFields(log.Fields{
				"ID":     id,
				"Action": action,
				"Target": target,
			}).Error("Not allowed to perform action on target")
			return fmt.Errorf("identity %s not allowed to perform %s on %s", id, action, target)
		}
	}

	node, err := actionFn(core.ClientID(id))
	if err != nil {
		return err
	}

	err = node.Sign(identity)
	if err != nil {
		return fmt.Errorf("failed to sign node: %w", err)
	}

	if abac != nil {
		abac.SetIdentity(identity)
	}

	return nil
}

func (n *AdapterSecureNodeCRDT) ID() core.NodeID {
	return n.nodeCrdt.ID
}

func (n *AdapterSecureNodeCRDT) SetLiteral(value interface{}, prvKey string) error { // Tested
	secureAction := func(clientID core.ClientID) (*NodeCRDT, error) {
		// TODO: Adapt to new Mutation system instead of direct modification
		_, err := n.nodeCrdt.SetLiteral(value, clientID)
		if err != nil {
			return nil, fmt.Errorf("failed to set literal: %w", err)
		}
		return n.nodeCrdt, nil
	}

	accessControl := true
	if n.nodeCrdt.ParentID == "" {
		accessControl = false // If the node is not attached to a tree, we skip ABAC checks
	}
	err := performSecureAction(
		accessControl,
		prvKey,
		abac.ActionModify,
		n.nodeCrdt.ID,
		n.nodeCrdt.tree.ABACPolicy,
		secureAction)
	return err
}

func (n *AdapterSecureNodeCRDT) GetLiteral() (interface{}, error) {
	return n.nodeCrdt.GetLiteral()
}

func (n *AdapterSecureNodeCRDT) CreateMapNode(prvKey string) (SecureNode, error) { // Tested
	var newNode *NodeCRDT

	secureAction := func(clientID core.ClientID) (*NodeCRDT, error) {
		node, err := n.nodeCrdt.CreateMapNode(clientID)
		if err != nil {
			return nil, fmt.Errorf("failed to create map node: %w", err)
		}
		newNode = node
		return n.nodeCrdt, nil
	}

	err := performSecureAction(
		true,
		prvKey,
		abac.ActionModify,
		n.nodeCrdt.ID,
		n.nodeCrdt.tree.ABACPolicy,
		secureAction)
	if err != nil {
		return nil, err
	}

	return &AdapterSecureNodeCRDT{nodeCrdt: newNode}, nil
}

func (n *AdapterSecureNodeCRDT) SetKeyValue(key string, value interface{}, prvKey string) (core.NodeID, error) { // Tested
	var newNodeID core.NodeID

	secureAction := func(clientID core.ClientID) (*NodeCRDT, error) {
		// TODO: Adapt to new Mutation system
		_, nodeID, err := n.nodeCrdt.SetKeyValue(key, value, clientID)
		if err != nil {
			return nil, fmt.Errorf("failed to set key value: %w", err)
		}
		newNodeID = nodeID
		return n.nodeCrdt, nil
	}

	err := performSecureAction(
		true,
		prvKey,
		abac.ActionModify,
		n.nodeCrdt.ID,
		n.nodeCrdt.tree.ABACPolicy,
		secureAction)
	if err != nil {
		return "", err
	}

	return newNodeID, nil
}

func (n *AdapterSecureNodeCRDT) GetNodeForKey(key string) (SecureNode, bool, error) {
	internalNode, ok, err := n.nodeCrdt.GetNodeForKey(key)
	if err != nil {
		return nil, false, err
	}
	return &AdapterSecureNodeCRDT{nodeCrdt: internalNode}, ok, nil
}

func (n *AdapterSecureNodeCRDT) RemoveKeyValue(key string, prvKey string) error { // Tested
	secureAction := func(clientID core.ClientID) (*NodeCRDT, error) {
		if err := n.nodeCrdt.RemoveKeyValue(key, clientID); err != nil {
			return nil, fmt.Errorf("failed to remove key-value: %w", err)
		}
		return n.nodeCrdt, nil
	}

	err := performSecureAction(
		true,
		prvKey,
		abac.ActionModify,
		n.nodeCrdt.ID,
		n.nodeCrdt.tree.ABACPolicy,
		secureAction,
	)
	return err
}

type AdapterSecureTreeCRDT struct {
	treeCrdt *TreeCRDT
}

func NewSecureTree(prvKey string) (SecureTree, error) {
	identity, err := crypto.CreateIdentityFromString(prvKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create identity from string: %w", err)
	}

	c := NewTreeCRDT()
	ownerID := identity.ID()
	c.ABACPolicy = abac.NewABACPolicy(c, ownerID, identity)
	c.ABACPolicy.Allow(ownerID, abac.ABACAction("*"), core.NodeID("root"), true) // Allow the owner to have full access to whole tree
	c.Secure = true

	return &AdapterSecureTreeCRDT{
		treeCrdt: c,
	}, nil
}

func (c *AdapterSecureTreeCRDT) ABAC() *abac.ABACPolicy {
	return c.treeCrdt.ABACPolicy
}

func (c *AdapterSecureTreeCRDT) CreateAttachedNode(name string, nodeType core.NodeType, parentID core.NodeID, prvKey string) (SecureNode, error) { // Tested
	var newNode *NodeCRDT

	secureAction := func(clientID core.ClientID) (*NodeCRDT, error) {
		// TODO: Adapt to new Mutation system
		node := c.treeCrdt.CreateAttachedNode(name, nodeType, parentID, clientID)
		newNode = node
		return newNode, nil
	}

	err := performSecureAction(
		true,
		prvKey,
		abac.ActionModify,
		parentID,
		c.treeCrdt.ABACPolicy,
		secureAction,
	)
	if err != nil {
		return nil, err
	}

	return &AdapterSecureNodeCRDT{nodeCrdt: newNode}, nil
}

func (c *AdapterSecureTreeCRDT) CreateNode(name string, nodeType core.NodeType, prvKey string) (SecureNode, error) { // Tested
	var newNode *NodeCRDT

	secureAction := func(clientID core.ClientID) (*NodeCRDT, error) {
		// TODO: Adapt to new Mutation system
		node := c.treeCrdt.CreateNode(name, nodeType, clientID)
		newNode = node
		return newNode, nil
	}

	// TODO: Implement proper signing with nonce
	// nonce, signature, err := crypto.SignWithRandomNonce(newNode, c.treeCrdt.ABACPolicy.Identity())
	// if err != nil {
	// 	return nil, nil, fmt.Errorf("failed to sign node: %w", err)
	// }

	err := performSecureAction(
		false,
		prvKey,
		abac.ActionModify,
		core.NodeID(""),
		c.treeCrdt.ABACPolicy,
		secureAction,
	)
	if err != nil {
		return nil, err
	}

	// TODO: Set nonce and signature properly
	// newNode.Nonce = nonce
	// newNode.Signature = signature

	return &AdapterSecureNodeCRDT{nodeCrdt: newNode}, nil
}

func (c *AdapterSecureTreeCRDT) GetNode(id core.NodeID) (SecureNode, bool) {
	node, ok := c.treeCrdt.GetNode(id)
	if !ok {
		return nil, false
	}
	return &AdapterSecureNodeCRDT{nodeCrdt: node}, true
}

func (c *AdapterSecureTreeCRDT) GetSibling(parentNodeID core.NodeID, index int) (SecureNode, error) {
	node, err := c.treeCrdt.GetSibling(parentNodeID, index)
	if err != nil {
		return nil, err
	}
	return &AdapterSecureNodeCRDT{nodeCrdt: node}, nil
}

func (c *AdapterSecureTreeCRDT) GetValueByPath(path string) (interface{}, error) {
	return c.treeCrdt.GetValueByPath(path)
}

func (c *AdapterSecureTreeCRDT) GetNodeByPath(path string) (SecureNode, error) {
	node, err := c.treeCrdt.GetNodeByPath(path)
	if err != nil {
		return nil, err
	}
	return &AdapterSecureNodeCRDT{nodeCrdt: node}, nil
}

func (c *AdapterSecureTreeCRDT) GetStringValueByPath(path string) (string, error) {
	return c.treeCrdt.GetStringValueByPath(path)
}

func (c *AdapterSecureTreeCRDT) AddEdge(from, to core.NodeID, label string, prvKey string) error { // Tested
	secureAction := func(clientID core.ClientID) (*NodeCRDT, error) {
		// Perform the actual edge addition
		node, ok := c.treeCrdt.GetNode(from)
		if !ok {
			return nil, fmt.Errorf("from node %s not found", from)
		}
		if err := c.treeCrdt.AddEdge(from, to, label, clientID); err != nil {
			return nil, fmt.Errorf("failed to add edge: %w", err)
		}
		return node, nil
	}

	// Write to the parent's node.Nonce and node.Signature
	err := performSecureAction(
		true,
		prvKey,
		abac.ActionModify,
		from,
		c.treeCrdt.ABACPolicy,
		secureAction,
	)
	return err
}

func (c *AdapterSecureTreeCRDT) RemoveEdge(from, to core.NodeID, prvKey string) error { // Tested
	secureAction := func(clientID core.ClientID) (*NodeCRDT, error) {
		node, ok := c.treeCrdt.GetNode(from)
		if !ok {
			return nil, fmt.Errorf("from node %s not found", from)
		}
		if err := c.treeCrdt.RemoveEdge(from, to, clientID); err != nil {
			return nil, fmt.Errorf("failed to remove edge: %w", err)
		}
		return node, nil
	}

	err := performSecureAction(
		true,
		prvKey,
		abac.ActionModify,
		from,
		c.treeCrdt.ABACPolicy,
		secureAction,
	)
	return err
}

func (c *AdapterSecureTreeCRDT) AppendEdge(from, to core.NodeID, label string, prvKey string) error { // Tested
	secureAction := func(clientID core.ClientID) (*NodeCRDT, error) {
		node, ok := c.treeCrdt.GetNode(from)
		if !ok {
			return nil, fmt.Errorf("from node %s not found", from)
		}
		if err := c.treeCrdt.AppendEdge(from, to, label, clientID); err != nil {
			return nil, fmt.Errorf("failed to append edge: %w", err)
		}
		return node, nil
	}

	err := performSecureAction(
		true,
		prvKey,
		abac.ActionModify, // We treat appending a child as modifying the parent
		from,
		c.treeCrdt.ABACPolicy,
		secureAction,
	)
	return err
}

func (c *AdapterSecureTreeCRDT) PrependEdge(from, to core.NodeID, label string, prvKey string) error { // Tested
	secureAction := func(clientID core.ClientID) (*NodeCRDT, error) {
		node, ok := c.treeCrdt.GetNode(from)
		if !ok {
			return nil, fmt.Errorf("from node %s not found", from)
		}
		if err := c.treeCrdt.PrependEdge(from, to, label, clientID); err != nil {
			return nil, fmt.Errorf("failed to prepend edge: %w", err)
		}
		return node, nil
	}

	err := performSecureAction(
		true,
		prvKey,
		abac.ActionModify, // Modifying the parent node structure
		from,
		c.treeCrdt.ABACPolicy,
		secureAction,
	)
	return err
}

func (c *AdapterSecureTreeCRDT) InsertEdgeLeft(from, to core.NodeID, label string, sibling core.NodeID, prvKey string) error { // Tested
	secureAction := func(clientID core.ClientID) (*NodeCRDT, error) {
		node, ok := c.treeCrdt.Nodes[from]
		if !ok {
			return nil, fmt.Errorf("from node %s not found", from)
		}
		if err := c.treeCrdt.InsertEdgeLeft(from, to, label, sibling, clientID); err != nil {
			return nil, fmt.Errorf("failed to insert edge left: %w", err)
		}
		return node, nil
	}

	err := performSecureAction(
		true,
		prvKey,
		abac.ActionModify,
		from,
		c.treeCrdt.ABACPolicy,
		secureAction,
	)
	return err
}

func (c *AdapterSecureTreeCRDT) InsertEdgeRight(from, to core.NodeID, label string, sibling core.NodeID, prvKey string) error {
	secureAction := func(clientID core.ClientID) (*NodeCRDT, error) {
		node, ok := c.treeCrdt.Nodes[from]
		if !ok {
			return nil, fmt.Errorf("from node %s not found", from)
		}
		if err := c.treeCrdt.InsertEdgeRight(from, to, label, sibling, clientID); err != nil {
			return nil, fmt.Errorf("failed to insert edge right: %w", err)
		}
		return node, nil
	}

	err := performSecureAction(
		true,
		prvKey,
		abac.ActionModify,
		from,
		c.treeCrdt.ABACPolicy,
		secureAction,
	)
	return err
}

func (c *AdapterSecureTreeCRDT) Merge(c2 SecureTree, prvKey string) error { // TODO: test
	c2Tree := c2.(*AdapterSecureTreeCRDT)
	return c.treeCrdt.Merge(c2Tree.treeCrdt)
}

func (c *AdapterSecureTreeCRDT) ImportJSON(rawJSON []byte, prvKey string) (core.NodeID, error) {
	identity, err := crypto.CreateIdentityFromString(prvKey)
	if err != nil {
		return "", fmt.Errorf("failed to create identity from string: %w", err)
	}
	return c.treeCrdt.SecureImportJSON(rawJSON, identity)
}

func (c *AdapterSecureTreeCRDT) ImportJSONToMap(rawJSON []byte, parentID core.NodeID, key string, prvKey string) (core.NodeID, error) {
	identity, err := crypto.CreateIdentityFromString(prvKey)
	if err != nil {
		return "", fmt.Errorf("failed to create identity from string: %w", err)
	}
	return c.treeCrdt.SecureImportJSONToMap(rawJSON, parentID, key, identity)
}

func (c *AdapterSecureTreeCRDT) ImportJSONToArray(rawJSON []byte, parentID core.NodeID, prvKey string) (core.NodeID, error) {
	identity, err := crypto.CreateIdentityFromString(prvKey)
	if err != nil {
		return "", fmt.Errorf("failed to create identity from string: %w", err)
	}
	return c.treeCrdt.SecureImportJSONToArray(rawJSON, parentID, identity)
}

func (c *AdapterSecureTreeCRDT) ExportJSON() ([]byte, error) {
	return c.treeCrdt.ExportJSON()
}

func (c *AdapterSecureTreeCRDT) Load(data []byte) error {
	return c.treeCrdt.Load(data)
}

func (c *AdapterSecureTreeCRDT) Save() ([]byte, error) {
	return c.treeCrdt.Save()
}

func (c *AdapterSecureTreeCRDT) Clone() (SecureTree, error) {
	treeCopy, err := c.treeCrdt.Clone()
	if err != nil {
		return nil, err
	}
	return &AdapterSecureTreeCRDT{treeCrdt: treeCopy}, nil
}

func (c *AdapterSecureTreeCRDT) Tidy() {
	c.treeCrdt.Tidy()
}

func (c *AdapterSecureTreeCRDT) VerifyTree() error {
	return c.treeCrdt.ValidateTree()
}