package gojaeventloop

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/joeycumines/goja"
)

// console.time/timeEnd/timeLog API

// SetConsoleOutput sets the writer for JavaScript console output.
// This is useful for testing or redirecting console methods. Defaults to
// [os.Stderr]. Setting to nil disables console output while timers and counters
// still track internally. Native Goja promise-job enqueue/execution failures and
// unhandled adapter-owned host callback failures are sent synchronously through
// the event loop's configured logger via `eventloop.Loop.Log`; a nil or disabled
// logger emits nothing. Those diagnostics are not controlled by this writer.
// SetConsoleOutput panics when called on a nil, zero, or copied Adapter.
func (a *Adapter) SetConsoleOutput(w io.Writer) {
	a.mustOriginalReceiver("SetConsoleOutput")
	a.consoleOutputMu.Lock()
	a.consoleOutput = w
	a.consoleOutputMu.Unlock()
}

func (a *Adapter) consoleWriter() io.Writer {
	a.consoleOutputMu.RLock()
	output := a.consoleOutput
	a.consoleOutputMu.RUnlock()
	return output
}

// bindConsole creates console.time/timeEnd/timeLog bindings in Goja global scope.
// This creates or extends the console object with timer methods.
func (a *Adapter) bindConsole(consoleObj *goja.Object) error {
	if consoleObj == nil {
		return fmt.Errorf("goja-eventloop: console object is unavailable")
	}

	// console.time(label) - starts a timer with given label
	// If label is omitted, "default" is used.
	// If a timer with the same label already exists, a warning is logged.
	if err := defineWebMethod(a.runtime, consoleObj, "time", 0, true, func(call goja.FunctionCall) goja.Value {
		label := "default"
		if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) {
			label = call.Argument(0).String()
		}

		a.consoleTimersMu.Lock()
		_, exists := a.consoleTimers[label]
		if !exists {
			a.consoleTimers[label] = time.Now()
		}
		a.consoleTimersMu.Unlock()

		if exists {
			// Timer already exists - log warning (matches Node.js behavior)
			if output := a.consoleWriter(); output != nil {
				fmt.Fprintf(output, "Warning: Timer '%s' already exists\n", label)
			}
			return goja.Undefined()
		}
		return goja.Undefined()
	}); err != nil {
		return fmt.Errorf("bind console.time: %w", err)
	}

	// console.timeEnd(label) - stops timer and logs duration
	// If label is omitted, "default" is used.
	// If no timer with the label exists, a warning is logged.
	// Output format matches Node.js: "label: X.XXXms"
	if err := defineWebMethod(a.runtime, consoleObj, "timeEnd", 0, true, func(call goja.FunctionCall) goja.Value {
		label := "default"
		if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) {
			label = call.Argument(0).String()
		}

		a.consoleTimersMu.Lock()
		startTime, exists := a.consoleTimers[label]
		if exists {
			delete(a.consoleTimers, label)
		}
		a.consoleTimersMu.Unlock()

		if !exists {
			// Timer doesn't exist - log warning (matches Node.js behavior)
			if output := a.consoleWriter(); output != nil {
				fmt.Fprintf(output, "Warning: Timer '%s' does not exist\n", label)
			}
			return goja.Undefined()
		}

		elapsed := time.Since(startTime)

		// Output format matches Node.js: "label: X.XXXms"
		if output := a.consoleWriter(); output != nil {
			fmt.Fprintf(output, "%s: %.3fms\n", label, float64(elapsed.Nanoseconds())/1e6)
		}

		return goja.Undefined()
	}); err != nil {
		return fmt.Errorf("bind console.timeEnd: %w", err)
	}

	// console.timeLog(label, ...data) - logs elapsed time without stopping timer
	// If label is omitted, "default" is used.
	// If no timer with the label exists, a warning is logged.
	// Additional arguments are logged after the time.
	if err := defineWebMethod(a.runtime, consoleObj, "timeLog", 0, true, func(call goja.FunctionCall) goja.Value {
		label := "default"
		if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) {
			label = call.Argument(0).String()
		}

		a.consoleTimersMu.RLock()
		startTime, exists := a.consoleTimers[label]
		a.consoleTimersMu.RUnlock()

		if !exists {
			// Timer doesn't exist - log warning (matches Node.js behavior)
			output := a.consoleWriter()
			if output != nil {
				fmt.Fprintf(output, "Warning: Timer '%s' does not exist\n", label)
			}
			return goja.Undefined()
		}

		elapsed := time.Since(startTime)

		// Build output with optional additional data
		output := a.consoleWriter()

		if output != nil {
			// Output format matches Node.js: "label: X.XXXms [data...]"
			if len(call.Arguments) > 1 {
				// Additional arguments to log
				dataStrs := make([]string, 0, len(call.Arguments)-1)
				for i := 1; i < len(call.Arguments); i++ {
					dataStrs = append(dataStrs, fmt.Sprintf("%v", call.Argument(i).Export()))
				}
				fmt.Fprintf(output, "%s: %.3fms %s\n", label, float64(elapsed.Nanoseconds())/1e6,
					strings.Join(dataStrs, " "))
			} else {
				fmt.Fprintf(output, "%s: %.3fms\n", label, float64(elapsed.Nanoseconds())/1e6)
			}
		}

		return goja.Undefined()
	}); err != nil {
		return fmt.Errorf("bind console.timeLog: %w", err)
	}

	// console.count(label) - logs count for label
	// If label is omitted, "default" is used.
	// Increments count and logs "label: count"
	if err := defineWebMethod(a.runtime, consoleObj, "count", 0, true, func(call goja.FunctionCall) goja.Value {
		label := "default"
		if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) && !goja.IsNull(call.Argument(0)) {
			label = call.Argument(0).String()
		}

		a.consoleCountersMu.Lock()
		a.consoleCounters[label]++
		count := a.consoleCounters[label]
		a.consoleCountersMu.Unlock()

		// Output format matches Node.js: "label: count"
		output := a.consoleWriter()

		if output != nil {
			fmt.Fprintf(output, "%s: %d\n", label, count)
		}

		return goja.Undefined()
	}); err != nil {
		return fmt.Errorf("bind console.count: %w", err)
	}

	// console.countReset(label) - resets counter for label
	// If label is omitted, "default" is used.
	// If no counter with the label exists, a warning is logged.
	if err := defineWebMethod(a.runtime, consoleObj, "countReset", 0, true, func(call goja.FunctionCall) goja.Value {
		label := "default"
		if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) && !goja.IsNull(call.Argument(0)) {
			label = call.Argument(0).String()
		}

		a.consoleCountersMu.Lock()
		_, exists := a.consoleCounters[label]
		if !exists {
			a.consoleCountersMu.Unlock()
			// Counter doesn't exist - log warning (matches Node.js behavior)
			output := a.consoleWriter()
			if output != nil {
				fmt.Fprintf(output, "Warning: Count for '%s' does not exist\n", label)
			}
			return goja.Undefined()
		}
		delete(a.consoleCounters, label)
		a.consoleCountersMu.Unlock()

		return goja.Undefined()
	}); err != nil {
		return fmt.Errorf("bind console.countReset: %w", err)
	}

	// console.assert(condition, ...data) - logs only when falsy
	// If the condition is falsy, logs "Assertion failed: data..."
	// If condition is truthy, does nothing.
	if err := defineWebMethod(a.runtime, consoleObj, "assert", 0, true, func(call goja.FunctionCall) goja.Value {
		// Get condition - defaults to undefined (falsy)
		condition := false
		if len(call.Arguments) > 0 {
			condition = call.Argument(0).ToBoolean()
		}

		// If condition is truthy, do nothing
		if condition {
			return goja.Undefined()
		}

		// Condition is falsy - log assertion failure
		output := a.consoleWriter()

		if output != nil {
			if len(call.Arguments) > 1 {
				// Additional data to log
				dataStrs := make([]string, 0, len(call.Arguments)-1)
				for i := 1; i < len(call.Arguments); i++ {
					dataStrs = append(dataStrs, fmt.Sprintf("%v", call.Argument(i).Export()))
				}
				fmt.Fprintf(output, "Assertion failed: %s\n", strings.Join(dataStrs, " "))
			} else {
				fmt.Fprintf(output, "Assertion failed\n")
			}
		}

		return goja.Undefined()
	}); err != nil {
		return fmt.Errorf("bind console.assert: %w", err)
	}

	// console.table() Implementation

	// console.table(data, columns?) - displays tabular data as an ASCII table
	// If data is an array, each element is a row
	// If data is an object, each property is a row
	// If columns is specified, only those columns are displayed
	if err := defineWebMethod(a.runtime, consoleObj, "table", 0, true, func(call goja.FunctionCall) goja.Value {
		output := a.consoleWriter()

		if output == nil {
			return goja.Undefined()
		}

		if len(call.Arguments) == 0 {
			return goja.Undefined()
		}

		data := call.Argument(0)
		if goja.IsNull(data) || goja.IsUndefined(data) {
			fmt.Fprintf(output, "(index)\n")
			return goja.Undefined()
		}

		// Get columns filter (optional second argument)
		var columnFilter []string
		if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) && !goja.IsNull(call.Argument(1)) {
			colArg := call.Argument(1)
			// Try to parse as array of column names
			if arr, err := a.consumeIterable(colArg); err == nil {
				for _, v := range arr {
					columnFilter = append(columnFilter, v.String())
				}
			}
		}

		// Generate table
		tableStr := a.generateConsoleTable(data, columnFilter)
		a.consoleIndentMu.RLock()
		indent := a.consoleIndent
		a.consoleIndentMu.RUnlock()
		indentStr := a.getIndentString(indent)

		// Add indent to each line
		lines := strings.SplitSeq(tableStr, "\n")
		for line := range lines {
			if line != "" {
				fmt.Fprintf(output, "%s%s\n", indentStr, line)
			}
		}

		return goja.Undefined()
	}); err != nil {
		return fmt.Errorf("bind console.table: %w", err)
	}

	// console.group/groupEnd/trace/clear/dir

	// console.group(label?) - starts a new indented group
	if err := defineWebMethod(a.runtime, consoleObj, "group", 0, true, func(call goja.FunctionCall) goja.Value {
		output := a.consoleWriter()

		// Get current indent for the label
		a.consoleIndentMu.RLock()
		indent := a.consoleIndent
		a.consoleIndentMu.RUnlock()

		// Print label if provided
		if output != nil {
			indentStr := a.getIndentString(indent)
			if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) {
				label := call.Argument(0).String()
				fmt.Fprintf(output, "%s▼ %s\n", indentStr, label)
			} else {
				fmt.Fprintf(output, "%s▼ console.group\n", indentStr)
			}
		}

		// Increase indent for subsequent calls
		a.consoleIndentMu.Lock()
		a.consoleIndent++
		a.consoleIndentMu.Unlock()

		return goja.Undefined()
	}); err != nil {
		return fmt.Errorf("bind console.group: %w", err)
	}

	// console.groupCollapsed(label?) - same as group (no collapse in terminal)
	if err := defineWebMethod(a.runtime, consoleObj, "groupCollapsed", 0, true, func(call goja.FunctionCall) goja.Value {
		output := a.consoleWriter()

		// Get current indent for the label
		a.consoleIndentMu.RLock()
		indent := a.consoleIndent
		a.consoleIndentMu.RUnlock()

		// Print label if provided (same as group, just different visual marker)
		if output != nil {
			indentStr := a.getIndentString(indent)
			if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) {
				label := call.Argument(0).String()
				fmt.Fprintf(output, "%s▶ %s\n", indentStr, label)
			} else {
				fmt.Fprintf(output, "%s▶ console.group\n", indentStr)
			}
		}

		// Increase indent for subsequent calls
		a.consoleIndentMu.Lock()
		a.consoleIndent++
		a.consoleIndentMu.Unlock()

		return goja.Undefined()
	}); err != nil {
		return fmt.Errorf("bind console.groupCollapsed: %w", err)
	}

	// console.groupEnd() - ends the current group and reduces indent
	if err := defineWebMethod(a.runtime, consoleObj, "groupEnd", 0, true, func(call goja.FunctionCall) goja.Value {
		a.consoleIndentMu.Lock()
		if a.consoleIndent > 0 {
			a.consoleIndent--
		}
		a.consoleIndentMu.Unlock()

		return goja.Undefined()
	}); err != nil {
		return fmt.Errorf("bind console.groupEnd: %w", err)
	}

	// console.trace(msg?) - prints a stack trace
	if err := defineWebMethod(a.runtime, consoleObj, "trace", 0, true, func(call goja.FunctionCall) goja.Value {
		output := a.consoleWriter()

		if output == nil {
			return goja.Undefined()
		}

		a.consoleIndentMu.RLock()
		indent := a.consoleIndent
		a.consoleIndentMu.RUnlock()
		indentStr := a.getIndentString(indent)

		// Print message if provided
		if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) {
			msg := call.Argument(0).String()
			fmt.Fprintf(output, "%sTrace: %s\n", indentStr, msg)
		} else {
			fmt.Fprintf(output, "%sTrace\n", indentStr)
		}

		// Get stack trace from Goja
		// We create an Error object to capture the stack
		stack := a.runtime.CaptureCallStack(10, nil)
		for _, frame := range stack {
			funcName := frame.FuncName()
			if funcName == "" {
				funcName = "(anonymous)"
			}
			pos := frame.Position()
			// file.Position has Filename and Line but not Col (column is part of String())
			fmt.Fprintf(output, "%s    at %s (%s:%d)\n", indentStr, funcName, pos.Filename, pos.Line)
		}

		return goja.Undefined()
	}); err != nil {
		return fmt.Errorf("bind console.trace: %w", err)
	}

	// console.clear() - clears the console (prints newlines or ANSI clear)
	if err := defineWebMethod(a.runtime, consoleObj, "clear", 0, true, func(call goja.FunctionCall) goja.Value {
		output := a.consoleWriter()

		if output == nil {
			return goja.Undefined()
		}

		// Print a few newlines to simulate clearing
		// In a terminal, we could use ANSI escape codes, but for portability we use newlines
		fmt.Fprintf(output, "\n\n\n")

		return goja.Undefined()
	}); err != nil {
		return fmt.Errorf("bind console.clear: %w", err)
	}

	// console.dir(obj, options?) - displays an interactive listing of the properties of a specified object
	if err := defineWebMethod(a.runtime, consoleObj, "dir", 0, true, func(call goja.FunctionCall) goja.Value {
		output := a.consoleWriter()

		if output == nil {
			return goja.Undefined()
		}

		a.consoleIndentMu.RLock()
		indent := a.consoleIndent
		a.consoleIndentMu.RUnlock()
		indentStr := a.getIndentString(indent)

		if len(call.Arguments) == 0 {
			fmt.Fprintf(output, "%sundefined\n", indentStr)
			return goja.Undefined()
		}

		obj := call.Argument(0)
		if goja.IsUndefined(obj) {
			fmt.Fprintf(output, "%sundefined\n", indentStr)
			return goja.Undefined()
		}
		if goja.IsNull(obj) {
			fmt.Fprintf(output, "%snull\n", indentStr)
			return goja.Undefined()
		}

		// Try to serialize as JSON for object inspection
		exported := obj.Export()
		inspection := a.inspectValue(exported, 0, 2)
		lines := strings.SplitSeq(inspection, "\n")
		for line := range lines {
			if line != "" {
				fmt.Fprintf(output, "%s%s\n", indentStr, line)
			}
		}

		return goja.Undefined()
	}); err != nil {
		return fmt.Errorf("bind console.dir: %w", err)
	}

	return nil
}

