package tournament

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

type timerReferenceCoreShape struct {
	MissingReturns bool
	SwapsAtomicBit bool
	ReferencedAdd  int64
	UnrefedAdd     int64
	Excluded       int
}

func TestTimerReferenceASTExtractionProof(t *testing.T) {
	repository, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	int32Descriptor, ok := timerReferenceDescriptorID("timer.ref-core.map-swap-int32.v1")
	if !ok {
		t.Fatal("missing Int32 reference descriptor")
	}
	int64Descriptor, ok := timerReferenceDescriptorID("timer.ref-core.map-swap-int64.v1")
	if !ok {
		t.Fatal("missing Int64 reference descriptor")
	}

	historical := []struct {
		owner  string
		source componentSourceIdentity
	}{{owner: int32Descriptor.SourceStorageID, source: int32Descriptor.Sources[0]}}
	for _, binding := range timerReferenceStorageBindings {
		if binding.Disposition == timerReferenceBindingNormalizedAlias {
			historical = append(historical, struct {
				owner  string
				source componentSourceIdentity
			}{owner: binding.StorageID, source: binding.NormalizedSource})
		}
	}
	for _, candidate := range historical {
		t.Run(candidate.owner, func(t *testing.T) {
			payload := timerReferenceSourcePayload(t, repository, candidate.source)
			shape, shapeErr := parseTimerReferenceCore(payload, "applyTimerRefChange", "timerMap", "refedTimerCount")
			if shapeErr != nil {
				t.Fatal(shapeErr)
			}
			if shape != (timerReferenceCoreShape{MissingReturns: true, SwapsAtomicBit: true, ReferencedAdd: 1, UnrefedAdd: -1, Excluded: 2}) {
				t.Fatalf("historical owner core shape = %+v", shape)
			}
		})
	}

	currentSource := timerReferenceSourcePayload(t, repository, int64Descriptor.Sources[0])
	currentShape, err := parseTimerReferenceCore(currentSource, "applyTimerRefChange", "timerMap", "refedTimerCount")
	if err != nil {
		t.Fatal(err)
	}
	if currentShape != (timerReferenceCoreShape{MissingReturns: true, SwapsAtomicBit: true, ReferencedAdd: 1, UnrefedAdd: -1, Excluded: 2}) {
		t.Fatalf("current owner core shape = %+v", currentShape)
	}

	for _, descriptor := range []timerReferenceDescriptor{int32Descriptor, int64Descriptor} {
		t.Run(descriptor.ID, func(t *testing.T) {
			payload := timerReferenceSourcePayload(t, repository, descriptor.MaterializationSources[0])
			shape, shapeErr := parseTimerReferenceCore(payload, "Apply", "entries", "refed")
			if shapeErr != nil {
				t.Fatal(shapeErr)
			}
			if shape != (timerReferenceCoreShape{MissingReturns: true, SwapsAtomicBit: true, ReferencedAdd: 1, UnrefedAdd: -1}) {
				t.Fatalf("materialized owner core shape = %+v", shape)
			}
		})
	}
}

func TestTimerReferenceASTExtractionRejectsDrift(t *testing.T) {
	canonical := `package reference
func (c *Core) Apply(id ID, refed bool) {
	value, exists := c.entries[id]
	if !exists { return }
	old := value.refed.Swap(refed)
	if old != refed {
		if refed { c.refed.Add(1) } else { c.refed.Add(-1) }
	}
}`
	mutations := map[string]string{
		"missing return":   strings.Replace(canonical, "if !exists { return }", "if !exists {}", 1),
		"non-atomic bit":   strings.Replace(canonical, ".Swap(refed)", ".Store(refed)", 1),
		"wrong increment":  strings.Replace(canonical, ".Add(1)", ".Add(2)", 1),
		"extra timed work": strings.Replace(canonical, "if refed {", "c.wake()\n\t\tif refed {", 1),
	}
	for name, source := range mutations {
		t.Run(name, func(t *testing.T) {
			shape, err := parseTimerReferenceCore([]byte(source), "Apply", "entries", "refed")
			if err == nil && shape == (timerReferenceCoreShape{MissingReturns: true, SwapsAtomicBit: true, ReferencedAdd: 1, UnrefedAdd: -1}) {
				t.Fatalf("accepted semantic drift: %+v", shape)
			}
		})
	}
}

