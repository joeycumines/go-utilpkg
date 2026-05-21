package tournament

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

func TestTimerReferenceConsideredStrategyASTRejectsDrift(t *testing.T) {
	owner := `package reference
func (l *Loop) refViaIsLoopThread(id TimerID, ref bool) error {
	if l.isLoopThread() { l.applyTimerRefChange(id, ref); return nil }
	return l.SubmitInternal(func() { l.applyTimerRefChange(id, ref) })
}`
	always := `package reference
func (l *Loop) refViaSubmitInternal(id TimerID, ref bool) error {
	return l.SubmitInternal(func() { l.applyTimerRefChange(id, ref) })
}`
	syncMap := `package reference
func (l *Loop) refViaSyncMap(id TimerID, ref bool) {
	val, ok := timerSyncMap.Load(uint64(id)); if !ok { return }; t := val.(*timer)
	old := t.refed.Swap(ref); if old != ref { if ref { l.refedTimerCount.Add(1) } else { l.refedTimerCount.Add(-1) } }
}`
	rwMutex := `package reference
func (l *Loop) refViaRWMutex(id TimerID, ref bool) {
	timerRefMu.RLock(); t, ok := l.timerMap[id]; timerRefMu.RUnlock(); if !ok { return }
	old := t.refed.Swap(ref); if old != ref { if ref { l.refedTimerCount.Add(1) } else { l.refedTimerCount.Add(-1) } }
}`
	tests := []struct {
		name     string
		source   string
		function string
		mutate   func(string) string
		validate func([]byte, string) error
	}{
		{"owner predicate", owner, "refViaIsLoopThread", func(value string) string { return strings.Replace(value, "isLoopThread", "isExternal", 1) }, validateTimerReferenceOwnerSourceBytes},
		{"owner direct branch", owner, "refViaIsLoopThread", func(value string) string {
			return strings.Replace(value, "l.applyTimerRefChange(id, ref); return nil", "return nil", 1)
		}, validateTimerReferenceOwnerSourceBytes},
		{"owner closure arguments", owner, "refViaIsLoopThread", func(value string) string {
			return strings.Replace(value, "l.applyTimerRefChange(id, ref) })", "l.applyTimerRefChange(id, !ref) })", 1)
		}, validateTimerReferenceOwnerSourceBytes},
		{"always conditional", always, "refViaSubmitInternal", func(value string) string {
			return strings.Replace(value, "return l.SubmitInternal(func() { l.applyTimerRefChange(id, ref) })", "if ref { return l.SubmitInternal(func() { l.applyTimerRefChange(id, ref) }) }; return nil", 1)
		}, validateTimerReferenceAlwaysSourceBytes},
		{"always direct", always, "refViaSubmitInternal", func(value string) string {
			return strings.Replace(value, "return l.SubmitInternal(func() { l.applyTimerRefChange(id, ref) })", "l.applyTimerRefChange(id, ref); return nil", 1)
		}, validateTimerReferenceAlwaysSourceBytes},
		{"sync key conversion", syncMap, "refViaSyncMap", func(value string) string { return strings.Replace(value, "uint64(id)", "id", 1) }, validateTimerReferenceSyncSourceBytes},
		{"sync assertion", syncMap, "refViaSyncMap", func(value string) string { return strings.Replace(value, "val.(*timer)", "val", 1) }, validateTimerReferenceSyncSourceBytes},
		{"sync store", syncMap, "refViaSyncMap", func(value string) string { return strings.Replace(value, ".Swap(ref)", ".Store(ref)", 1) }, validateTimerReferenceSyncSourceBytes},
		{"sync delta", syncMap, "refViaSyncMap", func(value string) string { return strings.Replace(value, ".Add(1)", ".Add(2)", 1) }, validateTimerReferenceSyncSourceBytes},
		{"rw write lock", rwMutex, "refViaRWMutex", func(value string) string { return strings.Replace(value, ".RLock()", ".Lock()", 1) }, validateTimerReferenceRWSourceBytes},
		{"rw missing unlock", rwMutex, "refViaRWMutex", func(value string) string { return strings.Replace(value, "timerRefMu.RUnlock();", "", 1) }, validateTimerReferenceRWSourceBytes},
		{"rw late unlock", rwMutex, "refViaRWMutex", func(value string) string {
			return strings.Replace(value, "timerRefMu.RUnlock(); if !ok { return }\n\told := t.refed.Swap(ref);", "if !ok { return }\n\told := t.refed.Swap(ref); timerRefMu.RUnlock();", 1)
		}, validateTimerReferenceRWSourceBytes},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mutated := []byte(test.mutate(test.source))
			if _, err := parser.ParseFile(token.NewFileSet(), test.function+".go", mutated, 0); err != nil {
				t.Fatalf("hostile mutation must remain valid Go: %v", err)
			}
			if err := test.validate(mutated, test.function); err == nil {
				t.Fatal("accepted strategy semantic drift")
			}
		})
	}
}

