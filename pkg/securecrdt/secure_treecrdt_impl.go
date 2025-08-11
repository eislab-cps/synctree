package securecrdt

import (
	"fmt"

	"github.com/eislab-cps/synctree/internal/crypto"
	"github.com/eislab-cps/synctree/pkg/abac"
	"github.com/eislab-cps/synctree/pkg/core"
	"github.com/eislab-cps/synctree/pkg/crdt"
	log "github.com/sirupsen/logrus"
)

type SecureNodeCRDTImpl struct {
	nodeCrdt *crdt.NodeCRDT
}

func performSecureAction(
	accessControl bool,
	prvKey string,
	action abac.ABACAction,
	target core.NodeID,
	abac *abac.ABACPolicy,
	actionFn func(core.ClientID) (crdt.Mutation, *crdt.NodeCRDT, error),
) (crdt.Mutation, error) {
	identity, err := crypto.CreateIdentityFromString(prvKey)
	if err != nil {
		return crdt.Mutation{}, fmt.Errorf("failed to create identity: %w", err)
	}

	id := identity.ID()

	if accessControl {
		if abac != nil && !abac.IsAllowed(id, action, target) {
			log.WithFields(log.Fields{
				"ID":     id,
				"Action": action,
				"Target": target,
			}).Error("Not allowed to perform action on target")
			return crdt.Mutation{}, fmt.Errorf("identity %s not allowed to perform %s on %s", id, action, target)
		}
	}

	mut, node, err := actionFn(core.ClientID(id))
	if err != nil {
		return crdt.Mutation{}, err
	}

	err = node.Sign(identity)
	if err != nil {
		return crdt.Mutation{}, fmt.Errorf("failed to sign node: %w", err)
	}

	// TODO: This is only required if the parent node was modified.
	if node.ParentID != "" {
		parentNode, ok := node.Tree().GetNode(node.ParentID)
		if !ok {
			return crdt.Mutation{}, fmt.Errorf("parent node %s not found for node %s", node.ParentID, node.ID)
		}

		// Sign the parent node with the same identity
		if err := parentNode.Sign(identity); err != nil {
			return crdt.Mutation{}, fmt.Errorf("failed to sign parent node: %w", err)
		}
	}

	return mut, nil
}

func (n *SecureNodeCRDTImpl) ID() core.NodeID {
	return n.nodeCrdt.ID
}

func (n *SecureNodeCRDTImpl) SetLiteral(value interface{}, prvKey string) (crdt.Mutation, error) {
	secureAction := func(clientID core.ClientID) (crdt.Mutation, *crdt.NodeCRDT, error) {
		mut, err := n.nodeCrdt.SetLiteral(value, clientID)
		if err != nil {
			return crdt.Mutation{}, nil, fmt.Errorf("failed to set literal: %w", err)
		}
		return mut, n.nodeCrdt, nil
	}

	accessControl := true
	if n.nodeCrdt.ParentID == "" {
		accessControl = false // If the node is not attached to a tree, we skip ABAC checks
	}

	return performSecureAction(
		accessControl,
		prvKey,
		abac.ActionModify,
		n.nodeCrdt.ID,
		n.nodeCrdt.Tree().ABACPolicy,
		secureAction)
}

func (n *SecureNodeCRDTImpl) GetLiteral() (interface{}, error) {
	return n.nodeCrdt.GetLiteral()
}

func (n *SecureNodeCRDTImpl) CreateMapNode(prvKey string) (crdt.Mutation, SecureNodeCRDT, error) {
	var newNode *crdt.NodeCRDT

	secureAction := func(clientID core.ClientID) (crdt.Mutation, *crdt.NodeCRDT, error) {
		node, err := n.nodeCrdt.CreateMapNode(clientID)
		if err != nil {
			return crdt.Mutation{}, nil, fmt.Errorf("failed to create map node: %w", err)
		}
		newNode = node
		return crdt.Mutation{}, newNode, nil
	}

	delta, err := performSecureAction(
		true,
		prvKey,
		abac.ActionModify,
		n.nodeCrdt.ID,
		n.nodeCrdt.Tree().ABACPolicy,
		secureAction)
	if err != nil {
		return crdt.Mutation{}, nil, err
	}

	return delta, &SecureNodeCRDTImpl{nodeCrdt: newNode}, nil
}

