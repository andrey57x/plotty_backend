.PHONY: up down build logs restart

# Запуск всего проекта
up:
	docker compose --env-file .env -f deploy/docker-compose.yml up -d

# Сборка и запуск (использовать, если поменял код на Go)
build:
	docker compose --env-file .env -f deploy/docker-compose.yml up -d --build

# Остановка всех контейнеров
down:
	docker compose -f deploy/docker-compose.yml down

# Посмотреть логи всех сервисов в реальном времени
logs:
	docker compose -f deploy/docker-compose.yml logs -f

# Перезапустить только фронтенд
restart-front:
	docker compose -f deploy/docker-compose.yml restart frontend