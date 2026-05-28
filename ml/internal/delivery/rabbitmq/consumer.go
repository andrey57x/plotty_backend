package rabbitmq

import (
	"context"
	"encoding/json"
	"time"

	sharedrmq "github.com/fivecode/plotty/internal/infrastructure/rabbitmq"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/rs/zerolog/log"
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

	log.Info().Str("queue", queueName).Msg("Воркер начал слушать очередь")

	go func() {
		for {
			select {
			case <-ctx.Done():
				log.Info().Str("queue", queueName).Msg("Остановка слушателя очереди")
				return
			case msg, ok := <-msgs:
				if !ok {
					return
				}

				var task sharedrmq.MLTaskMessage
				if err := json.Unmarshal(msg.Body, &task); err != nil {
					log.Error().Err(err).Msg("Ошибка парсинга сообщения из очереди")
					msg.Nack(false, false)
					continue
				}

				if err := handler(ctx, task); err != nil {
					log.Error().
						Err(err).
						Str("trace_id", task.TraceID).
						Str("task_id", task.TaskID).
						Str("queue", queueName).
						Msg("Ошибка обработки задачи")
					msg.Nack(false, false)
				} else {
					log.Info().
						Str("trace_id", task.TraceID).
						Str("task_id", task.TaskID).
						Msg("Задача успешно выполнена")
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
		log.Info().
			Str("trace_id", task.TraceID).
			Str("task_id", task.TaskID).
			Str("type", task.Type).
			Msg("Начинаем обработку задачи")

		start := time.Now()

		err := next(ctx, task)

		duration := time.Since(start)
		if err != nil {
			log.Error().
				Err(err).
				Str("trace_id", task.TraceID).
				Str("task_id", task.TaskID).
				Dur("duration", duration).
				Msg("Ошибка задачи")
		} else {
			log.Info().
				Str("trace_id", task.TraceID).
				Str("task_id", task.TaskID).
				Dur("duration", duration).
				Msg("Успешно завершена задача")
		}

		return err
	}
}
