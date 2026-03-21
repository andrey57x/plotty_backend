package rabbitmq

// Сообщение с задачей (Core -> ML)
type MLTaskMessage struct {
	TaskID  string `json:"task_id"`
	Type    string `json:"type"` // "spellcheck" или "image_generation"
	Payload string `json:"payload"`
}

// Сообщение с результатом (ML -> Core)
type MLResultMessage struct {
	TaskID string `json:"task_id"`
	Status string `json:"status"` // "completed" или "failed"
	Result []byte `json:"result"` // Сырой JSON ответа
	Error  string `json:"error"`
}
