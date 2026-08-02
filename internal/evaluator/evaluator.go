package evaluator

import (
	"os"

	"github.com/JeremyDev87/heimdall/internal/contract"
	"github.com/JeremyDev87/heimdall/internal/reducer"
	"github.com/JeremyDev87/heimdall/internal/report"
	"github.com/JeremyDev87/heimdall/internal/runner"
)

type Artifacts struct {
	Evidence   map[string]any
	Report     map[string]any
	Markdown   string
	TargetRoot string
}

func Evaluate(manifest string) (*Artifacts, error) {
	spec, err := contract.LoadSpec(manifest)
	if err != nil {
		return nil, err
	}
	temporary, err := os.MkdirTemp("", "heimdall-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(temporary)
	evidence, err := runner.Run(spec, temporary)
	if err != nil {
		return nil, err
	}
	assessment, err := reducer.Reduce(evidence)
	if err != nil {
		return nil, err
	}
	return &Artifacts{Evidence: evidence, Report: assessment, Markdown: report.RenderMarkdown(assessment), TargetRoot: spec.TargetRoot}, nil
}
func ExitCode(state string) int {
	switch state {
	case "PASS":
		return 0
	case "FAIL":
		return 1
	case "BLOCKED":
		return 2
	case "INCONCLUSIVE":
		return 3
	default:
		return 2
	}
}
