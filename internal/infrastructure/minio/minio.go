package storage

import (
	"context"
	"fmt"
	"io"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type MinioStorage struct {
	client     *minio.Client
	bucketName string
}

func NewMinioStorage(endpoint, user, password, bucket string) (*MinioStorage, error) {
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(user, password, ""),
		Secure: false, // На локальной разработке без SSL
	})
	if err != nil {
		return nil, err
	}

	return &MinioStorage{
		client:     client,
		bucketName: bucket,
	}, nil
}

func (s *MinioStorage) Upload(ctx context.Context, fileName string, reader io.Reader, size int64, contentType string) (string, error) {
	_, err := s.client.PutObject(ctx, s.bucketName, fileName, reader, size, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return "", fmt.Errorf("ошибка загрузки в MinIO: %w", err)
	}

	// Формируем URL для доступа (в продакшене лучше использовать пре-подписанные URL или CDN)
	return fmt.Sprintf("/%s/%s", s.bucketName, fileName), nil
}
