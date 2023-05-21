package mysql

import (
	"bytes"
	"github.com/pingcap/tidb/parser/ast"
	"github.com/pingcap/tidb/parser/format"
	"io"
	"strings"
)

type (
	astVisitor struct {
		fn   func(node ast.Node) (skip, ok bool)
		done bool
	}

	astNode = ast.Node

	// astNodeString implements ast.Node.Restore by writing content as-is
	astNodeString struct {
		astNode
		content string
	}

	// astNodeSlice implements ast.Node.Restore by passing through to each of nodes, skipping nil values
	astNodeSlice struct {
		astNode
		nodes []ast.Node
	}

	astExprNodeStub struct {
		astNode
		astExprNodeNested
	}

	astExprNode       = ast.ExprNode
	astExprNodeNested struct{ astExprNode }
)

func (x *astVisitor) Enter(node ast.Node) (ast.Node, bool) {
	if x.done {
		return node, true
	}
	skip, ok := x.fn(node)
	if !ok {
		x.done = true
		return node, true
	}
	return node, skip
}

func (x *astVisitor) Leave(node ast.Node) (ast.Node, bool) {
	return node, !x.done
}

func (x *astNodeString) Restore(ctx *format.RestoreCtx) (err error) {
	_, err = io.Copy(ctx.In, strings.NewReader(x.content))
	return
}

func (x *astNodeSlice) Restore(ctx *format.RestoreCtx) error {
	for _, node := range x.nodes {
		if node == nil {
			continue
		}
		if err := node.Restore(ctx); err != nil {
			return err
		}
	}
	return nil
}

func astNodeFormat(node ast.Node, flags format.RestoreFlags) (string, error) {
	var b bytes.Buffer
	if err := node.Restore(format.NewRestoreCtx(flags, &b)); err != nil {
		return ``, err
	}
	return b.String(), nil
}

func astNodeAnd(conditions ...ast.Node) ast.ExprNode {
	var nodes []ast.Node
	for _, node := range conditions {
		if len(nodes) != 0 {
			nodes = append(nodes, &astNodeString{content: ` AND `})
		}
		nodes = append(
			nodes,
			&astNodeString{content: `(`},
			node,
			&astNodeString{content: `)`},
		)
	}
	return &astExprNodeStub{astNode: &astNodeSlice{nodes: nodes}}
}
