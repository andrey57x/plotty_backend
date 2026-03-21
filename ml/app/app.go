package app

import (
	"context"
	"log"

	"github.com/jackc/pgx/v5/pgxpool"
	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/fivecode/plotty/internal/infrastructure/gigachat"
	storage "github.com/fivecode/plotty/internal/infrastructure/minio"
	"github.com/fivecode/plotty/ml/config"
	"github.com/fivecode/plotty/ml/internal/delivery/rabbitmq"
	"github.com/fivecode/plotty/ml/internal/repository"
	"github.com/fivecode/plotty/ml/internal/usecase"
)

type App struct {
	cfg      *config.Config
	rmqConn  *amqp.Connection
	rmqChan  *amqp.Channel // Добавлено хранение канала
	dbPool   *pgxpool.Pool
	storage  *storage.MinioStorage
	consumer *rabbitmq.Consumer
}

func NewApp(cfg *config.Config, rmqConn *amqp.Connection, dbPool *pgxpool.Pool) (*App, error) {
	// 1. Инициализируем GigaChat клиент
	gcClient := gigachat.NewClient(cfg.GigaChatAuthKey)

	// 2. Инициализируем MinIO клиент
	st, err := storage.NewMinioStorage(
		cfg.MinioEndpoint,
		cfg.MinioUser,
		cfg.MinioPassword,
		cfg.MinioBucket,
	)
	if err != nil {
		return nil, err
	}

	// 3. Создаем канал для публикации результатов из ML обратно в Core
	rmqChan, err := rmqConn.Channel()
	if err != nil {
		return nil, err
	}

	// На всякий случай декларируем очередь результатов (если ML поднимется раньше Core)
	_, err = rmqChan.QueueDeclare("ml_results_queue", true, false, false, false, nil)
	if err != nil {
		return nil, err
	}

	// 4. Инициализируем Repository и Usecase (ПЕРЕДАЕМ КАНАЛ 4-М АРГУМЕНТОМ!)
	repo := repository.NewPostgresRepository(dbPool)
	uc := usecase.NewAIUsecase(repo, gcClient, st, rmqChan)

	// 5. Инициализируем Consumer (транспорт приема задач)
	consumer, err := rabbitmq.NewConsumer(rmqConn, uc)
	if err != nil {
		return nil, err
	}

	return &App{
		cfg:      cfg,
		rmqConn:  rmqConn,
		rmqChan:  rmqChan,
		dbPool:   dbPool,
		storage:  st,
		consumer: consumer,
	}, nil
}

// Run запускает воркер и блокирует выполнение
func (a *App) Run(ctx context.Context) error {
	return a.consumer.Start(ctx)
}

// Stop красиво завершает все соединения
func (a *App) Stop() {
	log.Println("Очистка ресурсов ML сервиса...")
	if a.consumer != nil {
		a.consumer.Close()
	}
	if a.rmqChan != nil {
		a.rmqChan.Close()
	}
	if a.rmqConn != nil {
		a.rmqConn.Close()
	}
	if a.dbPool != nil {
		a.dbPool.Close()
	}
}
