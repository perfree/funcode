package builtin

import "github.com/perfree/funcode/internal/tool"

// RegisterAll registers all builtin tools to the registry
func RegisterAll(registry *tool.Registry) {
	registry.Register(NewBashTool())
	registry.Register(NewReadTool())
	registry.Register(NewReadRangeTool())
	registry.Register(NewWriteTool())
	registry.Register(NewEditTool())
	registry.Register(NewPatchTool())
	registry.Register(NewDeleteTool())
	registry.Register(NewGlobTool())
	registry.Register(NewTreeTool())
	registry.Register(NewGrepTool())
	registry.Register(NewDiffTool())
	registry.Register(NewRunTaskTool())
}
