// Copyright 2018 The CUE Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cue

import (
	"strings"

	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/build"
	"cuelang.org/go/cue/errors"
	"cuelang.org/go/cue/token"
	"cuelang.org/go/internal"
)

// An Instance defines a single configuration based on a collection of
// underlying CUE files.
type Instance struct {
	*index

	rootStruct *structLit // the struct to insert root values into
	rootValue  value      // the value to evaluate: may add comprehensions

	// scope is used as an additional top-level scope between the package scope
	// and the predeclared identifiers.
	scope *structLit

	ImportPath string
	Dir        string
	Name       string

	Incomplete bool  // true if Pkg and all its dependencies are free of errors
	Err        error // non-nil if the package had errors

	inst *build.Instance

	complete bool // for cycle detection
}

// NewInstance creates a new instance. Use Insert to populate the instance.
func (x *index) NewInstance(p *build.Instance) *Instance {
	st := &structLit{baseValue: baseValue{nil}}
	i := &Instance{
		index:      x,
		rootStruct: st,
		rootValue:  st,
		inst:       p,
	}
	if p != nil {
		i.ImportPath = p.ImportPath
		i.Dir = p.Dir
		i.Name = p.PkgName
		if p.Err != nil {
			i.setListOrError(p.Err)
		}
	}
	return i
}

func (inst *Instance) setListOrError(err error) {
	inst.Incomplete = true
	switch x := err.(type) {
	case errors.List:
		if inst.Err == nil {
			inst.Err = x
			return
		}
		for _, e := range x {
			inst.setError(e)
		}
	case errors.Error:
		inst.setError(x)
	default:
		inst.setError(errors.Wrapf(err, token.NoPos, "unknown error"))
	}
}

func (inst *Instance) setError(err errors.Error) {
	inst.Incomplete = true
	var list errors.List
	switch x := inst.Err.(type) {
	default:
		// Should never happen, but in the worst case at least one error is
		// recorded.
		return
	case nil:
		inst.Err = err
		return
	case errors.List:
		list = x
	case errors.Error:
		list.Add(err)
	}
	inst.Err = list
}

func (inst *Instance) eval(ctx *context) evaluated {
	// TODO: remove manifest here? It may be good here, though not consistent.
	v := ctx.manifest(inst.rootValue)
	if s, ok := v.(*structLit); ok && s.emit != nil {
		v = ctx.manifest(s.emit)
	}
	// manifest to benefit from validation step.
	// TODO: consider not doing manifest.
	return ctx.manifest(v)
}

func init() {
	internal.EvalExpr = func(value, expr interface{}) interface{} {
		v := value.(Value)
		e := expr.(ast.Expr)
		ctx := v.idx.newContext()
		return newValueRoot(ctx, evalExpr(v.idx, v.eval(ctx), e))
	}
}

func evalExpr(idx *index, x value, expr ast.Expr) evaluated {
	if isBottom(x) {
		return idx.mkErr(x, "error evaluating instance: %v", x)
	}
	obj, ok := x.(*structLit)
	if !ok {
		return idx.mkErr(obj, "instance is not a struct")
	}

	v := newVisitor(idx, nil, nil, obj)
	return eval(idx, v.walk(expr))
}

func (inst *Instance) evalExpr(ctx *context, expr ast.Expr) evaluated {
	root := inst.eval(ctx)
	if isBottom(root) {
		return ctx.mkErr(root, "error evaluating instance")
	}
	obj, ok := root.(*structLit)
	if !ok {
		return ctx.mkErr(obj, "instance is not a struct")
	}
	v := newVisitor(ctx.index, inst.inst, nil, obj)
	return v.walk(expr).evalPartial(ctx)
}

// Doc returns the package comments for this instance.
func (inst *Instance) Doc() []*ast.CommentGroup {
	var docs []*ast.CommentGroup
	if inst.inst == nil {
		return nil
	}
	for _, f := range inst.inst.Files {
		if strings.HasPrefix(f.Filename, inst.Dir) {
			continue
		}
		var cg *ast.CommentGroup
		for _, c := range f.Comments() {
			if c.Position == 0 {
				cg = c
			}
		}
		if cg != nil {
			docs = append(docs, cg)
		}
	}
	return docs
}

