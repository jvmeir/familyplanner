package server

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/jvmeir/familyplanner/internal/web"
)

func TestToRefs(t *testing.T) {
	got := toRefs([]web.ViewRef{{ID: 1, Name: "Demo"}, {ID: 2, Name: "Aftellen"}})
	require.Len(t, got, 2)
	require.Equal(t, int64(1), got[0].ID)
	require.Equal(t, "Aftellen", got[1].Name)
	require.Empty(t, toRefs(nil))
}

func TestToCellDTO_Simple(t *testing.T) {
	got := toCellDTO(web.CellVM{
		Kind: "countdown", Title: "Kerstmis", Big: "160", Sub: "nog 160 dagen", Stale: true,
	})
	require.Equal(t, "countdown", got.Kind)
	require.Equal(t, "Kerstmis", got.Title)
	require.Equal(t, "160", got.Big)
	require.True(t, got.Stale)
	require.Nil(t, got.Month)
}

func TestToCellDTO_MonthAndSchedule(t *testing.T) {
	got := toCellDTO(web.CellVM{
		Kind: "calendar",
		Month: &web.MonthVM{
			Title:    "juli",
			Weekdays: []string{"ma", "di"},
			Weeks:    [][]web.DayVM{{{Day: 1, InMonth: true, Today: true, Events: []string{"Feest"}}}},
		},
		Schedule:      []web.ScheduleDayVM{{Label: "Vandaag", Today: true, Events: []string{"Zwemmen"}}},
		ScheduleTable: true,
	})
	require.NotNil(t, got.Month)
	require.Equal(t, "juli", got.Month.Title)
	require.Len(t, got.Month.Weeks, 1)
	require.True(t, got.Month.Weeks[0][0].Today)
	require.Equal(t, "Feest", got.Month.Weeks[0][0].Events[0])
	require.Len(t, got.Schedule, 1)
	require.True(t, got.ScheduleTable)
	require.Equal(t, "Zwemmen", got.Schedule[0].Events[0])
}

func TestToLayoutDTO_Recursive(t *testing.T) {
	leafA := web.CellVM{Kind: "countdown", Title: "A"}
	leafB := web.CellVM{Kind: "clock", Title: "B"}
	vm := web.LayoutVM{
		Dir: "row",
		Children: []web.LayoutChildVM{
			{Weight: 2, Node: web.LayoutVM{Cell: &leafA}},
			{Weight: 1, Node: web.LayoutVM{Cell: &leafB}},
		},
	}
	got := toLayoutDTO(vm)
	require.Equal(t, "row", got.Dir)
	require.Nil(t, got.Cell)
	require.Len(t, got.Children, 2)
	require.Equal(t, float64(2), got.Children[0].Weight)
	require.NotNil(t, got.Children[0].Node.Cell)
	require.Equal(t, "A", got.Children[0].Node.Cell.Title)
	require.Equal(t, "clock", got.Children[1].Node.Cell.Kind)
}