// console.table() Helpers

// getIndentString returns a string of spaces for the current indentation level.
// Each level is 2 spaces.
func (a *Adapter) getIndentString(level int) string {
	if level <= 0 {
		return ""
	}
	var result strings.Builder
	for range level {
		result.WriteString("  ")
	}
	return result.String()
}

// generateConsoleTable generates an ASCII table from the given data.
func (a *Adapter) generateConsoleTable(data goja.Value, columnFilter []string) string {
	if data == nil || goja.IsNull(data) || goja.IsUndefined(data) {
		return "(index)"
	}

	exported := data.Export()

	// Check if it's an array-like structure
	if arr, ok := exported.([]any); ok {
		return a.generateTableFromArray(arr, columnFilter)
	}

	// Check if it's a map (object)
	if obj, ok := exported.(map[string]any); ok {
		return a.generateTableFromObject(obj, columnFilter)
	}

	// For primitives, just return the value as string
	return fmt.Sprintf("%v", exported)
}

// generateTableFromArray creates a table from an array.
func (a *Adapter) generateTableFromArray(arr []any, columnFilter []string) string {
	if len(arr) == 0 {
		return "(index)"
	}

	// Collect all column names from the data
	allColumns := make(map[string]bool)
	rows := make([]map[string]string, len(arr))

	for i, item := range arr {
		rows[i] = make(map[string]string)
		rows[i]["(index)"] = fmt.Sprintf("%d", i)

		if obj, ok := item.(map[string]any); ok {
			for k, v := range obj {
				allColumns[k] = true
				rows[i][k] = a.formatCellValue(v)
			}
		} else {
			// For non-object items, use "Values" as column name
			allColumns["Values"] = true
			rows[i]["Values"] = a.formatCellValue(item)
		}
	}

	// Build ordered column list
	columns := []string{"(index)"}
	if columnFilter != nil {
		// Use specified columns only if they exist
		for _, c := range columnFilter {
			if allColumns[c] {
				columns = append(columns, c)
			}
		}
	} else {
		// Use all collected columns in sorted order
		sortedCols := make([]string, 0, len(allColumns))
		for k := range allColumns {
			sortedCols = append(sortedCols, k)
		}
		// Simple sort for deterministic output
		for i := 0; i < len(sortedCols); i++ {
			for j := i + 1; j < len(sortedCols); j++ {
				if sortedCols[i] > sortedCols[j] {
					sortedCols[i], sortedCols[j] = sortedCols[j], sortedCols[i]
				}
			}
		}
		columns = append(columns, sortedCols...)
	}

	return a.renderTable(columns, rows)
}