func validateTimerReferenceOwnerSourceBytes(source []byte, function string) error {
	declaration, err := timerReferenceStrategyMethod(source, function)
	if err != nil {
		return err
	}
	return validateTimerReferenceOwnerSource(declaration)
}

func validateTimerReferenceAlwaysSourceBytes(source []byte, function string) error {
	declaration, err := timerReferenceStrategyMethod(source, function)
	if err != nil {
		return err
	}
	return validateTimerReferenceAlwaysSource(declaration)
}

func validateTimerReferenceSyncSourceBytes(source []byte, function string) error {
	return validateTimerReferenceSyncCore(source, function, "", "timerSyncMap", "l", "refedTimerCount", "timer")
}

func validateTimerReferenceRWSourceBytes(source []byte, function string) error {
	return validateTimerReferenceRWCore(source, function, "", "timerRefMu", "l", "timerMap", "refedTimerCount")
}

func validateTimerReferenceOwnerSource(declaration *ast.FuncDecl) error {
	if err := validateTimerReferenceStrategySignature(declaration, true); err != nil {
		return err
	}
	statements := declaration.Body.List
	if len(statements) != 2 {
		return fmt.Errorf("owner source statement count = %d, want 2", len(statements))
	}
	branch, ok := statements[0].(*ast.IfStmt)
	if !ok || branch.Else != nil || !timerReferenceSimpleCall(branch.Cond, "l", "isLoopThread") || len(branch.Body.List) != 2 {
		return fmt.Errorf("owner source direct branch differs")
	}
	if !timerReferenceCoreCall(branch.Body.List[0], "l", "applyTimerRefChange", "id", "ref") || !timerReferenceNilReturn(branch.Body.List[1]) {
		return fmt.Errorf("owner source direct application differs")
	}
	if !timerReferenceSubmittedCore(statements[1], "l", "SubmitInternal", "applyTimerRefChange", "id", "ref") {
		return fmt.Errorf("owner source external submission differs")
	}
	return nil
}

func validateTimerReferenceAlwaysSource(declaration *ast.FuncDecl) error {
	if err := validateTimerReferenceStrategySignature(declaration, true); err != nil {
		return err
	}
	if len(declaration.Body.List) != 1 || !timerReferenceSubmittedCore(declaration.Body.List[0], "l", "SubmitInternal", "applyTimerRefChange", "id", "ref") {
		return fmt.Errorf("always-submit source differs")
	}
	return nil
}

func validateTimerReferenceSyncCore(source []byte, function, lookupReceiver, lookupField, aggregateReceiver, aggregateField, assertedType string) error {
	declaration, err := timerReferenceStrategyMethod(source, function)
	if err != nil {
		return err
	}
	if err := validateTimerReferenceStrategySignature(declaration, false); err != nil {
		return err
	}
	statements := declaration.Body.List
	if len(statements) != 5 {
		return fmt.Errorf("sync.Map core statement count = %d, want 5", len(statements))
	}
	lookup, ok := statements[0].(*ast.AssignStmt)
	if !ok || lookup.Tok != token.DEFINE || len(lookup.Lhs) != 2 || len(lookup.Rhs) != 1 || !timerReferenceSyncLoad(lookup.Rhs[0], lookupReceiver, lookupField, "id") {
		return fmt.Errorf("sync.Map lookup differs")
	}
	loaded, loadedOK := lookup.Lhs[0].(*ast.Ident)
	found, foundOK := lookup.Lhs[1].(*ast.Ident)
	if !loadedOK || !foundOK || !timerReferenceMissingReturn(statements[1], found.Name) {
		return fmt.Errorf("sync.Map missing-ID branch differs")
	}
	assertion, ok := statements[2].(*ast.AssignStmt)
	if !ok || assertion.Tok != token.DEFINE || len(assertion.Lhs) != 1 || len(assertion.Rhs) != 1 {
		return fmt.Errorf("sync.Map concrete assertion assignment differs")
	}
	value, valueOK := assertion.Lhs[0].(*ast.Ident)
	typeAssertion, assertionOK := assertion.Rhs[0].(*ast.TypeAssertExpr)
	pointer, pointerOK := typeAssertionType(typeAssertion)
	if !valueOK || !assertionOK || !timerReferenceIdentifier(typeAssertion.X, loaded.Name) || !pointerOK || pointer != assertedType {
		return fmt.Errorf("sync.Map concrete assertion differs")
	}
	reference := declaration.Type.Params.List[1].Names[0].Name
	return validateTimerReferenceChangedCore(statements[3:], value.Name, "refed", reference, aggregateReceiver, aggregateField)
}