func timerReferenceSourcePayload(t *testing.T, repository string, source componentSourceIdentity) []byte {
	t.Helper()
	var object string
	switch source.ProvenanceKind {
	case "commit":
		object = source.OriginCommit + ":eventloop/" + source.Path
	case "archived-index-candidate", "index-candidate-materialization":
		object = ":eventloop/" + source.Path
	default:
		t.Fatalf("unsupported reference provenance kind %q", source.ProvenanceKind)
	}
	return runComponentGit(t, repository, "show", object)
}

func parseTimerReferenceCore(source []byte, function, mapField, aggregateField string) (timerReferenceCoreShape, error) {
	file, err := parser.ParseFile(token.NewFileSet(), function+".go", source, 0)
	if err != nil {
		return timerReferenceCoreShape{}, fmt.Errorf("parse %s: %w", function, err)
	}
	var declaration *ast.FuncDecl
	for _, candidate := range file.Decls {
		if value, ok := candidate.(*ast.FuncDecl); ok && value.Name.Name == function {
			if declaration != nil {
				return timerReferenceCoreShape{}, fmt.Errorf("duplicate function %s", function)
			}
			declaration = value
		}
	}
	if declaration == nil || declaration.Recv == nil || declaration.Body == nil {
		return timerReferenceCoreShape{}, fmt.Errorf("missing receiver method %s", function)
	}
	if len(declaration.Recv.List) != 1 || len(declaration.Recv.List[0].Names) != 1 {
		return timerReferenceCoreShape{}, fmt.Errorf("%s receiver differs", function)
	}
	receiverName := declaration.Recv.List[0].Names[0].Name
	parameters := declaration.Type.Params.List
	if len(parameters) != 2 || len(parameters[0].Names) != 1 || len(parameters[1].Names) != 1 {
		return timerReferenceCoreShape{}, fmt.Errorf("%s parameters are not (id, reference bit)", function)
	}
	idName := parameters[0].Names[0].Name
	refName := parameters[1].Names[0].Name
	statements := declaration.Body.List
	if len(statements) != 4 {
		return timerReferenceCoreShape{}, fmt.Errorf("%s top-level statement count = %d, want 4", function, len(statements))
	}

	lookup, ok := statements[0].(*ast.AssignStmt)
	if !ok || lookup.Tok != token.DEFINE || len(lookup.Lhs) != 2 || len(lookup.Rhs) != 1 {
		return timerReferenceCoreShape{}, fmt.Errorf("%s has no two-result map lookup", function)
	}
	entryName, okEntry := lookup.Lhs[0].(*ast.Ident)
	foundName, okFound := lookup.Lhs[1].(*ast.Ident)
	index, okIndex := lookup.Rhs[0].(*ast.IndexExpr)
	if !okEntry || !okFound || !okIndex || !timerReferenceOwnedSelector(index.X, receiverName, mapField) || !timerReferenceIdentifier(index.Index, idName) {
		return timerReferenceCoreShape{}, fmt.Errorf("%s map lookup shape differs", function)
	}

	missing, ok := statements[1].(*ast.IfStmt)
	if !ok || missing.Else != nil || !timerReferenceNegatedIdentifier(missing.Cond, foundName.Name) || len(missing.Body.List) != 1 {
		return timerReferenceCoreShape{}, fmt.Errorf("%s missing-ID branch differs", function)
	}
	if _, ok := missing.Body.List[0].(*ast.ReturnStmt); !ok {
		return timerReferenceCoreShape{}, fmt.Errorf("%s missing-ID branch does not return", function)
	}

	swap, ok := statements[2].(*ast.AssignStmt)
	if !ok || swap.Tok != token.DEFINE || len(swap.Lhs) != 1 || len(swap.Rhs) != 1 {
		return timerReferenceCoreShape{}, fmt.Errorf("%s reference swap assignment differs", function)
	}
	oldName, ok := swap.Lhs[0].(*ast.Ident)
	if !ok || !timerReferenceCall(swap.Rhs[0], entryName.Name, "refed", "Swap", refName) {
		return timerReferenceCoreShape{}, fmt.Errorf("%s reference bit is not atomically swapped", function)
	}

	changed, ok := statements[3].(*ast.IfStmt)
	if !ok || changed.Else != nil || !timerReferenceNotEqual(changed.Cond, oldName.Name, refName) || len(changed.Body.List) == 0 {
		return timerReferenceCoreShape{}, fmt.Errorf("%s changed-bit branch differs", function)
	}
	delta, ok := changed.Body.List[0].(*ast.IfStmt)
	if !ok || !timerReferenceIdentifier(delta.Cond, refName) || len(delta.Body.List) != 1 {
		return timerReferenceCoreShape{}, fmt.Errorf("%s aggregate branch differs", function)
	}
	otherwise, ok := delta.Else.(*ast.BlockStmt)
	if !ok || len(otherwise.List) != 1 {
		return timerReferenceCoreShape{}, fmt.Errorf("%s unref aggregate branch differs", function)
	}
	up, ok := timerReferenceAdd(delta.Body.List[0], receiverName, aggregateField)
	if !ok {
		return timerReferenceCoreShape{}, fmt.Errorf("%s referenced aggregate update differs", function)
	}
	down, ok := timerReferenceAdd(otherwise.List[0], receiverName, aggregateField)
	if !ok {
		return timerReferenceCoreShape{}, fmt.Errorf("%s unref aggregate update differs", function)
	}
	return timerReferenceCoreShape{
		MissingReturns: true,
		SwapsAtomicBit: true,
		ReferencedAdd:  up,
		UnrefedAdd:     down,
		Excluded:       len(changed.Body.List) - 1,
	}, nil
}

