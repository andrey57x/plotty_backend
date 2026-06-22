package app

import (
	"context"
	"net/http"

	"github.com/fivecode/plotty/internal/infrastructure/gigachat"
	"github.com/fivecode/plotty/internal/infrastructure/languagetool"
	storage "github.com/fivecode/plotty/internal/infrastructure/minio"
	sharedrmq "github.com/fivecode/plotty/internal/infrastructure/rabbitmq"
	"github.com/fivecode/plotty/ml/config"
	"github.com/fivecode/plotty/ml/internal/adapters"
	mlhttp "github.com/fivecode/plotty/ml/internal/delivery/http"
	"github.com/fivecode/plotty/ml/internal/delivery/rabbitmq"
	"github.com/fivecode/plotty/ml/internal/repository"
	"github.com/fivecode/plotty/ml/internal/usecase"
	"github.com/jackc/pgx/v5/pgxpool"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/rs/zerolog/log"
)

type App struct {
	cfg        *config.Config
	rmqConn    *amqp.Connection
	rmqChan    *amqp.Channel
	dbPool     *pgxpool.Pool
	storage    *storage.MinioStorage
	usecase    *usecase.AIUsecase
	consumer   *rabbitmq.Consumer
	httpServer *http.Server
}

func NewApp(cfg *config.Config, rmqConn *amqp.Connection, dbPool *pgxpool.Pool) (*App, error) {
	gcClient := gigachat.NewClient(cfg.GigaChatAuthKey)
	ltClient := languagetool.NewClient(cfg.LanguageToolURL)

	st, err := storage.NewMinioStorage(
		cfg.MinioEndpoint,
		cfg.MinioUser,
		cfg.MinioPassword,
		cfg.MinioBucket,
		cfg.MinioPublicURL,
	)
	if err != nil {
		return nil, err
	}

	ltAdapter := adapters.NewLanguageToolAdapter(ltClient)

	rmqChan, err := rmqConn.Channel()
	if err != nil {
		return nil, err
	}
	if _, err := rmqChan.QueueDeclare("ml_results_queue", true, false, false, false, nil); err != nil {
		return nil, err
	}

	embClient := adapters.NewEmbeddingsClient(cfg.EmbeddingsURL)

	repo := repository.NewPostgresRepository(dbPool) // Используем созданный репозиторий
	uc := usecase.NewAIUsecase(repo, ltAdapter, gcClient, st, embClient, rmqChan)

	consumer, err := rabbitmq.NewConsumer(rmqConn)
	if err != nil {
		return nil, err
	}

	handler := mlhttp.NewHandler(repo, embClient)
	httpServer := &http.Server{
		Addr:    ":" + cfg.HTTPPort,
		Handler: handler,
	}

	return &App{
		cfg:        cfg,
		rmqConn:    rmqConn,
		rmqChan:    rmqChan,
		dbPool:     dbPool,
		storage:    st,
		consumer:   consumer,
		usecase:    uc,
		httpServer: httpServer,
	}, nil
}

func (a *App) Run(ctx context.Context) error {
	// 1. Слушаем проверки правописания
	err1 := a.consumer.StartWorker(ctx, "spellcheck_queue", rabbitmq.LoggingMiddleware(func(c context.Context, task sharedrmq.MLTaskMessage) error {
		return a.usecase.ProcessSpellcheck(c, task)
	}))
	if err1 != nil {
		return err1
	}

	// 2. Слушаем быстрые задачи
	err2 := a.consumer.StartWorker(ctx, "ml_tasks_queue", rabbitmq.LoggingMiddleware(a.usecase.ProcessMLTask))
	if err2 != nil {
		return err2
	}

	// 3. Слушаем медленные задачи картинок
	err3 := a.consumer.StartWorker(ctx, "ml_image_queue", rabbitmq.LoggingMiddleware(a.usecase.ProcessMLTask))
	if err3 != nil {
		return err3
	}

	go func() {
		log.Info().Str("port", a.cfg.HTTPPort).Msg("ML Worker: Внутренний HTTP сервер запущен")
		if err := a.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("Ошибка HTTP сервера ML")
		}
	}()

	<-ctx.Done()
	log.Info().Msg("ML Worker: Контекст отменен, завершение работы...")
	return ctx.Err()
}

func (a *App) Stop() {
	log.Info().Msg("ML Worker: Очистка ресурсов...")

	if a.httpServer != nil {
		_ = a.httpServer.Shutdown(context.Background())
	}

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
	log.Info().Msg("ML Worker: Ресурсы очищены.")
}
