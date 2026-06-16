package server

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/jvmeir/familyplanner/internal/db/dbgen"
	"github.com/jvmeir/familyplanner/internal/layout"
	"github.com/jvmeir/familyplanner/internal/web"
)

func parsePath(s string) []int {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ".")
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil
		}
		out = append(out, n)
	}
	return out
}

func (s *Server) loadLayout(view dbgen.View) layout.Node {
	if view.LayoutJson != "" {
		if n, err := layout.Parse(view.LayoutJson); err == nil {
			return n
		}
	}
	return layout.SingleLeaf(0) // start as a single empty pane
}

func (s *Server) saveLayout(ctx context.Context, id int64, n layout.Node) error {
	js, err := n.Marshal()
	if err != nil {
		return err
	}
	return s.store.SetViewLayout(ctx, dbgen.SetViewLayoutParams{LayoutJson: js, ID: id})
}

func (s *Server) handleViewLayout(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	view, err := s.store.GetView(r.Context(), id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	root := s.loadLayout(view)
	s.render(w, r, web.ViewLayoutPage(id, view.Name, s.buildEditorVM(r.Context(), root, ""), s.widgetVMs(r.Context())))
}

func (s *Server) handleLayoutSplit(w http.ResponseWriter, r *http.Request) {
	s.mutateLayout(w, r, func(root *layout.Node) {
		_ = root.SplitLeaf(parsePath(r.URL.Query().Get("path")), layout.Direction(r.URL.Query().Get("dir")))
	})
}

func (s *Server) handleLayoutRemove(w http.ResponseWriter, r *http.Request) {
	s.mutateLayout(w, r, func(root *layout.Node) {
		_ = root.Remove(parsePath(r.URL.Query().Get("path")))
	})
}

func (s *Server) handleLayoutSetWidget(w http.ResponseWriter, r *http.Request) {
	s.mutateLayout(w, r, func(root *layout.Node) {
		widgetID, _ := strconv.ParseInt(r.FormValue("widget"), 10, 64)
		_ = root.SetWidget(parsePath(r.URL.Query().Get("path")), widgetID)
	})
}

// mutateLayout loads the view's tree, applies fn, validates+persists, and returns
// the re-rendered editor pane (innerHTML swap of #editor-canvas).
func (s *Server) mutateLayout(w http.ResponseWriter, r *http.Request, fn func(*layout.Node)) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	view, err := s.store.GetView(r.Context(), id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	root := s.loadLayout(view)
	fn(&root)
	if err := root.Validate(); err != nil {
		http.Error(w, "invalid layout", http.StatusBadRequest)
		return
	}
	if err := s.saveLayout(r.Context(), id, root); err != nil {
		http.Error(w, "save failed", http.StatusInternalServerError)
		return
	}
	s.render(w, r, web.EditorPane(id, s.buildEditorVM(r.Context(), root, ""), s.widgetVMs(r.Context())))
}

// handleLayoutWeights applies dragged divider weights to a split's children.
func (s *Server) handleLayoutWeights(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	view, err := s.store.GetView(r.Context(), id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	root := s.loadLayout(view)
	parent := parsePath(r.FormValue("path"))
	for i, ws := range strings.Split(r.FormValue("weights"), ",") {
		wgt, perr := strconv.ParseFloat(strings.TrimSpace(ws), 64)
		if perr != nil || wgt <= 0 {
			continue
		}
		childPath := append(append([]int{}, parent...), i)
		_ = root.SetWeight(childPath, wgt)
	}
	if err := s.saveLayout(r.Context(), id, root); err != nil {
		http.Error(w, "save failed", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) buildEditorVM(ctx context.Context, node layout.Node, path string) web.EditorNodeVM {
	if node.Leaf != nil {
		name := ""
		if node.Leaf.WidgetID != 0 {
			if wd, err := s.store.GetWidget(ctx, node.Leaf.WidgetID); err == nil {
				name = wd.Name
			}
		}
		return web.EditorNodeVM{Path: path, IsLeaf: true, WidgetID: node.Leaf.WidgetID, WidgetName: name}
	}
	vm := web.EditorNodeVM{Path: path, Dir: string(node.Split.Dir)}
	for i, c := range node.Split.Children {
		childPath := strconv.Itoa(i)
		if path != "" {
			childPath = path + "." + childPath
		}
		vm.Children = append(vm.Children, web.EditorChildVM{
			Weight: c.Weight,
			Node:   s.buildEditorVM(ctx, c.Node, childPath),
		})
	}
	return vm
}
