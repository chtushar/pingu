package evals

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// LoadCase loads a single eval case from a YAML file.
func LoadCase(path string) (*EvalCase, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	var tc EvalCase
	if err := yaml.Unmarshal(data, &tc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	// Default name from filename if not specified.
	if tc.Name == "" {
		base := filepath.Base(path)
		tc.Name = strings.TrimSuffix(base, filepath.Ext(base))
	}

	return &tc, nil
}

// EvalSuite groups cases loaded from a directory.
type EvalSuite struct {
	Name  string
	Cases []EvalCase
}

// LoadSuites walks a directory tree and loads all .yaml files, grouped by
// immediate subdirectory into EvalSuites.
func LoadSuites(root string) ([]EvalSuite, error) {
	suiteMap := make(map[string]*EvalSuite)

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || filepath.Ext(path) != ".yaml" {
			return nil
		}

		tc, err := LoadCase(path)
		if err != nil {
			return err
		}

		// Suite name = first directory component relative to root.
		rel, _ := filepath.Rel(root, path)
		parts := strings.SplitN(rel, string(filepath.Separator), 2)
		suiteName := "default"
		if len(parts) > 1 {
			suiteName = parts[0]
		}

		suite, ok := suiteMap[suiteName]
		if !ok {
			suite = &EvalSuite{Name: suiteName}
			suiteMap[suiteName] = suite
		}
		suite.Cases = append(suite.Cases, *tc)

		return nil
	})
	if err != nil {
		return nil, err
	}

	suites := make([]EvalSuite, 0, len(suiteMap))
	for _, s := range suiteMap {
		suites = append(suites, *s)
	}
	return suites, nil
}