func (n *SecureNodeCRDTImpl) SetKeyValue(key string, value interface{}, prvKey string) (crdt.Mutation, core.NodeID, error) {
	var newNodeID core.NodeID

	secureAction := func(clientID core.ClientID) (crdt.Mutation, *crdt.NodeCRDT, error) {
		_, id, err := n.nodeCrdt.SetKeyValue(key, value, clientID)
		if err != nil {
			return crdt.Mutation{}, nil, fmt.Errorf("failed to set key-value: %w", err)
		}
		newNodeID = id
		newNode, ok := n.nodeCrdt.Tree().GetNode(newNodeID)
		if !ok {
			return crdt.Mutation{}, nil, fmt.Errorf("new node %s not found in tree after setting key-value", newNodeID)
		}
		return crdt.Mutation{}, newNode, nil
	}

	mut, err := performSecureAction(
		true,
		prvKey,
		abac.ActionModify,
		n.nodeCrdt.ID,
		n.nodeCrdt.Tree().ABACPolicy,
		secureAction)
	if err != nil {
		return mut, "", err
	}

	return mut, newNodeID, nil
}

func (n *SecureNodeCRDTImpl) GetNodeForKey(key string) (SecureNodeCRDT, bool, error) {
	internalNode, ok, err := n.nodeCrdt.GetNodeForKey(key)
	if err != nil || !ok {
		return nil, ok, err
	}
	return &SecureNodeCRDTImpl{nodeCrdt: internalNode}, ok, nil
}

func (n *SecureNodeCRDTImpl) RemoveKeyValue(key string, prvKey string) (crdt.Mutation, error) {
	secureAction := func(clientID core.ClientID) (crdt.Mutation, *crdt.NodeCRDT, error) {
		if err := n.nodeCrdt.RemoveKeyValue(key, clientID); err != nil {
			return crdt.Mutation{}, nil, fmt.Errorf("failed to remove key-value: %w", err)
		}
		return crdt.Mutation{}, n.nodeCrdt, nil
	}

	return performSecureAction(
		true,
		prvKey,
		abac.ActionModify,
		n.nodeCrdt.ID,
		n.nodeCrdt.Tree().ABACPolicy,
		secureAction,
	)
}

type SecureTreeCRDTImpl struct {
	treeCrdt *crdt.TreeCRDT
}

func NewSecureTreeCRDT(prvKey string) (SecureTreeCRDT, error) {
	identity, err := crypto.CreateIdentityFromString(prvKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create identity from string: %w", err)
	}

	c := crdt.NewTreeCRDT()
	ownerID := identity.ID()
	c.ABACPolicy = abac.NewABACPolicy(c, ownerID, identity)
	c.ABACPolicy.Allow(ownerID, "*", "root", true) // Allow the owner to have full access to whole tree
	c.Secure = true

	return &SecureTreeCRDTImpl{
		treeCrdt: c,
	}, nil
}

func (c *SecureTreeCRDTImpl) ABAC() *abac.ABACPolicy {
	return c.treeCrdt.ABACPolicy
}

func (c *SecureTreeCRDTImpl) CreateAttachedNode(name string, nodeType core.NodeType, parentID core.NodeID, prvKey string) (crdt.Mutation, SecureNodeCRDT, error) {
	var newNode *crdt.NodeCRDT

	secureAction := func(clientID core.ClientID) (crdt.Mutation, *crdt.NodeCRDT, error) {
		node := c.treeCrdt.CreateAttachedNode(name, nodeType, parentID, clientID)
		newNode = node
		return crdt.Mutation{}, newNode, nil
	}

	mut, err := performSecureAction(
		true,
		prvKey,
		abac.ActionModify,
		parentID,
		c.treeCrdt.ABACPolicy,
		secureAction,
	)
	if err != nil {
		return crdt.Mutation{}, nil, err
	}

	return mut, &SecureNodeCRDTImpl{nodeCrdt: newNode}, nil
}