// generateTableFromObject creates a table from an object.
func (a *Adapter) generateTableFromObject(obj map[string]any, columnFilter []string) string {
	if len(obj) == 0 {
		return "(index)"
	}

	// Collect all column names from nested objects
	allColumns := make(map[string]bool)
	rows := make([]map[string]string, 0, len(obj))

	// Sort keys for deterministic output
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[i] > keys[j] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}

	for _, k := range keys {
		v := obj[k]
		row := make(map[string]string)
		row["(index)"] = k

		if nested, ok := v.(map[string]any); ok {
			for nk, nv := range nested {
				allColumns[nk] = true
				row[nk] = a.formatCellValue(nv)
			}
		} else {
			// For non-object values, use "Values" as column name
			allColumns["Values"] = true
			row["Values"] = a.formatCellValue(v)
		}
		rows = append(rows, row)
	}

	// Build ordered column list
	columns := []string{"(index)"}
	if columnFilter != nil {
		// Use specified columns only if they exist
		for _, c := range columnFilter {
			if allColumns[c] {
				columns = append(columns, c)
			}
		}
	} else {
		// Use all collected columns in sorted order
		sortedCols := make([]string, 0, len(allColumns))
		for k := range allColumns {
			sortedCols = append(sortedCols, k)
		}
		for i := 0; i < len(sortedCols); i++ {
			for j := i + 1; j < len(sortedCols); j++ {
				if sortedCols[i] > sortedCols[j] {
					sortedCols[i], sortedCols[j] = sortedCols[j], sortedCols[i]
				}
			}
		}
		columns = append(columns, sortedCols...)
	}

	return a.renderTable(columns, rows)
}

