package config

import (
	"fmt"
	"log"

	"github.com/spf13/viper"
)

type Config struct {
	HTTPPort        string `mapstructure:"ML_HTTP_PORT"`
	RabbitMQURL     string `mapstructure:"RABBITMQ_URL"`
	GigaChatAuthKey string `mapstructure:"GIGACHAT_AUTH_KEY"`
	LanguageToolURL string `mapstructure:"LANGUAGETOOL_URL"`

	DBHost     string `mapstructure:"ML_DB_HOST"`
	DBPort     string `mapstructure:"ML_DB_PORT"`
	DBUser     string `mapstructure:"ML_DB_USER"`
	DBPassword string `mapstructure:"ML_DB_PASSWORD"`
	DBName     string `mapstructure:"ML_DB_NAME"`

	MinioEndpoint  string `mapstructure:"MINIO_ENDPOINT"`
	MinioPublicURL string `mapstructure:"MINIO_PUBLIC_URL"`
	MinioUser      string `mapstructure:"MINIO_ROOT_USER"`
	MinioPassword  string `mapstructure:"MINIO_ROOT_PASSWORD"`
	MinioBucket    string `mapstructure:"MINIO_BUCKET_NAME"`

	EmbeddingsURL string `mapstructure:"EMBEDDINGS_URL"`
}

func (c *Config) GetDSN() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
		c.DBUser, c.DBPassword, c.DBHost, "5432", c.DBName)
}

func Load() (*Config, error) {
	viper.SetConfigFile(".env")
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		log.Println("Не найден файл .env, используются переменные окружения ОС")
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("ошибка парсинга ML конфига: %w", err)
	}

	// if cfg.RabbitMQURL == "" {
	// 	cfg.RabbitMQURL = "amqp://guest:guest@localhost:5672/"
	// }
	// if cfg.HTTPPort == "" {
	// 	cfg.HTTPPort = "8081"
	// }

	return &cfg, nil
}