func (c *SecureTreeCRDTImpl) CreateNode(name string, nodeType core.NodeType, prvKey string) (crdt.Mutation, SecureNodeCRDT, error) {
	var newNode *crdt.NodeCRDT

	secureAction := func(clientID core.ClientID) (crdt.Mutation, *crdt.NodeCRDT, error) {
		node := c.treeCrdt.CreateNode(name, nodeType, clientID)
		newNode = node
		return crdt.Mutation{}, newNode, nil
	}

	var nonce, signature string
	delta, err := performSecureAction(
		false, // Check ABAC policy since this node is not attached to the tree yet
		prvKey,
		abac.ActionModify,
		c.treeCrdt.Root.ID, // Treat as adding under root
		c.treeCrdt.ABACPolicy,
		secureAction,
	)
	if err != nil {
		return crdt.Mutation{}, nil, err
	}

	newNode.Nonce = nonce
	newNode.Signature = signature

	return delta, &SecureNodeCRDTImpl{nodeCrdt: newNode}, nil
}

func (c *SecureTreeCRDTImpl) GetNode(id core.NodeID) (SecureNodeCRDT, bool) {
	node, ok := c.treeCrdt.GetNode(id)
	if !ok {
		return nil, false
	}
	return &SecureNodeCRDTImpl{nodeCrdt: node}, true
}

func (c *SecureTreeCRDTImpl) GetSibling(parentNodeID core.NodeID, index int) (SecureNodeCRDT, error) {
	node, err := c.treeCrdt.GetSibling(parentNodeID, index)
	if err != nil {
		return nil, err
	}
	return &SecureNodeCRDTImpl{nodeCrdt: node}, nil
}

func (c *SecureTreeCRDTImpl) GetValueByPath(path string) (interface{}, error) {
	return c.treeCrdt.GetValueByPath(path)
}

func (c *SecureTreeCRDTImpl) GetNodeByPath(path string) (SecureNodeCRDT, error) {
	node, err := c.treeCrdt.GetNodeByPath(path)
	if err != nil {
		return nil, err
	}
	return &SecureNodeCRDTImpl{nodeCrdt: node}, nil
}

func (c *SecureTreeCRDTImpl) GetStringValueByPath(path string) (string, error) {
	return c.treeCrdt.GetStringValueByPath(path)
}

func (c *SecureTreeCRDTImpl) AddEdge(from, to core.NodeID, label string, prvKey string) (crdt.Mutation, error) {
	secureAction := func(clientID core.ClientID) (crdt.Mutation, *crdt.NodeCRDT, error) {
		node, ok := c.treeCrdt.GetNode(from)
		if !ok {
			return crdt.Mutation{}, nil, fmt.Errorf("parent node %s not found", from)
		}

		err := c.treeCrdt.AddEdge(from, to, label, clientID)
		if err != nil {
			return crdt.Mutation{}, nil, fmt.Errorf("failed to add edge from %s to %s: %w", from, to, err)
		}
		return crdt.Mutation{}, node, nil
	}

	return performSecureAction(
		true,
		prvKey,
		abac.ActionModify,
		from,
		c.treeCrdt.ABACPolicy,
		secureAction,
	)
}

func (c *SecureTreeCRDTImpl) RemoveEdge(from, to core.NodeID, prvKey string) (crdt.Mutation, error) {
	secureAction := func(clientID core.ClientID) (crdt.Mutation, *crdt.NodeCRDT, error) {
		node, ok := c.treeCrdt.GetNode(from)
		if !ok {
			return crdt.Mutation{}, nil, fmt.Errorf("parent node %s not found", from)
		}
		err := c.treeCrdt.RemoveEdge(from, to, clientID)
		if err != nil {
			return crdt.Mutation{}, nil, fmt.Errorf("failed to remove edge from %s to %s: %w", from, to, err)
		}
		return crdt.Mutation{}, node, nil
	}

	return performSecureAction(
		true,
		prvKey,
		abac.ActionModify,
		from,
		c.treeCrdt.ABACPolicy,
		secureAction,
	)
}

