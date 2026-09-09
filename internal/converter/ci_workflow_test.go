package converter

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The CI workflow used to select unit tests with -run 'Test[^C]', which silently
// excluded every test whose name began with "TestC" — including six
// dependency-free tests of Convert and of the pandoc argument construction. The
// loss was invisible, because the integration job still ran them and reported
// green.
//
// These tests pin the fix in place: the unit job must not filter by test name,
// and the heavy test must stay behind a build tag rather than behind a naming
// convention. They read the workflow file directly, since that is where the
// mistake lived.

// workflowPath is the CI definition, relative to this package's directory.
const workflowPath = "../../.github/workflows/ci.yml"

// runFlagRe matches a "go test" invocation that selects tests by name.
var runFlagRe = regexp.MustCompile(`go test[^\n]*\s-run[\s=]`)

func readWorkflow(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Clean(workflowPath))
	if err != nil {
		t.Fatalf("read %s: %v", workflowPath, err)
	}
	return string(data)
}

func TestWorkflow_DoesNotSelectTestsByName(t *testing.T) {
	workflow := readWorkflow(t)

	if loc := runFlagRe.FindString(workflow); loc != "" {
		t.Errorf("CI selects tests by name (%q).\n"+
			"Name-based selection silently drops tests whose names happen to match. "+
			"Put anything needing external tools behind the \"integration\" build tag instead.", loc)
	}
}

func TestWorkflow_RunsIntegrationTestsUnderTheBuildTag(t *testing.T) {
	workflow := readWorkflow(t)

	if !strings.Contains(workflow, "-tags integration") {
		t.Error("no CI step runs the integration-tagged tests; " +
			"the integration job must pass -tags integration or that suite never runs")
	}
}

// TestWorkflow_IntegrationFileCarriesBuildTag checks the tag is actually on the
// file, so the untagged unit run stays free of external-tool dependencies.
func TestWorkflow_IntegrationFileCarriesBuildTag(t *testing.T) {
	const path = "converter_integration_test.go"
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if !strings.HasPrefix(string(data), "//go:build integration") {
		t.Errorf("%s must start with //go:build integration, "+
			"or the dependency-free unit run will try to drive mmdc and Chromium", path)
	}
}
