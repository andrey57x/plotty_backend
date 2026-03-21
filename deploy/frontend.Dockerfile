# Этап 1: Сборка
FROM node:20-alpine AS builder
WORKDIR /app

# Устанавливаем зависимости
COPY package*.json ./
RUN npm ci

# Копируем код
COPY . .

# Прописываем переменные окружения для сборки
# (Next.js вшивает NEXT_PUBLIC переменные прямо в JS при билде)
ARG BACKEND_URL
ENV BACKEND_URL=$BACKEND_URL
ENV NEXT_PUBLIC_API_MOCKING=disabled

RUN npm run build

# Этап 2: Запуск
FROM node:20-alpine
WORKDIR /app

# Копируем только необходимые файлы из этапа сборки
COPY --from=builder /app/package*.json ./
COPY --from=builder /app/.next ./.next
COPY --from=builder /app/public ./public
COPY --from=builder /app/node_modules ./node_modules

EXPOSE 3000
# Запускаем сервер Next.js
CMD ["npm", "run", "start"]