func (c *SecureTreeCRDTImpl) AppendEdge(from, to core.NodeID, label string, prvKey string) (crdt.Mutation, error) {
	secureAction := func(clientID core.ClientID) (crdt.Mutation, *crdt.NodeCRDT, error) {
		node, ok := c.treeCrdt.GetNode(from)
		if !ok {
			return crdt.Mutation{}, nil, fmt.Errorf("parent node %s not found", from)
		}
		err := c.treeCrdt.AppendEdge(from, to, label, clientID)
		if err != nil {
			return crdt.Mutation{}, nil, fmt.Errorf("failed to append edge from %s to %s: %w", from, to, err)
		}
		return crdt.Mutation{}, node, nil
	}

	return performSecureAction(
		true,
		prvKey,
		abac.ActionModify,
		from,
		c.treeCrdt.ABACPolicy,
		secureAction,
	)
}

func (c *SecureTreeCRDTImpl) PrependEdge(from, to core.NodeID, label string, prvKey string) (crdt.Mutation, error) {
	secureAction := func(clientID core.ClientID) (crdt.Mutation, *crdt.NodeCRDT, error) {
		node, ok := c.treeCrdt.GetNode(from)
		if !ok {
			return crdt.Mutation{}, nil, fmt.Errorf("parent node %s not found", from)
		}
		err := c.treeCrdt.PrependEdge(from, to, label, clientID)
		if err != nil {
			return crdt.Mutation{}, nil, fmt.Errorf("failed to prepend edge from %s to %s: %w", from, to, err)
		}
		return crdt.Mutation{}, node, nil
	}

	return performSecureAction(
		true,
		prvKey,
		abac.ActionModify,
		from,
		c.treeCrdt.ABACPolicy,
		secureAction,
	)
}

func (c *SecureTreeCRDTImpl) InsertEdgeLeft(from, to core.NodeID, label string, sibling core.NodeID, prvKey string) (crdt.Mutation, error) {
	secureAction := func(clientID core.ClientID) (crdt.Mutation, *crdt.NodeCRDT, error) {
		node, ok := c.treeCrdt.Nodes[from]
		if !ok {
			return crdt.Mutation{}, nil, fmt.Errorf("parent node %s not found", from)
		}
		err := c.treeCrdt.InsertEdgeLeft(from, to, label, sibling, clientID)
		if err != nil {
			return crdt.Mutation{}, nil, fmt.Errorf("failed to insert edge left from %s to %s: %w", from, to, err)
		}
		return crdt.Mutation{}, node, nil
	}

	return performSecureAction(
		true,
		prvKey,
		abac.ActionModify,
		from,
		c.treeCrdt.ABACPolicy,
		secureAction,
	)
}

func (c *SecureTreeCRDTImpl) InsertEdgeRight(from, to core.NodeID, label string, sibling core.NodeID, prvKey string) (crdt.Mutation, error) {
	secureAction := func(clientID core.ClientID) (crdt.Mutation, *crdt.NodeCRDT, error) {
		node, ok := c.treeCrdt.Nodes[from]
		if !ok {
			return crdt.Mutation{}, nil, fmt.Errorf("parent node %s not found", from)
		}
		err := c.treeCrdt.InsertEdgeRight(from, to, label, sibling, clientID)
		if err != nil {
			return crdt.Mutation{}, nil, fmt.Errorf("failed to insert edge right from %s to %s: %w", from, to, err)
		}
		return crdt.Mutation{}, node, nil
	}

	return performSecureAction(
		true,
		prvKey,
		abac.ActionModify,
		from,
		c.treeCrdt.ABACPolicy,
		secureAction,
	)
}

func (c *SecureTreeCRDTImpl) Merge(c2 SecureTreeCRDT, prvKey string) error {
	adapter, ok := c2.(*SecureTreeCRDTImpl)
	if !ok {
		panic("Merge: Tree must be of type *AdapterTreeCRDT")
	}
	return c.treeCrdt.SecureMerge(adapter.treeCrdt, prvKey)
}

// ApplyMutation(mut crdt.Mutation, prvKey string) error
func (c *SecureTreeCRDTImpl) ApplyMutation(mut crdt.Mutation, prvKey string) error {
	// TODO:

	// Check if the identity is allowed to apply this mutation

	return nil
}

