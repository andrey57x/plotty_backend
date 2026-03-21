package app

import (
	"context"
	"log"

	"github.com/jackc/pgx/v5/pgxpool"
	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/fivecode/plotty/internal/infrastructure/gigachat"
	"github.com/fivecode/plotty/internal/infrastructure/minio"
	"github.com/fivecode/plotty/ml/config"
	"github.com/fivecode/plotty/ml/internal/delivery/rabbitmq"
	"github.com/fivecode/plotty/ml/internal/repository"
	"github.com/fivecode/plotty/ml/internal/usecase"
)

type App struct {
	cfg      *config.Config
	rmqConn  *amqp.Connection
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

	// 3. Инициализируем Repository (работа с базой)
	repo := repository.NewPostgresRepository(dbPool)

	// 4. Инициализируем Usecase (бизнес-логика)
	uc := usecase.NewAIUsecase(repo, gcClient, st)

	// 5. Инициализируем Consumer (транспорт)
	consumer, err := rabbitmq.NewConsumer(rmqConn, uc)
	if err != nil {
		return nil, err
	}

	return &App{
		cfg:      cfg,
		rmqConn:  rmqConn,
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
	if a.rmqConn != nil {
		a.rmqConn.Close()
	}
	if a.dbPool != nil {
		a.dbPool.Close()
	}
}
