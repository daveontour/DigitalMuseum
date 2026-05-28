package sqlutil

import "strings"

// OrderByAscNullsLast returns a SQLite-safe ORDER BY expression that sorts expr
// ascending with NULL values last. Optional tieBreakers append as ", tie" clauses.
func OrderByAscNullsLast(expr string, tieBreakers ...string) string {
	return orderByNullsLast(expr, "ASC", tieBreakers)
}

// OrderByDescNullsLast returns a SQLite-safe ORDER BY expression that sorts expr
// descending with NULL values last. Optional tieBreakers append as ", tie" clauses.
func OrderByDescNullsLast(expr string, tieBreakers ...string) string {
	return orderByNullsLast(expr, "DESC", tieBreakers)
}

func orderByNullsLast(expr, direction string, tieBreakers []string) string {
	parts := []string{expr + " IS NULL", expr + " " + direction}
	for _, t := range tieBreakers {
		t = strings.TrimSpace(t)
		if t != "" {
			parts = append(parts, t)
		}
	}
	return strings.Join(parts, ", ")
}
