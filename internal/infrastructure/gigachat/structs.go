package gigachat

// --- Структуры для Chat Completions ---
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatRequest struct {
	Model        string    `json:"model"`
	Messages     []Message `json:"messages"`
	Temperature  float64   `json:"temperature"`
	FunctionCall string    `json:"function_call,omitempty"`
}

type ChatResponse struct {
	Choices []struct {
		Message Message `json:"message"`
	} `json:"choices"`
}
