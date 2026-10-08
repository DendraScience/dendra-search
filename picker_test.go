package search

import "testing"

func TestReducePicker(t *testing.T) {
	orgs := []CatalogName{{ID: "north", Name: "North Reserve"}, {ID: "south", Name: "South Reserve"}}
	groups := []CatalogName{
		{ID: "field", Name: "Field crew"},
		{ID: "lab", Name: "Lab"},
		{ID: "volunteers", Name: "Volunteers"},
		{ID: "internal", Name: "Internal"},
	}
	byOrg := map[string][]string{
		"north": {"field", "lab", "volunteers"},
		"south": {"lab"},
	}
	cases := []struct {
		name   string
		orgs   []string
		groups []string
		left   string
		right  string
	}{
		{name: "none", left: "north,south", right: "field,lab,volunteers,internal"},
		{name: "both orgs", orgs: []string{"north", "south"}, left: "north,south", right: "field,lab,volunteers"},
		{name: "volunteers", groups: []string{"volunteers"}, left: "north", right: "field,lab,volunteers,internal"},
		{name: "both orgs and volunteers", orgs: []string{"north", "south"}, groups: []string{"volunteers"}, left: "north,south~", right: "field,lab,volunteers"},
		{name: "internal and north", orgs: []string{"north"}, groups: []string{"internal"}, left: "north~", right: "field,lab,volunteers,internal~"},
	}
	for _, tc := range cases {
		orgRows, groupRows := ReducePicker(orgs, groups, tc.orgs, tc.groups, byOrg)
		if got := formatRows(orgRows); got != tc.left {
			t.Fatalf("%s orgs %s", tc.name, got)
		}
		if got := formatRows(groupRows); got != tc.right {
			t.Fatalf("%s groups %s", tc.name, got)
		}
	}

	// A check the text filter hid stays in the check set and still narrows
	// the other side. The row itself is absent until the catalog includes it.
	hidden, groupRows := ReducePicker(orgs[:1], groups, []string{"north", "south"}, []string{"volunteers"}, byOrg)
	if formatRows(hidden) != "north" {
		t.Fatalf("hidden south %s", formatRows(hidden))
	}
	if formatRows(groupRows) != "field,lab,volunteers" {
		t.Fatalf("hidden south still narrows groups: %s", formatRows(groupRows))
	}
}

func formatRows(rows []PickerRow) string {
	if len(rows) == 0 {
		return ""
	}
	out := rows[0].ID
	if rows[0].Grey {
		out += "~"
	}
	for _, row := range rows[1:] {
		out += "," + row.ID
		if row.Grey {
			out += "~"
		}
	}
	return out
}
