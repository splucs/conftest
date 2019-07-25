package test

import (
	"bytes"
	"context"

	"github.com/hashicorp/go-multierror"
	"github.com/open-policy-agent/opa/ast"
	"github.com/splucs/conftest/pkg/parser"
)

type StaticTester struct {
	compiler *ast.Compiler
}

func BuildCompiler(path string) (*StaticTester, error) {
	compiler, err := buildCompiler(path)
	if err != nil {
		return nil, err
	}
	return &StaticTester{
		compiler: compiler,
	}, nil
}

func (s *StaticTester) ProcessManifest(ctx context.Context, data []byte) (error, error) {
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
		failures, warnings := processData(ctx, input, s.compiler)
		if failures != nil {
			failuresList = multierror.Append(failuresList, failures)
		}
		if warnings != nil {
			warningsList = multierror.Append(warningsList, warnings)
		}
	}
	return failuresList.ErrorOrNil(), warningsList.ErrorOrNil()
}
