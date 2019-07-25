package parser

import (
	"path/filepath"

	"github.com/splucs/conftest/pkg/parser/cue"
	"github.com/splucs/conftest/pkg/parser/terraform"
	"github.com/splucs/conftest/pkg/parser/toml"
	"github.com/splucs/conftest/pkg/parser/yaml"
)

// Parser is the interface implemented by objects that can unmarshal
// bytes into a golang interface
type Parser interface {
	Unmarshal(p []byte, v interface{}) error
}

// GetParser returns a Parser for the given file type. Defaults to returning the YAML parser.
func GetParser(fileName string) Parser {
	suffix := filepath.Ext(fileName)

	switch suffix {
	case ".toml":
		return &toml.Parser{
			FileName: fileName,
		}
	case ".tf":
		return &terraform.Parser{
			FileName: fileName,
		}
	case ".cue":
		return &cue.Parser{
			FileName: fileName,
		}
	default:
		return &yaml.Parser{
			FileName: fileName,
		}
	}
}