// formatCellValue formats a value for display in a table cell.
func (a *Adapter) formatCellValue(v any) string {
	if v == nil {
		return "null"
	}

	switch val := v.(type) {
	case []any:
		return "Array(" + fmt.Sprintf("%d", len(val)) + ")"
	case map[string]any:
		return "Object"
	case string:
		return val
	case bool:
		return fmt.Sprintf("%v", val)
	case float64:
		// Format numbers nicely (no trailing zeros for integers)
		if val == float64(int64(val)) {
			return fmt.Sprintf("%d", int64(val))
		}
		return fmt.Sprintf("%g", val)
	case int64:
		return fmt.Sprintf("%d", val)
	case int:
		return fmt.Sprintf("%d", val)
	default:
		return fmt.Sprintf("%v", val)
	}
}

// renderTable renders the table as ASCII.
func (a *Adapter) renderTable(columns []string, rows []map[string]string) string {
	if len(columns) == 0 {
		return ""
	}

	// Calculate column widths
	widths := make([]int, len(columns))
	for i, col := range columns {
		widths[i] = len(col)
	}
	for _, row := range rows {
		for i, col := range columns {
			cellLen := len(row[col])
			if cellLen > widths[i] {
				widths[i] = cellLen
			}
		}
	}

	var result strings.Builder

	// Build separator line
	separatorParts := make([]string, len(columns))
	for i, w := range widths {
		separatorParts[i] = strings.Repeat("─", w+2)
	}
	separator := "├" + strings.Join(separatorParts, "┼") + "┤"

	// Build top border
	topParts := make([]string, len(columns))
	for i, w := range widths {
		topParts[i] = strings.Repeat("─", w+2)
	}
	topBorder := "┌" + strings.Join(topParts, "┬") + "┐"

	// Build bottom border
	bottomParts := make([]string, len(columns))
	for i, w := range widths {
		bottomParts[i] = strings.Repeat("─", w+2)
	}
	bottomBorder := "└" + strings.Join(bottomParts, "┴") + "┘"

	// Top border
	result.WriteString(topBorder)
	result.WriteString("\n")

	// Header row
	headerParts := make([]string, len(columns))
	for i, col := range columns {
		headerParts[i] = a.padRight(col, widths[i])
	}
	result.WriteString("│ ")
	result.WriteString(strings.Join(headerParts, " │ "))
	result.WriteString(" │\n")

	// Separator
	result.WriteString(separator)
	result.WriteString("\n")

	// Data rows
	for _, row := range rows {
		cellParts := make([]string, len(columns))
		for i, col := range columns {
			cellParts[i] = a.padRight(row[col], widths[i])
		}
		result.WriteString("│ ")
		result.WriteString(strings.Join(cellParts, " │ "))
		result.WriteString(" │\n")
	}

	// Bottom border
	result.WriteString(bottomBorder)

	return result.String()
}

