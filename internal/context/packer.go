package context

import "openclaw_go/internal/llm"

type Config struct {
	MaxPromptTokens   int
	ReserveForOutput  int
	MinMessagesToKeep int
}

type Packer struct {
	cfg Config
}

func NewPacker(cfg Config) *Packer {
	if cfg.MinMessagesToKeep <= 0 {
		cfg.MinMessagesToKeep = 1
	}
	return &Packer{cfg: cfg}
}

// Pack trims oldest messages until prompt budget is satisfied.
func (p *Packer) Pack(messages []llm.Message) []llm.Message {
	if len(messages) == 0 || p.cfg.MaxPromptTokens <= 0 {
		return messages
	}

	budget := p.cfg.MaxPromptTokens - p.cfg.ReserveForOutput
	if budget <= 0 {
		budget = p.cfg.MaxPromptTokens
	}

	trimmed := append([]llm.Message(nil), messages...)
	for len(trimmed) > p.cfg.MinMessagesToKeep && llm.EstimatePromptTokens(trimmed) > budget {
		trimmed = trimmed[1:]
	}
	return trimmed
}
