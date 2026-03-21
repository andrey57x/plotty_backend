package rabbitmq

import (
	"context"
	"encoding/json"
	"log"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
)

const TaskQueueName = "ml_tasks_queue"

// Входящее сообщение от Core API
type MLTaskMessage struct {
	TaskID  string `json:"task_id"`
	Type    string `json:"type"` // "spellcheck" или "image_gen"
	Payload string `json:"payload"`
}

// MLUsecase определяет интерфейс, который ожидает наш consumer.
// Теперь он принимает taskID в формате uuid.UUID
type MLUsecase interface {
	ProcessSpellcheck(ctx context.Context, taskID uuid.UUID, text string) error
	ProcessImageGen(ctx context.Context, taskID uuid.UUID, text string) error
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

	// Объявляем очередь. Если ее нет, она создастся.
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

// Start начинает слушать очередь в бесконечном цикле
func (c *Consumer) Start(ctx context.Context) error {
	msgs, err := c.channel.Consume(
		TaskQueueName,
		"ml_worker", // consumer name
		false,       // auto-ack (отправляем подтверждение вручную)
		false,       // exclusive
		false,       // no-local
		false,       // no-wait
		nil,         // args
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
				return nil // Канал закрыт
			}

			// 1. Парсим сообщение
			var task MLTaskMessage
			if err := json.Unmarshal(msg.Body, &task); err != nil {
				log.Printf("Ошибка парсинга JSON сообщения: %v. Сообщение отклонено.", err)
				msg.Nack(false, false)
				continue
			}

			// 2. Валидируем и конвертируем UUID
			taskID, err := uuid.Parse(task.TaskID)
			if err != nil {
				log.Printf("Невалидный UUID в сообщении: %s. Сообщение отклонено.", task.TaskID)
				msg.Nack(false, false)
				continue
			}

			log.Printf("Получена задача [%s]: %s", task.Type, taskID)

			// 3. Вызываем соответствующий метод Usecase
			var processErr error
			switch task.Type {
			case "spellcheck":
				processErr = c.usecase.ProcessSpellcheck(ctx, taskID, task.Payload)
			case "image_gen":
				processErr = c.usecase.ProcessImageGen(ctx, taskID, task.Payload)
			default:
				log.Printf("Неизвестный тип задачи: %s. Сообщение отклонено.", task.Type)
			}

			// 4. Отправляем подтверждение (ACK) или отклонение (NACK)
			if processErr != nil {
				log.Printf("Ошибка обработки задачи %s: %v", taskID, processErr)
				msg.Nack(false, false)
			} else {
				log.Printf("Задача %s успешно выполнена!", taskID)
				msg.Ack(false)
			}
		}
	}
}

// Close закрывает канал
func (c *Consumer) Close() {
	if c.channel != nil {
		c.channel.Close()
	}
}
