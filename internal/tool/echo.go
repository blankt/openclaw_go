package tool

import (
	"context"
	"encoding/json"
)

type echoInput struct {
	Text string `json:"text"`
}

type echoOutput struct {
	Echoed string `json:"echoed"`
}

type EchoTool struct{}

func NewEchoTool() *EchoTool {
	return &EchoTool{}
}

func (t *EchoTool) Name() string {
	return "echo"
}

func (t *EchoTool) Execute(_ context.Context, call Call) Result {
	var in echoInput
	if err := json.Unmarshal(call.Input, &in); err != nil {
		return Result{
			Name:    t.Name(),
			Success: false,
			Error: &Error{
				Code:      "invalid_input",
				Message:   err.Error(),
				Retryable: false,
			},
		}
	}

	out, _ := json.Marshal(echoOutput{Echoed: in.Text})
	return Result{Name: t.Name(), Success: true, Output: out}
}
