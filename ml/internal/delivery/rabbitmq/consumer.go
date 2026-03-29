package rabbitmq

import (
	"context"
	"encoding/json"
	"log"

	sharedrmq "github.com/fivecode/plotty/internal/infrastructure/rabbitmq"
	"github.com/google/uuid"
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

func (c *Consumer) StartWorker(ctx context.Context, queueName string, handler func(context.Context, uuid.UUID, string, string) error) error {
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
					msg.Nack(false, false)
					continue
				}

				taskID, _ := uuid.Parse(task.TaskID)

				if err := handler(ctx, taskID, task.Type, task.Payload); err != nil {
					log.Printf("Ошибка обработки [%s] в %s: %v", taskID, queueName, err)
					msg.Nack(false, false)
				} else {
					log.Printf("Задача [%s] успешно выполнена", taskID)
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
