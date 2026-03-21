package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type SpellcheckItem struct {
	FragmentText string `json:"fragmentText"`
	StartOffset  int    `json:"startOffset"`
	EndOffset    int    `json:"endOffset"`
	Message      string `json:"message"`
	Suggestion   string `json:"suggestion"`
}

type SpellcheckResult struct {
	Summary string           `json:"summary"`
	Items   []SpellcheckItem `json:"items"`
}

type ImageResult struct {
	URL    string `json:"url"`
	Prompt string `json:"prompt"`
}

type AITask struct {
	ID        uuid.UUID
	Status    string
	Result    json.RawMessage
	UpdatedAt time.Time
}
