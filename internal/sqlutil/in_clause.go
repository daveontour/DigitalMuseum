package sqlutil

import (
	"fmt"
	"strings"
)

// Int64IN returns "column IN (?start,...)" with one SQLite ?NNN placeholder per id, or "FALSE" when ids is empty.
func Int64IN(column string, ids []int64, start int) (cond string, args []any, next int) {
	if len(ids) == 0 {
		return "FALSE", nil, start
	}
	args = make([]any, len(ids))
	ph := make([]string, len(ids))
	for i, id := range ids {
		ph[i] = fmt.Sprintf("?%d", start+i)
		args[i] = id
	}
	return fmt.Sprintf("%s IN (%s)", column, strings.Join(ph, ",")), args, start + len(ids)
}

// StringIN returns "column IN (?start,...)" with one SQLite ?NNN placeholder per value, or "FALSE" when vals is empty.
func StringIN(column string, vals []string, start int) (cond string, args []any, next int) {
	if len(vals) == 0 {
		return "FALSE", nil, start
	}
	args = make([]any, len(vals))
	ph := make([]string, len(vals))
	for i, v := range vals {
		ph[i] = fmt.Sprintf("?%d", start+i)
		args[i] = v
	}
	return fmt.Sprintf("%s IN (%s)", column, strings.Join(ph, ",")), args, start + len(vals)
}