func validateTimerReferenceRWCore(source []byte, function, lockReceiver, lockField, mapReceiver, mapField, aggregateField string) error {
	declaration, err := timerReferenceStrategyMethod(source, function)
	if err != nil {
		return err
	}
	if err := validateTimerReferenceStrategySignature(declaration, false); err != nil {
		return err
	}
	statements := declaration.Body.List
	if len(statements) != 6 {
		return fmt.Errorf("RWMutex core statement count = %d, want 6", len(statements))
	}
	if !timerReferenceLockCall(statements[0], lockReceiver, lockField, "RLock") || !timerReferenceLockCall(statements[2], lockReceiver, lockField, "RUnlock") {
		return fmt.Errorf("RWMutex lock window differs")
	}
	lookup, ok := statements[1].(*ast.AssignStmt)
	if !ok || lookup.Tok != token.DEFINE || len(lookup.Lhs) != 2 || len(lookup.Rhs) != 1 {
		return fmt.Errorf("RWMutex map lookup assignment differs")
	}
	value, valueOK := lookup.Lhs[0].(*ast.Ident)
	found, foundOK := lookup.Lhs[1].(*ast.Ident)
	index, indexOK := lookup.Rhs[0].(*ast.IndexExpr)
	if !valueOK || !foundOK || !indexOK || !timerReferenceOwnedSelector(index.X, mapReceiver, mapField) || !timerReferenceIdentifier(index.Index, "id") || !timerReferenceMissingReturn(statements[3], found.Name) {
		return fmt.Errorf("RWMutex map lookup or missing-ID branch differs")
	}
	reference := declaration.Type.Params.List[1].Names[0].Name
	return validateTimerReferenceChangedCore(statements[4:], value.Name, "refed", reference, mapReceiver, aggregateField)
}

func validateTimerReferenceChangedCore(statements []ast.Stmt, value, field, reference, aggregateReceiver, aggregateField string) error {
	if len(statements) != 2 {
		return fmt.Errorf("reference mutation statement count = %d, want 2", len(statements))
	}
	swap, ok := statements[0].(*ast.AssignStmt)
	if !ok || swap.Tok != token.DEFINE || len(swap.Lhs) != 1 || len(swap.Rhs) != 1 {
		return fmt.Errorf("reference swap assignment differs")
	}
	old, oldOK := swap.Lhs[0].(*ast.Ident)
	if !oldOK || !timerReferenceCall(swap.Rhs[0], value, field, "Swap", reference) {
		return fmt.Errorf("reference bit is not atomically swapped")
	}
	changed, ok := statements[1].(*ast.IfStmt)
	if !ok || changed.Else != nil || !timerReferenceNotEqual(changed.Cond, old.Name, reference) || len(changed.Body.List) != 1 {
		return fmt.Errorf("changed-bit branch differs")
	}
	delta, ok := changed.Body.List[0].(*ast.IfStmt)
	if !ok || !timerReferenceIdentifier(delta.Cond, reference) || len(delta.Body.List) != 1 {
		return fmt.Errorf("aggregate branch differs")
	}
	otherwise, ok := delta.Else.(*ast.BlockStmt)
	if !ok || len(otherwise.List) != 1 {
		return fmt.Errorf("unref aggregate branch differs")
	}
	up, upOK := timerReferenceAdd(delta.Body.List[0], aggregateReceiver, aggregateField)
	down, downOK := timerReferenceAdd(otherwise.List[0], aggregateReceiver, aggregateField)
	if !upOK || !downOK || up != 1 || down != -1 {
		return fmt.Errorf("aggregate deltas = (%d, %d), want (1, -1)", up, down)
	}
	return nil
}

func timerReferenceStrategyMethod(source []byte, function string) (*ast.FuncDecl, error) {
	file, err := parser.ParseFile(token.NewFileSet(), function+".go", source, 0)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", function, err)
	}
	var declaration *ast.FuncDecl
	for _, candidate := range file.Decls {
		value, ok := candidate.(*ast.FuncDecl)
		if !ok || value.Name.Name != function || value.Recv == nil {
			continue
		}
		if declaration != nil {
			return nil, fmt.Errorf("duplicate receiver method %s", function)
		}
		declaration = value
	}
	if declaration == nil || declaration.Body == nil {
		return nil, fmt.Errorf("missing receiver method %s", function)
	}
	return declaration, nil
}

