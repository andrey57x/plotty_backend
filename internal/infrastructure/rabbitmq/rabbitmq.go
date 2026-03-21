package rabbitmq

import (
	"fmt"
	"log"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

func NewConnection(url string) (*amqp.Connection, error) {
	var conn *amqp.Connection
	var err error

	// Пробуем подключиться 5 раз с паузой в 5 секунд
	for i := 0; i < 5; i++ {
		conn, err = amqp.Dial(url)
		if err == nil {
			return conn, nil
		}

		log.Printf("[RabbitMQ] Попытка %d: Брокер еще не готов (%v). Ждем...", i+1, err)
		time.Sleep(5 * time.Second)
	}

	return nil, fmt.Errorf("не удалось подключиться к RabbitMQ после 5 попыток: %w", err)
}