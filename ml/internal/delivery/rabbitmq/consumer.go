package rabbitmq

import (
	"context"
	"encoding/json"
	"log"
	"time"

	sharedrmq "github.com/fivecode/plotty/internal/infrastructure/rabbitmq"
	amqp "github.com/rabbitmq/amqp091-go"
)

type Consumer struct {
	conn    *amqp.Connection
	channel *amqp.Channel
}

func NewConsumer(conn *amqp.Connection) (*Consumer, error) {
	ch, err := conn.Channel()
	if err != nil {
		return nil, err
	}
	return &Consumer{conn: conn, channel: ch}, nil
}

func (c *Consumer) StartWorker(ctx context.Context, queueName string, handler func(context.Context, sharedrmq.MLTaskMessage) error) error {
	_, err := c.channel.QueueDeclare(queueName, true, false, false, false, nil)
	if err != nil {
		return err
	}

	if err := c.channel.Qos(1, 0, false); err != nil {
		return err
	}

	msgs, err := c.channel.Consume(queueName, "ml_worker_"+queueName, false, false, false, false, nil)
	if err != nil {
		return err
	}

	log.Printf("Воркер начал слушать очередь: %s...", queueName)

	go func() {
		for {
			select {
			case <-ctx.Done():
				log.Printf("Остановка слушателя %s...", queueName)
				return
			case msg, ok := <-msgs:
				if !ok {
					return
				}

				var task sharedrmq.MLTaskMessage
				if err := json.Unmarshal(msg.Body, &task); err != nil {
					log.Printf("Ошибка парсинга сообщения: %v", err)
					msg.Nack(false, false)
					continue
				}

				if err := handler(ctx, task); err != nil {
					log.Printf("[TraceID: %s] Ошибка обработки [%s] в %s: %v", task.TraceID, task.TaskID, queueName, err)
					msg.Nack(false, false)
				} else {
					log.Printf("[TraceID: %s] Задача [%s] успешно выполнена", task.TraceID, task.TaskID)
					msg.Ack(false)
				}
			}
		}
	}()

	return nil
}

func (c *Consumer) Close() {
	if c.channel != nil {
		c.channel.Close()
	}
}

func LoggingMiddleware(next func(context.Context, sharedrmq.MLTaskMessage) error) func(context.Context, sharedrmq.MLTaskMessage) error {
	return func(ctx context.Context, task sharedrmq.MLTaskMessage) error {
		log.Printf("[TraceID: %s] Начинаем обработку задачи [%s] типа '%s'", task.TraceID, task.TaskID, task.Type)
		start := time.Now()

		err := next(ctx, task)

		duration := time.Since(start)
		if err != nil {
			log.Printf("[TraceID: %s] Ошибка задачи [%s]: %v (выполнено за %s)", task.TraceID, task.TaskID, err, duration)
		} else {
			log.Printf("[TraceID: %s] Успешно завершена задача [%s] за %s", task.TraceID, task.TaskID, duration)
		}

		return err
	}
}