// Value returns the root value of the configuration. If the configuration
// defines in emit value, it will be that value. Otherwise it will be all
// top-level values.
func (inst *Instance) Value() Value {
	ctx := inst.newContext()
	return newValueRoot(ctx, inst.eval(ctx))
}

// Eval evaluates an expression within an existing instance.
//
// Expressions may refer to builtin packages if they can be uniquely identified.
func (inst *Instance) Eval(expr ast.Expr) Value {
	ctx := inst.newContext()
	result := inst.evalExpr(ctx, expr)
	return newValueRoot(ctx, result)
}

// Merge unifies the given instances into a single one.
//
// Errors regarding conflicts are included in the result, but not reported, so
// that these will only surface during manifestation. This allows
// non-conflicting parts to be used.
func Merge(inst ...*Instance) *Instance {
	switch len(inst) {
	case 0:
		return nil
	case 1:
		return inst[0]
	}

	values := []value{}
	for _, i := range inst {
		if i.Err != nil {
			return i
		}
		values = append(values, i.rootValue)
	}
	merged := &mergedValues{values: values}

	ctx := inst[0].newContext()

	st, ok := ctx.manifest(merged).(*structLit)
	if !ok {
		return nil
	}

	p := &Instance{
		rootStruct: st,
		rootValue:  merged,
		index:      ctx.index,
		complete:   true,
	}
	return p
}

// Build creates a new instance from the build instances, allowing unbound
// identifier to bind to the top-level field in inst. The top-level fields in
// inst take precedence over predeclared identifier and builtin functions.
func (inst *Instance) Build(p *build.Instance) *Instance {
	p.Complete()

	idx := inst.index

	i := idx.NewInstance(p)
	if i.Err != nil {
		return i
	}

	ctx := inst.newContext()
	val := newValueRoot(ctx, inst.rootValue)
	v, err := val.structVal(ctx)
	if err != nil {
		i.setError(val.toErr(err))
		return i
	}
	i.scope = v.n

	if err := resolveFiles(idx, p); err != nil {
		i.setError(err)
		return i
	}
	for _, f := range p.Files {
		if err := i.insertFile(f); err != nil {
			i.setListOrError(err)
		}
	}
	i.complete = true

	return i
}

// Lookup reports the value starting from the top level struct (not the emitted
// value), or an error if the path is not found.
// The empty path returns the top-level configuration struct, regardless of
// whether an emit value was specified.
func (inst *Instance) Lookup(path ...string) Value {
	idx := inst.index
	ctx := idx.newContext()
	v := newValueRoot(ctx, inst.rootValue)
	for _, k := range path {
		obj, err := v.structVal(ctx)
		if err != nil {
			return Value{idx, &valueData{arc: arc{cache: err, v: err}}}
		}
		v = obj.Lookup(k)
	}
	return v
}

// Fill creates a new instance with the values of the old instance unified with
// the given value. It is not possible to update the emit value.
func (inst *Instance) Fill(x interface{}, path ...string) (*Instance, error) {
	ctx := inst.newContext()
	root := ctx.manifest(inst.rootValue)
	for i := len(path) - 1; i >= 0; i-- {
		x = map[string]interface{}{path[i]: x}
	}
	value := convert(ctx, root, x)
	eval := binOp(ctx, baseValue{}, opUnify, root, value)
	// TODO: validate recursively?
	err := inst.Err
	var st *structLit
	switch x := eval.(type) {
	case *structLit:
		st = x
	default:
		// This should not happen.
		err = errors.Newf(x.Pos(), "error filling struct")
	case *bottom:
		err = inst.Value().toErr(x)
	}
	inst = &Instance{
		rootStruct: st,
		rootValue:  st,
		index:      inst.index,
		inst:       nil,

		ImportPath: inst.ImportPath,
		Name:       inst.Name,
		Incomplete: inst.Incomplete,
		Err:        err,

		complete: err != nil,
	}
	return inst, err
}
