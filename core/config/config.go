package config

import (
	"fmt"
	"log"

	"github.com/spf13/viper"
)

type Config struct {
	HTTPPort string `mapstructure:"HTTP_PORT"`

	AllowedOrigins string `mapstructure:"ALLOWED_ORIGINS"`

	DBHost     string `mapstructure:"DB_HOST"`
	DBPort     string `mapstructure:"DB_PORT"`
	DBUser     string `mapstructure:"DB_USER"`
	DBPassword string `mapstructure:"DB_PASSWORD"`
	DBName     string `mapstructure:"DB_NAME"`
	DBSSLMode  string `mapstructure:"DB_SSLMODE"`

	RedisHost     string `mapstructure:"REDIS_HOST"`
	RedisPort     string `mapstructure:"REDIS_PORT"`
	RedisPassword string `mapstructure:"REDIS_PASSWORD"`

	MLBaseURL string `mapstructure:"ML_BASE_URL"`

	RabbitMQURL string `mapstructure:"RABBITMQ_URL"`

	SessionDurationDays int `mapstructure:"SESSION_DURATION_DAYS"`

	MinioEndpoint  string `mapstructure:"MINIO_ENDPOINT"`
	MinioPublicURL string `mapstructure:"MINIO_PUBLIC_URL"`
	MinioUser      string `mapstructure:"MINIO_ROOT_USER"`
	MinioPassword  string `mapstructure:"MINIO_ROOT_PASSWORD"`
	MinioBucket    string `mapstructure:"MINIO_BUCKET_NAME"`
}

func Load() (*Config, error) {
	viper.SetConfigFile(".env")

	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		log.Println("Не найден файл .env, используются системные переменные окружения")
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("ошибка парсинга конфига: %w", err)
	}

	if cfg.HTTPPort == "" {
		cfg.HTTPPort = "8080"
	}

	if cfg.SessionDurationDays == 0 {
		cfg.SessionDurationDays = 30
	}

	return &cfg, nil
}

func (c *Config) GetDSN() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s",
		c.DBUser, c.DBPassword, c.DBHost, c.DBPort, c.DBName, c.DBSSLMode)
}

func (c *Config) GetRedisAddr() string {
	return fmt.Sprintf("%s:%s", c.RedisHost, c.RedisPort)
}
