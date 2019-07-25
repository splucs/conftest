package test

import (
	"bytes"
	"context"

	"github.com/hashicorp/go-multierror"
	"github.com/open-policy-agent/opa/ast"
	"github.com/splucs/conftest/pkg/parser"
)

func BuildCompiler(path string) (*ast.Compiler, error) {
	return buildCompiler(path)
}

func ProcessManifest(ctx context.Context, data []byte, compiler *ast.Compiler) (error, error) {
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
		failures, warnings := processData(ctx, input, compiler)
		if failures != nil {
			failuresList = multierror.Append(failuresList, failures)
		}
		if warnings != nil {
			warningsList = multierror.Append(warningsList, warnings)
		}
	}
	return failuresList.ErrorOrNil(), warningsList.ErrorOrNil()
}
