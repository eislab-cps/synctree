package crdt

import (
	"fmt"
	"sort"

	"github.com/eislab-cps/synctree/pkg/core"
	log "github.com/sirupsen/logrus"
)

func lowestClientID(a, b core.ClientID) core.ClientID {
	if a < b {
		return a
	}
	return b
}

func normalizeNumber(v interface{}) interface{} {
	switch n := v.(type) {
	case int:
		return float64(n)
	case int64:
		return float64(n)
	default:
		return v
	}
}

func setNodeTypeFlags(node *NodeCRDT, nodeType core.NodeType) {
	switch nodeType {
	case Root:
		node.IsRoot = true
	case Map:
		node.IsMap = true
	case Array:
		node.IsArray = true
	case Literal:
		node.IsLiteral = true
	default:
		log.WithField("NodeType", nodeType).Error("Unknown node type, defaulting to literal")
		node.IsLiteral = true
	}
}

func buildOpString(opName string, args ...interface{}) string {
	if len(args) == 0 {
		return opName + "()"
	}

	str := opName + "("
	for i, arg := range args {
		if i > 0 {
			str += ", "
		}
		str += fmt.Sprintf("%v", arg)
	}
	str += ")"
	return str
}

func sortEdgesByLSEQ(edges []*EdgeCRDT) {
	sort.SliceStable(edges, func(i, j int) bool {
		p1 := edges[i].LSEQPosition
		p2 := edges[j].LSEQPosition

		// Lexicographic comparison
		for k := 0; k < len(p1) && k < len(p2); k++ {
			if p1[k] < p2[k] {
				return true
			}
			if p1[k] > p2[k] {
				return false
			}
		}

		// If one is prefix of the other, shorter one is smaller
		if len(p1) != len(p2) {
			return len(p1) < len(p2)
		}

		// Tie-breaker: use Node To ID to guarantee deterministic sort
		return edges[i].To < edges[j].To
	})
}