func (c *SecureTreeCRDTImpl) ImportJSON(rawJSON []byte, prvKey string) (core.NodeID, error) {
	identity, err := crypto.CreateIdentityFromString(prvKey)
	if err != nil {
		return "", fmt.Errorf("failed to create identity from string: %w", err)
	}

	id := identity.ID()

	if !c.treeCrdt.ABACPolicy.IsAllowed(id, abac.ActionModify, c.treeCrdt.Root.ID) {
		return "", fmt.Errorf("identity %s is not allowed to import under root", id)
	}

	return c.treeCrdt.SecureImportJSON(rawJSON, identity)
}

func (c *SecureTreeCRDTImpl) ImportJSONToMap(rawJSON []byte, parentID core.NodeID, key string, prvKey string) (core.NodeID, error) {
	identity, err := crypto.CreateIdentityFromString(prvKey)
	if err != nil {
		return "", fmt.Errorf("failed to create identity from string: %w", err)
	}

	id := identity.ID()

	if !c.treeCrdt.ABACPolicy.IsAllowed(id, abac.ActionModify, parentID) {
		return "", fmt.Errorf("identity %s is not allowed to import under parent %s", id, parentID)
	}

	return c.treeCrdt.SecureImportJSONToMap(rawJSON, parentID, key, identity)
}

func (c *SecureTreeCRDTImpl) ImportJSONToArray(rawJSON []byte, parentID core.NodeID, prvKey string) (core.NodeID, error) {
	identity, err := crypto.CreateIdentityFromString(prvKey)
	if err != nil {
		return "", fmt.Errorf("failed to create identity from string: %w", err)
	}

	id := identity.ID()

	if !c.treeCrdt.ABACPolicy.IsAllowed(id, abac.ActionModify, parentID) {
		return "", fmt.Errorf("identity %s is not allowed to import under parent %s", id, parentID)
	}

	return c.treeCrdt.SecureImportJSONToArray(rawJSON, parentID, identity)
}

func (c *SecureTreeCRDTImpl) Clone() (SecureTreeCRDT, error) {
	newTree, err := c.treeCrdt.Clone()
	if err != nil {
		return nil, fmt.Errorf("failed to clone tree: %w", err)
	}
	if newTree == nil {
		return nil, fmt.Errorf("failed to clone tree")
	}
	newTree.ABACPolicy, err = newTree.ABACPolicy.Clone()
	if err != nil {
		return nil, fmt.Errorf("failed to clone ABAC policy: %w", err)
	}
	newTree.ABACPolicy.SetTreeChecker(newTree)
	newTree.ABACPolicy.SetIdentity(c.treeCrdt.ABACPolicy.Identity())
	return &SecureTreeCRDTImpl{treeCrdt: newTree}, nil
}

func (c *SecureTreeCRDTImpl) ExportJSON() ([]byte, error) {
	return c.treeCrdt.ExportJSON()
}

func (c *SecureTreeCRDTImpl) Load(data []byte) error {
	identity := c.treeCrdt.ABACPolicy.Identity()
	err := c.treeCrdt.Load(data)
	if err != nil {
		return fmt.Errorf("failed to load tree data: %w", err)
	}
	c.treeCrdt.ABACPolicy.SetTreeChecker(c.treeCrdt)
	c.treeCrdt.ABACPolicy.SetIdentity(identity)
	recoveredID, err := c.treeCrdt.ABACPolicy.Verify()
	if err != nil {
		log.WithFields(log.Fields{
			"Identity":    identity.ID(),
			"Action":      "Load",
			"Owner":       c.treeCrdt.ABACPolicy.OwnerID,
			"RecoveredID": recoveredID,
			"Error":       err,
		}).Error("Failed to verify ABAC policy after loading, recovered ID does not match owner ID")

		return fmt.Errorf("failed to verify ABAC policy after loading: %w", err)
	}

	return nil
}

func (c *SecureTreeCRDTImpl) Save() ([]byte, error) {
	return c.treeCrdt.Save()
}

func (c *SecureTreeCRDTImpl) Subscribe(path string, ch chan crdt.NodeEvent) {
	c.treeCrdt.Subscribe(path, ch)
}

func (c *SecureTreeCRDTImpl) Tidy() {
	c.treeCrdt.Tidy()
}

func (c *SecureTreeCRDTImpl) VerifyTree() error {
	return c.treeCrdt.VerifyTree()
}
