package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

type Message struct {
	Role    Role   `json:"role"`
	Content string `json:"content"`
}

type Request struct {
	Messages        []Message `json:"messages"`
	MaxOutputTokens int       `json:"max_output_tokens"`
}

type DecisionKind string

const (
	DecisionTool DecisionKind = "tool"
	DecisionDone DecisionKind = "done"
)

type Decision struct {
	Kind      DecisionKind    `json:"kind"`
	ToolName  string          `json:"tool_name,omitempty"`
	ToolInput json.RawMessage `json:"tool_input,omitempty"`
	Final     string          `json:"final,omitempty"`
	Reason    string          `json:"reason,omitempty"`
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

type Client interface {
	Decide(ctx context.Context, req Request) (Decision, Usage, error)
}

type Policy struct {
	MaxPromptTokens int
	MaxRetries      int
	BaseBackoff     time.Duration
}

type RetryingClient struct {
	inner  Client
	policy Policy
}

func NewRetryingClient(inner Client, policy Policy) *RetryingClient {
	if policy.MaxRetries < 0 {
		policy.MaxRetries = 0
	}
	if policy.BaseBackoff <= 0 {
		policy.BaseBackoff = 30 * time.Millisecond
	}
	return &RetryingClient{inner: inner, policy: policy}
}

func (c *RetryingClient) Decide(ctx context.Context, req Request) (Decision, Usage, error) {
	if c.policy.MaxPromptTokens > 0 {
		est := EstimatePromptTokens(req.Messages)
		if est > c.policy.MaxPromptTokens {
			return Decision{}, Usage{}, fmt.Errorf("prompt budget exceeded: got %d want <= %d", est, c.policy.MaxPromptTokens)
		}
	}

	attempts := c.policy.MaxRetries + 1
	var lastErr error
	for i := 0; i < attempts; i++ {
		decision, usage, err := c.inner.Decide(ctx, req)
		if err == nil {
			return decision, usage, nil
		}
		lastErr = err
		if !IsRetryable(err) || i == attempts-1 {
			break
		}
		backoff := c.policy.BaseBackoff * time.Duration(1<<i)
		select {
		case <-ctx.Done():
			return Decision{}, Usage{}, ctx.Err()
		case <-time.After(backoff):
		}
	}
	return Decision{}, Usage{}, lastErr
}

type RetryableError struct {
	err error
}

func (e RetryableError) Error() string { return e.err.Error() }
func (e RetryableError) Unwrap() error { return e.err }

func WrapRetryable(err error) error {
	if err == nil {
		return nil
	}
	return RetryableError{err: err}
}

func IsRetryable(err error) bool {
	var re RetryableError
	return errors.As(err, &re)
}

func EstimatePromptTokens(messages []Message) int {
	if len(messages) == 0 {
		return 0
	}
	chars := 0
	for _, m := range messages {
		chars += len(m.Content) + 8
	}
	if chars < 4 {
		return 1
	}
	return chars / 4
}

// DeterministicClient keeps early migration stages fully offline and reproducible.
type DeterministicClient struct{}

func NewDeterministicClient() *DeterministicClient {
	return &DeterministicClient{}
}

func (c *DeterministicClient) Decide(_ context.Context, req Request) (Decision, Usage, error) {
	var goal string
	seenToolResult := false
	for i := len(req.Messages) - 1; i >= 0; i-- {
		m := req.Messages[i]
		if m.Role == RoleUser && goal == "" {
			goal = m.Content
		}
		if m.Role == RoleTool {
			seenToolResult = true
		}
	}
	if goal == "" {
		goal = "no-goal"
	}

	usage := Usage{PromptTokens: EstimatePromptTokens(req.Messages), CompletionTokens: 30}
	if seenToolResult {
		return Decision{
			Kind:   DecisionDone,
			Final:  "Goal completed with one deterministic tool step.",
			Reason: "Tool result available; finishing run.",
		}, usage, nil
	}

	payload, _ := json.Marshal(map[string]string{"text": strings.TrimSpace(goal)})
	return Decision{
		Kind:      DecisionTool,
		ToolName:  "echo",
		ToolInput: payload,
		Reason:    "First step calls echo tool in bootstrap mode.",
	}, usage, nil
}
