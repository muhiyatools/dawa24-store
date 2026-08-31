package catalog

import "sort"

// overridePlan returns a copy of plan with the admin's bindings applied.
func overridePlan(plan ColumnPlan, columns map[string]int) ColumnPlan {
	out := ColumnPlan{
		Columns:    make(map[string]int, len(plan.Columns)),
		Unmapped:   plan.Unmapped,
		Positional: plan.Positional,
	}
	for field, col := range plan.Columns {
		out.Columns[field] = col
	}

	for field, oneBased := range columns {
		if oneBased == IgnoreColumn {
			delete(out.Columns, field)
			continue
		}
		if oneBased > 0 {
			out.Columns[field] = oneBased - 1
		}
	}

	// Bindings drive the report and the per-field labels, so they are rebuilt
	// from the resolved columns rather than left describing the old mapping.
	for _, b := range plan.Bindings {
		if col, ok := out.Columns[b.Field]; ok && col == b.Index {
			out.Bindings = append(out.Bindings, b)
		}
	}
	for field, col := range out.Columns {
		if hasBinding(out.Bindings, field) {
			continue
		}
		out.Bindings = append(out.Bindings, ColumnBinding{
			Field: field,
			Label: FieldLabels[field],
			Index: col,
			// An admin's own binding is certain by definition; it is not a guess
			// the report should ask them to double-check.
			Score:  100,
			Header: FieldLabels[field],
		})
	}
	sort.Slice(out.Bindings, func(i, j int) bool { return out.Bindings[i].Index < out.Bindings[j].Index })
	return out
}

func hasBinding(bindings []ColumnBinding, field string) bool {
	for _, b := range bindings {
		if b.Field == field {
			return true
		}
	}
	return false
}
