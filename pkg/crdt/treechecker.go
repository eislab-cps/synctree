package crdt

import "github.com/eislab-cps/synctree/pkg/core"

func (t *TreeCRDT) IsDescendant(root core.NodeID, target core.NodeID) bool {
	if root == target {
		return true
	}
	visited := make(map[core.NodeID]bool)
	var dfs func(core.NodeID) bool
	dfs = func(n core.NodeID) bool {
		if visited[n] {
			return false
		}
		visited[n] = true
		node, ok := t.Nodes[n]
		if !ok {
			return false
		}
		for _, edge := range node.Edges {
			if edge.To == target || dfs(edge.To) {
				return true
			}
		}
		return false
	}
	return dfs(root)
}