// padRight pads a string to the right with spaces.
func (a *Adapter) padRight(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}

// console.dir() Helper

// inspectValue creates a human-readable representation of a value.
// maxDepth controls how deep to inspect nested objects.
func (a *Adapter) inspectValue(v any, depth int, maxDepth int) string {
	if v == nil {
		return "null"
	}

	indent := strings.Repeat("  ", depth)
	nextIndent := strings.Repeat("  ", depth+1)

	switch val := v.(type) {
	case []any:
		if depth >= maxDepth {
			return fmt.Sprintf("Array(%d)", len(val))
		}
		if len(val) == 0 {
			return "[]"
		}
		var parts []string
		for i, item := range val {
			parts = append(parts, fmt.Sprintf("%s%d: %s", nextIndent, i, a.inspectValue(item, depth+1, maxDepth)))
		}
		return "[\n" + strings.Join(parts, ",\n") + "\n" + indent + "]"

	case map[string]any:
		if depth >= maxDepth {
			return "Object"
		}
		if len(val) == 0 {
			return "{}"
		}
		// Sort keys for deterministic output
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		for i := 0; i < len(keys); i++ {
			for j := i + 1; j < len(keys); j++ {
				if keys[i] > keys[j] {
					keys[i], keys[j] = keys[j], keys[i]
				}
			}
		}
		var parts []string
		for _, k := range keys {
			parts = append(parts, fmt.Sprintf("%s%s: %s", nextIndent, k, a.inspectValue(val[k], depth+1, maxDepth)))
		}
		return "{\n" + strings.Join(parts, ",\n") + "\n" + indent + "}"

	case string:
		return fmt.Sprintf("'%s'", val)

	case bool:
		return fmt.Sprintf("%v", val)

	case float64:
		// Format numbers nicely (no trailing zeros for integers)
		if val == float64(int64(val)) {
			return fmt.Sprintf("%d", int64(val))
		}
		return fmt.Sprintf("%g", val)

	case int64:
		return fmt.Sprintf("%d", val)

	case int:
		return fmt.Sprintf("%d", val)

	default:
		return fmt.Sprintf("%v", val)
	}
}