func timerReferenceOwnedSelector(expression ast.Expr, receiver, field string) bool {
	selector, ok := expression.(*ast.SelectorExpr)
	return ok && selector.Sel.Name == field && timerReferenceIdentifier(selector.X, receiver)
}

func timerReferenceIdentifier(expression ast.Expr, name string) bool {
	identifier, ok := expression.(*ast.Ident)
	return ok && identifier.Name == name
}

func timerReferenceNegatedIdentifier(expression ast.Expr, name string) bool {
	unary, ok := expression.(*ast.UnaryExpr)
	return ok && unary.Op == token.NOT && timerReferenceIdentifier(unary.X, name)
}

func timerReferenceNotEqual(expression ast.Expr, left, right string) bool {
	binary, ok := expression.(*ast.BinaryExpr)
	return ok && binary.Op == token.NEQ && timerReferenceIdentifier(binary.X, left) && timerReferenceIdentifier(binary.Y, right)
}

func timerReferenceCall(expression ast.Expr, receiver, field, method, argument string) bool {
	call, ok := expression.(*ast.CallExpr)
	if !ok || len(call.Args) != 1 || !timerReferenceIdentifier(call.Args[0], argument) {
		return false
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != method {
		return false
	}
	owner, ok := selector.X.(*ast.SelectorExpr)
	return ok && owner.Sel.Name == field && timerReferenceIdentifier(owner.X, receiver)
}

func timerReferenceAdd(statement ast.Stmt, receiver, aggregate string) (int64, bool) {
	expression, ok := statement.(*ast.ExprStmt)
	if !ok {
		return 0, false
	}
	call, ok := expression.X.(*ast.CallExpr)
	if !ok || len(call.Args) != 1 {
		return 0, false
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "Add" || !timerReferenceOwnedSelector(selector.X, receiver, aggregate) {
		return 0, false
	}
	switch value := call.Args[0].(type) {
	case *ast.BasicLit:
		if value.Kind == token.INT && value.Value == "1" {
			return 1, true
		}
	case *ast.UnaryExpr:
		literal, literalOK := value.X.(*ast.BasicLit)
		if value.Op == token.SUB && literalOK && literal.Kind == token.INT && literal.Value == "1" {
			return -1, true
		}
	}
	return 0, false
}
