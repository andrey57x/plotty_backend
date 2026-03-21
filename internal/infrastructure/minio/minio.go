package storage

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type MinioStorage struct {
	client          *minio.Client
	bucketName      string
	publicBaseURL   string // например http://87.239.107.15:9000 — для URL, отдаваемых клиенту
}

func NewMinioStorage(endpoint, user, password, bucket, publicBaseURL string) (*MinioStorage, error) {
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(user, password, ""),
		Secure: false, // На локальной разработке без SSL
	})
	if err != nil {
		return nil, err
	}

	return &MinioStorage{
		client:        client,
		bucketName:    bucket,
		publicBaseURL: strings.TrimSpace(publicBaseURL),
	}, nil
}

func (s *MinioStorage) Upload(ctx context.Context, fileName string, reader io.Reader, size int64, contentType string) (string, error) {
	_, err := s.client.PutObject(ctx, s.bucketName, fileName, reader, size, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return "", fmt.Errorf("ошибка загрузки в MinIO: %w", err)
	}

	return s.objectURL(fileName)
}

func (s *MinioStorage) objectURL(fileName string) (string, error) {
	if s.publicBaseURL == "" {
		return fmt.Sprintf("/%s/%s", s.bucketName, fileName), nil
	}
	base, err := url.Parse(strings.TrimSuffix(s.publicBaseURL, "/"))
	if err != nil {
		return "", fmt.Errorf("некорректный MINIO_PUBLIC_URL: %w", err)
	}
	if base.Scheme == "" || base.Host == "" {
		return "", fmt.Errorf("MINIO_PUBLIC_URL должен быть полным URL, например http://87.239.107.15:9000")
	}
	segments := []string{s.bucketName}
	for _, p := range strings.Split(fileName, "/") {
		if p != "" {
			segments = append(segments, p)
		}
	}
	return base.JoinPath(segments...).String(), nil
}
