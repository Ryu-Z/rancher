package terminalaudit

import (
	"context"
	"errors"
	"io"
	"path"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
)

type s3ReplayStore struct {
	client   *s3.S3
	uploader *s3manager.Uploader
	bucket   string
	sse      string
}

func NewS3ReplayStore(cfg S3Config) (ReplayStore, error) {
	if cfg.Bucket == "" {
		return nil, errors.New("s3 bucket is required")
	}

	awsCfg := aws.NewConfig().
		WithRegion(cfg.Region).
		WithS3ForcePathStyle(cfg.ForcePathStyle)
	if cfg.Endpoint != "" {
		awsCfg = awsCfg.WithEndpoint(cfg.Endpoint)
	}
	if cfg.AccessKey != "" || cfg.SecretKey != "" {
		awsCfg = awsCfg.WithCredentials(credentials.NewStaticCredentials(cfg.AccessKey, cfg.SecretKey, ""))
	}

	sess, err := session.NewSession(awsCfg)
	if err != nil {
		return nil, err
	}

	client := s3.New(sess)
	return &s3ReplayStore{
		client:   client,
		uploader: s3manager.NewUploaderWithClient(client),
		bucket:   cfg.Bucket,
		sse:      cfg.ServerSideEnc,
	}, nil
}

func (s *s3ReplayStore) Upload(key string, contentType string, body io.Reader) error {
	input := &s3manager.UploadInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(cleanObjectKey(key)),
		Body:   body,
	}
	if contentType != "" {
		input.ContentType = aws.String(contentType)
	}
	if s.sse != "" {
		input.ServerSideEncryption = aws.String(s.sse)
	}
	_, err := s.uploader.UploadWithContext(context.Background(), input)
	return err
}

func (s *s3ReplayStore) Download(key string) (io.ReadCloser, string, error) {
	out, err := s.client.GetObjectWithContext(context.Background(), &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(cleanObjectKey(key)),
	})
	if err != nil {
		return nil, "", err
	}
	contentType := ""
	if out.ContentType != nil {
		contentType = *out.ContentType
	}
	return out.Body, contentType, nil
}

func (s *s3ReplayStore) Presign(key string, ttl time.Duration) (string, error) {
	req, _ := s.client.GetObjectRequest(&s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(cleanObjectKey(key)),
	})
	return req.Presign(ttl)
}

func (s *s3ReplayStore) Exists(key string) (bool, error) {
	_, err := s.client.HeadObjectWithContext(context.Background(), &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(cleanObjectKey(key)),
	})
	if err == nil {
		return true, nil
	}
	if awsErr, ok := err.(awserr.RequestFailure); ok && awsErr.StatusCode() == 404 {
		return false, nil
	}
	return false, err
}

func (s *s3ReplayStore) Type() string {
	return replayStorageS3
}

func replayObjectKey(prefix, date, sessionID, filename string) string {
	prefix = strings.Trim(prefix, "/")
	if prefix == "" {
		prefix = defaultReplayPrefix
	}
	date = strings.TrimSpace(date)
	if date == "" {
		date = time.Now().UTC().Format("2006-01-02")
	}
	return cleanObjectKey(path.Join(prefix, date, sanitizePathSegment(sessionID), sanitizePathSegment(filename)))
}

func cleanObjectKey(key string) string {
	key = strings.TrimSpace(strings.ReplaceAll(key, "\\", "/"))
	key = strings.Trim(key, "/")
	return path.Clean(key)
}

func sanitizePathSegment(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, "\\", "_")
	value = strings.ReplaceAll(value, "/", "_")
	value = strings.ReplaceAll(value, "..", "_")
	return value
}
