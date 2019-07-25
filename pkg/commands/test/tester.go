package test

import (
	"bytes"
	"context"

	"github.com/containerd/containerd/log"
	"github.com/hashicorp/go-multierror"
	"github.com/open-policy-agent/opa/ast"
	"github.com/splucs/conftest/pkg/parser"
)

type Tester struct {
	compiler *ast.Compiler
}

func BuildTester(ctx context.Context, policiesDir string) (*Tester, error) {
	compiler, err := buildCompiler(policiesDir)
	if err != nil {
		log.G(ctx).Fatalf("Problem building rego compiler: %s", err)
	}
	return &Tester{
		compiler: compiler,
	}, err
}

func (t *Tester) ProcessManifest(ctx context.Context, data []byte) (error, error) {
	linebreak := detectLineBreak(data)
	bits := bytes.Split(data, []byte(linebreak+"---"+linebreak))

	parser := parser.GetParser("*.yaml")

	var failuresList *multierror.Error
	var warningsList *multierror.Error
	for _, element := range bits {
		var input interface{}
		err := parser.Unmarshal([]byte(element), &input)
		if err != nil {
			return err, nil
		}
		failures, warnings := processData(ctx, input, t.compiler)
		if failures != nil {
			failuresList = multierror.Append(failuresList, failures)
		}
		if warnings != nil {
			warningsList = multierror.Append(warningsList, warnings)
		}
	}
	return failuresList.ErrorOrNil(), warningsList.ErrorOrNil()
}
