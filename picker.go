package search

// CatalogName is one row from the host's organization or group list.
// The host supplies the name. This package does not look it up.
type CatalogName struct {
	ID   string
	Name string
}

// PickerRow is one visible picker row. Grey means the row is checked, the
// other side has at least one check, and this id is not relevant. The host
// keeps that check until the user clears it.
type PickerRow struct {
	ID   string
	Name string
	Grey bool
}

// ReducePicker builds the organization list and the group list.
//
// orgs and groups are the catalogs after the host's text filter, in the
// order to display. checkedOrgs and checkedGroups are the full check sets,
// including ids the text filter hid. A hidden check still narrows the other
// side and is omitted from the returned rows. groupsByOrg maps each visible
// organization to the group ids GroupIDs returned for it.
//
// A checked id that is not in the catalog argument is omitted here. This
// function does not change the check sets. The host drops a check that is
// outside the unfiltered catalog.
//
// Neither side checked: both lists are the filtered catalogs, and nothing is
// grey. Checking organizations whittles groups to the union of groupsByOrg
// for those organizations. Checking groups whittles organizations to those
// whose set intersects the checked groups. The two reductions are independent.
func ReducePicker(orgs, groups []CatalogName, checkedOrgs, checkedGroups []string, groupsByOrg map[string][]string) (orgRows, groupRows []PickerRow) {
	orgChecked := idSet(checkedOrgs)
	groupChecked := idSet(checkedGroups)

	referenced := map[string]struct{}{}
	for id := range orgChecked {
		for _, g := range groupsByOrg[id] {
			if g == "" {
				continue
			}
			referenced[g] = struct{}{}
		}
	}
	orgsForGroups := map[string]struct{}{}
	if groupChecked != nil {
		for orgID, gs := range groupsByOrg {
			for _, g := range gs {
				if _, ok := groupChecked[g]; ok {
					orgsForGroups[orgID] = struct{}{}
					break
				}
			}
		}
	}

	orgRows = pickerRows(orgs, orgChecked, groupChecked != nil, orgsForGroups)
	groupRows = pickerRows(groups, groupChecked, orgChecked != nil, referenced)
	return orgRows, groupRows
}

// pickerRows keeps catalog order. relevant is the set allowed by the other
// side. When the other side has no checks, every catalog row is relevant.
func pickerRows(catalog []CatalogName, checked map[string]struct{}, otherChecked bool, relevant map[string]struct{}) []PickerRow {
	out := make([]PickerRow, 0, len(catalog))
	for _, row := range catalog {
		_, isChecked := checked[row.ID]
		isRelevant := !otherChecked
		if otherChecked {
			_, isRelevant = relevant[row.ID]
		}
		if !isRelevant && !isChecked {
			continue
		}
		out = append(out, PickerRow{
			ID:   row.ID,
			Name: row.Name,
			Grey: isChecked && otherChecked && !isRelevant,
		})
	}
	return out
}
