package commands

import "github.com/iqbalmu/dbm/internal/state"

const globalGroupLabel = "global"

// groupLabel renders an empty group as the global label.
func groupLabel(group string) string {
	if group == "" {
		return globalGroupLabel
	}
	return group
}

// filterByGroup keeps databases belonging to a group; the special value
// "global" matches ungrouped databases. An empty filter keeps everything.
func filterByGroup(dbs []*state.Database, group string) []*state.Database {
	if group == "" {
		return dbs
	}

	var out []*state.Database
	for _, d := range dbs {
		if group == globalGroupLabel {
			if d.Group == "" {
				out = append(out, d)
			}
			continue
		}
		if d.Group == group {
			out = append(out, d)
		}
	}
	return out
}
