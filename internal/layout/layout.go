// Package layout models a view's arrangement as a recursive split tree.
//
// A view starts as a single pane (a leaf). A leaf can be split horizontally
// ("row") or vertically ("col") into weighted children, recursively; panes can
// be removed (merging their space back). Leaves reference a widget (WidgetID 0 =
// an empty, not-yet-assigned pane). The tree is stored as JSON in views.layout_json
// and rendered with nested flexbox.
package layout

import (
	"encoding/json"
	"errors"
)

// Direction of a split.
type Direction string

const (
	Row Direction = "row" // children side by side
	Col Direction = "col" // children stacked
)

// Node is either a Split or a Leaf (exactly one is non-nil).
type Node struct {
	Split *Split `json:"split,omitempty"`
	Leaf  *Leaf  `json:"leaf,omitempty"`
}

// Split divides space among weighted children.
type Split struct {
	Dir      Direction `json:"dir"`
	Children []Child   `json:"children"`
}

// Child is a weighted sub-node.
type Child struct {
	Weight float64 `json:"weight"`
	Node   Node    `json:"node"`
}

// Leaf holds a widget (0 = empty pane) plus optional per-placement overrides.
type Leaf struct {
	WidgetID int64           `json:"widget_id"`
	Config   json.RawMessage `json:"config,omitempty"`
}

// SingleLeaf returns the default layout: one pane holding widgetID (0 = empty).
func SingleLeaf(widgetID int64) Node {
	return Node{Leaf: &Leaf{WidgetID: widgetID}}
}

// Parse decodes and validates a layout tree from JSON.
func Parse(s string) (Node, error) {
	var n Node
	if err := json.Unmarshal([]byte(s), &n); err != nil {
		return Node{}, err
	}
	if err := n.Validate(); err != nil {
		return Node{}, err
	}
	return n, nil
}

// Marshal encodes the tree to JSON.
func (n Node) Marshal() (string, error) {
	b, err := json.Marshal(n)
	return string(b), err
}

// Validate checks the tree is well-formed.
func (n Node) Validate() error {
	switch {
	case n.Leaf != nil && n.Split != nil:
		return errors.New("layout: node has both leaf and split")
	case n.Leaf == nil && n.Split == nil:
		return errors.New("layout: empty node")
	case n.Split != nil:
		if n.Split.Dir != Row && n.Split.Dir != Col {
			return errors.New("layout: invalid direction")
		}
		if len(n.Split.Children) < 2 {
			return errors.New("layout: split needs at least two children")
		}
		for _, c := range n.Split.Children {
			if err := c.Node.Validate(); err != nil {
				return err
			}
		}
	}
	return nil
}

// at returns a pointer to the node addressed by path (child indices from root),
// or nil if the path is invalid.
func (n *Node) at(path []int) *Node {
	cur := n
	for _, idx := range path {
		if cur.Split == nil || idx < 0 || idx >= len(cur.Split.Children) {
			return nil
		}
		cur = &cur.Split.Children[idx].Node
	}
	return cur
}

// SplitLeaf splits the leaf at path into two equal panes along dir. The existing
// content stays in the first pane; the second is a new empty pane.
func (n *Node) SplitLeaf(path []int, dir Direction) error {
	if dir != Row && dir != Col {
		return errors.New("layout: invalid direction")
	}
	target := n.at(path)
	if target == nil {
		return errors.New("layout: path not found")
	}
	if target.Leaf == nil {
		return errors.New("layout: can only split a leaf")
	}
	old := *target
	*target = Node{Split: &Split{Dir: dir, Children: []Child{
		{Weight: 1, Node: old},
		{Weight: 1, Node: Node{Leaf: &Leaf{}}},
	}}}
	return nil
}

// Remove deletes the child addressed by path from its parent split, collapsing
// the parent into its remaining child when only one is left (the "merge").
func (n *Node) Remove(path []int) error {
	if len(path) == 0 {
		return errors.New("layout: cannot remove the root")
	}
	parent := n.at(path[:len(path)-1])
	idx := path[len(path)-1]
	if parent == nil || parent.Split == nil {
		return errors.New("layout: parent is not a split")
	}
	ch := parent.Split.Children
	if idx < 0 || idx >= len(ch) {
		return errors.New("layout: bad child index")
	}
	parent.Split.Children = append(ch[:idx:idx], ch[idx+1:]...)
	if len(parent.Split.Children) == 1 {
		*parent = parent.Split.Children[0].Node // collapse the split away
	}
	return nil
}

// SetWidget assigns a widget to the leaf at path (0 clears it).
func (n *Node) SetWidget(path []int, widgetID int64) error {
	t := n.at(path)
	if t == nil || t.Leaf == nil {
		return errors.New("layout: path is not a leaf")
	}
	t.Leaf.WidgetID = widgetID
	return nil
}

// SetWeight sets the weight of the child addressed by path.
func (n *Node) SetWeight(path []int, weight float64) error {
	if len(path) == 0 {
		return errors.New("layout: root has no weight")
	}
	if weight <= 0 {
		return errors.New("layout: weight must be positive")
	}
	parent := n.at(path[:len(path)-1])
	idx := path[len(path)-1]
	if parent == nil || parent.Split == nil || idx < 0 || idx >= len(parent.Split.Children) {
		return errors.New("layout: bad path")
	}
	parent.Split.Children[idx].Weight = weight
	return nil
}