func validateTimerReferenceStrategySignature(declaration *ast.FuncDecl, errorResult bool) error {
	if declaration.Recv == nil || len(declaration.Recv.List) != 1 || len(declaration.Recv.List[0].Names) != 1 {
		return fmt.Errorf("%s receiver differs", declaration.Name.Name)
	}
	parameters := declaration.Type.Params.List
	if len(parameters) != 2 || len(parameters[0].Names) != 1 || parameters[0].Names[0].Name != "id" || len(parameters[1].Names) != 1 || (parameters[1].Names[0].Name != "ref" && parameters[1].Names[0].Name != "refed") {
		return fmt.Errorf("%s parameters differ", declaration.Name.Name)
	}
	resultCount := 0
	if declaration.Type.Results != nil {
		resultCount = len(declaration.Type.Results.List)
	}
	if errorResult {
		if resultCount != 1 || !timerReferenceIdentifier(declaration.Type.Results.List[0].Type, "error") {
			return fmt.Errorf("%s result differs", declaration.Name.Name)
		}
	} else if resultCount != 0 {
		return fmt.Errorf("%s has unexpected result", declaration.Name.Name)
	}
	return nil
}

func timerReferenceSimpleCall(expression ast.Expr, receiver, method string) bool {
	call, ok := expression.(*ast.CallExpr)
	if !ok || len(call.Args) != 0 {
		return false
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	return ok && selector.Sel.Name == method && timerReferenceIdentifier(selector.X, receiver)
}

func timerReferenceCoreCall(statement ast.Stmt, receiver, method, id, reference string) bool {
	expression, ok := statement.(*ast.ExprStmt)
	if !ok {
		return false
	}
	call, ok := expression.X.(*ast.CallExpr)
	if !ok || len(call.Args) != 2 || !timerReferenceIdentifier(call.Args[0], id) || !timerReferenceIdentifier(call.Args[1], reference) {
		return false
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	return ok && selector.Sel.Name == method && timerReferenceIdentifier(selector.X, receiver)
}

func timerReferenceSubmittedCore(statement ast.Stmt, receiver, submit, core, id, reference string) bool {
	result, ok := statement.(*ast.ReturnStmt)
	if !ok || len(result.Results) != 1 {
		return false
	}
	call, ok := result.Results[0].(*ast.CallExpr)
	if !ok || len(call.Args) != 1 {
		return false
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	closure, closureOK := call.Args[0].(*ast.FuncLit)
	return ok && selector.Sel.Name == submit && timerReferenceIdentifier(selector.X, receiver) && closureOK && closure.Type.Params.NumFields() == 0 && len(closure.Body.List) == 1 && timerReferenceCoreCall(closure.Body.List[0], receiver, core, id, reference)
}

func timerReferenceLockCall(statement ast.Stmt, receiver, field, method string) bool {
	expression, ok := statement.(*ast.ExprStmt)
	if !ok {
		return false
	}
	call, ok := expression.X.(*ast.CallExpr)
	if !ok || len(call.Args) != 0 {
		return false
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != method {
		return false
	}
	if receiver == "" {
		return timerReferenceIdentifier(selector.X, field)
	}
	return timerReferenceOwnedSelector(selector.X, receiver, field)
}

func timerReferenceSyncLoad(expression ast.Expr, receiver, field, id string) bool {
	call, ok := expression.(*ast.CallExpr)
	if !ok || len(call.Args) != 1 {
		return false
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "Load" {
		return false
	}
	if receiver == "" {
		if !timerReferenceIdentifier(selector.X, field) {
			return false
		}
	} else if !timerReferenceOwnedSelector(selector.X, receiver, field) {
		return false
	}
	conversion, ok := call.Args[0].(*ast.CallExpr)
	return ok && len(conversion.Args) == 1 && timerReferenceIdentifier(conversion.Fun, "uint64") && timerReferenceIdentifier(conversion.Args[0], id)
}

func typeAssertionType(expression *ast.TypeAssertExpr) (string, bool) {
	if expression == nil {
		return "", false
	}
	pointer, ok := expression.Type.(*ast.StarExpr)
	if !ok {
		return "", false
	}
	identifier, ok := pointer.X.(*ast.Ident)
	if !ok {
		return "", false
	}
	return identifier.Name, true
}

func timerReferenceMissingReturn(statement ast.Stmt, found string) bool {
	branch, ok := statement.(*ast.IfStmt)
	return ok && branch.Else == nil && timerReferenceNegatedIdentifier(branch.Cond, found) && len(branch.Body.List) == 1 && timerReferenceBareReturn(branch.Body.List[0])
}

func timerReferenceBareReturn(statement ast.Stmt) bool {
	result, ok := statement.(*ast.ReturnStmt)
	return ok && len(result.Results) == 0
}

func timerReferenceNilReturn(statement ast.Stmt) bool {
	result, ok := statement.(*ast.ReturnStmt)
	return ok && len(result.Results) == 1 && timerReferenceIdentifier(result.Results[0], "nil")
}
