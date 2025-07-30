package crdt

import (
	"testing"

	"github.com/eislab-cps/synctree/pkg/core"
	"github.com/eislab-cps/synctree/pkg/utils"
	"github.com/stretchr/testify/assert"
)

func TestMutationSetLiteral(t *testing.T) {
	clientA := core.ClientID("clientA")
	initialJSON := []byte(`["A", "B", "B"]`)

	c1 := NewTreeCRDT()
	_, err := c1.ImportJSON(initialJSON, clientA)
	assert.Nil(t, err)

	c2, err := c1.Clone()
	assert.Nil(t, err)

	aNode, err := c1.GetNodeByPath("/0")
	assert.Nil(t, err)
	mut, err := aNode.SetLiteral("AA", clientA)
	assert.Nil(t, err)

	err = c2.ApplyMutation(mut)
	assert.Nil(t, err)

	// c1 and c2 should be equal after applying mutation

	json1, err := c1.ExportJSON()
	assert.Nil(t, err)

	json2, err := c2.ExportJSON()
	assert.Nil(t, err)

	utils.CompareJSON(t, json1, json2)
}
