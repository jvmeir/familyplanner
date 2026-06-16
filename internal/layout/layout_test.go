package layout_test

import (
	"testing"

	"github.com/jvmeir/familyplanner/internal/layout"
	"github.com/stretchr/testify/require"
)

func TestSingleLeafRoundTrip(t *testing.T) {
	n := layout.SingleLeaf(7)
	s, err := n.Marshal()
	require.NoError(t, err)

	got, err := layout.Parse(s)
	require.NoError(t, err)
	require.NotNil(t, got.Leaf)
	require.Equal(t, int64(7), got.Leaf.WidgetID)
}

func TestValidate(t *testing.T) {
	_, err := layout.Parse(`{}`)
	require.Error(t, err, "empty node is invalid")

	_, err = layout.Parse(`{"split":{"dir":"row","children":[{"weight":1,"node":{"leaf":{"widget_id":1}}}]}}`)
	require.Error(t, err, "split needs >= 2 children")

	_, err = layout.Parse(`{"split":{"dir":"diagonal","children":[{"weight":1,"node":{"leaf":{}}},{"weight":1,"node":{"leaf":{}}}]}}`)
	require.Error(t, err, "bad direction")
}

func TestSplitLeaf(t *testing.T) {
	n := layout.SingleLeaf(1)
	require.NoError(t, n.SplitLeaf(nil, layout.Row))

	require.NotNil(t, n.Split)
	require.Equal(t, layout.Row, n.Split.Dir)
	require.Len(t, n.Split.Children, 2)
	require.Equal(t, int64(1), n.Split.Children[0].Node.Leaf.WidgetID, "original content stays in first pane")
	require.Equal(t, int64(0), n.Split.Children[1].Node.Leaf.WidgetID, "second pane is empty")

	// split a nested leaf
	require.NoError(t, n.SplitLeaf([]int{1}, layout.Col))
	require.NotNil(t, n.Split.Children[1].Node.Split)

	// cannot split a split
	require.Error(t, n.SplitLeaf(nil, layout.Row))
}

func TestRemoveCollapses(t *testing.T) {
	n := layout.SingleLeaf(1)
	require.NoError(t, n.SplitLeaf(nil, layout.Row)) // -> split of [leaf1, empty]
	require.NoError(t, n.SetWidget([]int{1}, 2))     // assign widget 2 to the empty pane

	// remove the second pane -> parent collapses back to a single leaf (widget 1)
	require.NoError(t, n.Remove([]int{1}))
	require.Nil(t, n.Split)
	require.NotNil(t, n.Leaf)
	require.Equal(t, int64(1), n.Leaf.WidgetID)

	require.Error(t, n.Remove(nil), "cannot remove root")
}

func TestSetWeight(t *testing.T) {
	n := layout.SingleLeaf(1)
	require.NoError(t, n.SplitLeaf(nil, layout.Row))
	require.NoError(t, n.SetWeight([]int{0}, 3))
	require.Equal(t, float64(3), n.Split.Children[0].Weight)
	require.Error(t, n.SetWeight([]int{0}, 0), "weight must be positive")
}
