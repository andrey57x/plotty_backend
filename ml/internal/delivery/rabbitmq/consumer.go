package rabbitmq

import (
	"context"
	"encoding/json"
	"log"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"

	// Импортируем общую структуру сообщений
	sharedrmq "github.com/fivecode/plotty/internal/infrastructure/rabbitmq"
)

const TaskQueueName = "ml_tasks_queue"

type MLUsecase interface {
	ProcessSpellcheck(ctx context.Context, taskID uuid.UUID, payload string) error
	ProcessImageGen(ctx context.Context, taskID uuid.UUID, payload string) error
}

type Consumer struct {
	conn    *amqp.Connection
	channel *amqp.Channel
	usecase MLUsecase
}

func NewConsumer(conn *amqp.Connection, uc MLUsecase) (*Consumer, error) {
	ch, err := conn.Channel()
	if err != nil {
		return nil, err
	}

	_, err = ch.QueueDeclare(
		TaskQueueName,
		true,  // durable
		false, // auto-delete
		false, // exclusive
		false, // no-wait
		nil,   // arguments
	)
	if err != nil {
		return nil, err
	}

	return &Consumer{
		conn:    conn,
		channel: ch,
		usecase: uc,
	}, nil
}

func (c *Consumer) Start(ctx context.Context) error {
	msgs, err := c.channel.Consume(
		TaskQueueName,
		"ml_worker",
		false, // auto-ack отключен, отправляем руками
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return err
	}

	log.Println("Воркер ML начал слушать задачи из RabbitMQ...")

	for {
		select {
		case <-ctx.Done():
			log.Println("Остановка консьюмера...")
			return nil
		case msg, ok := <-msgs:
			if !ok {
				return nil
			}

			// Парсим в ОБЩУЮ структуру
			var task sharedrmq.MLTaskMessage
			if err := json.Unmarshal(msg.Body, &task); err != nil {
				log.Printf("Ошибка парсинга JSON: %v", err)
				msg.Nack(false, false)
				continue
			}

			taskID, err := uuid.Parse(task.TaskID)
			if err != nil {
				log.Printf("Невалидный UUID: %s", task.TaskID)
				msg.Nack(false, false)
				continue
			}

			log.Printf("Получена задача [%s]: %s", task.Type, taskID)

			var processErr error
			switch task.Type {
			case "spellcheck":
				processErr = c.usecase.ProcessSpellcheck(ctx, taskID, task.Payload)
			case "image_gen": // Имя совпадает с тем, что отправляет core
				processErr = c.usecase.ProcessImageGen(ctx, taskID, task.Payload)
			default:
				log.Printf("Неизвестный тип задачи: %s", task.Type)
			}

			if processErr != nil {
				log.Printf("Ошибка обработки %s: %v", taskID, processErr)
				msg.Nack(false, false) // Nack, чтобы сообщение ушло в drop или dead-letter
			} else {
				log.Printf("Задача %s выполнена успешно", taskID)
				msg.Ack(false) // Подтверждаем RabbitMQ
			}
		}
	}
}

func (c *Consumer) Close() {
	if c.channel != nil {
		c.channel.Close()
	}
}
