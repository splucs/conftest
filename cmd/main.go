package main

import (
	"os"

	"github.com/splucs/conftest/pkg/commands"
)

func main() {
	if err := commands.NewDefaultCommand().Execute(); err != nil {
		os.Exit(1)
	}
}
