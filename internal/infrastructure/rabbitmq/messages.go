package rabbitmq

import "encoding/json"

// MLTaskMessage - Сообщение с задачей (Core -> ML)
type MLTaskMessage struct {
	TaskID   string            `json:"task_id"`
	TraceID  string            `json:"trace_id"` // Для сквозного логирования
	Type     string            `json:"type"`     // "spellcheck", "image_gen", "extract_lore", "check_logic", "generate_summary"
	Payload  string            `json:"payload"`
	Metadata map[string]string `json:"metadata,omitempty"` // Доп. данные (story_id, chapter_id)
}

// MLErrorDetails - Структурированная ошибка от ML
type MLErrorDetails struct {
	Code    string `json:"code"`    // Например: "LLM_TIMEOUT", "INVALID_PROMPT"
	Message string `json:"message"` // Человекочитаемая ошибка
	Details string `json:"details,omitempty"`
}

// MLResultMessage - Сообщение с результатом (ML -> Core)
type MLResultMessage struct {
	TaskID           string            `json:"task_id"`
	TraceID          string            `json:"trace_id"`
	Type             string            `json:"type"`
	Status           string            `json:"status"`
	Result           json.RawMessage   `json:"result,omitempty"`
	Error            *MLErrorDetails   `json:"error,omitempty"`
	Metadata         map[string]string `json:"metadata,omitempty"`
	PromptTokens     int               `json:"prompt_tokens,omitempty"`     // Добавлено для логирования входных токенов
	CompletionTokens int               `json:"completion_tokens,omitempty"` // Добавлено для логирования выходных токенов
	TotalTokens      int               `json:"total_tokens,omitempty"`      // Добавлено для логирования общих токенов
}
