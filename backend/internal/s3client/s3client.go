package s3client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
)

type Client struct {
	internal           *s3.Client // Used for server-side writes to S3_ENDPOINT, for example http://minio:9000.
	presign            *s3.PresignClient
	bucket             string
	publicBaseURL      string
	usePathStyle       bool
	publicEndpointHost string
}

type ObjectMeta struct {
	ContentType   string
	ContentLength int64
	ETag          string
	LastModified  time.Time
	CacheControl  string
	ContentRange  string
	AcceptRanges  string
}

type ObjectReader struct {
	ObjectMeta
	Body io.ReadCloser
}

func New(endpoint, publicEndpoint, region, accessKey, secretKey, bucket string, usePathStyle bool) (*Client, error) {
	cfg, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion(region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
	)
	if err != nil {
		return nil, err
	}
	internalBase := strings.TrimSuffix(endpoint, "/")
	internal := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(internalBase)
		o.UsePathStyle = usePathStyle
	})

	publicBase := strings.TrimSuffix(publicEndpoint, "/")
	if publicBase == "" {
		publicBase = internalBase
	}
	// Presigned uploads must be signed for the host the browser actually reaches.
	// Leaving S3_ENDPOINT as an internal Docker host such as http://minio:9000
	// would produce URLs the browser cannot access.
	publicClient := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(publicBase)
		o.UsePathStyle = usePathStyle
	})
	pe := strings.TrimPrefix(strings.TrimPrefix(publicEndpoint, "https://"), "http://")
	if pe == "" {
		pe = strings.TrimPrefix(strings.TrimPrefix(endpoint, "https://"), "http://")
	}
	return &Client{
		internal:           internal,
		presign:            s3.NewPresignClient(publicClient),
		bucket:             bucket,
		publicBaseURL:      publicBase,
		usePathStyle:       usePathStyle,
		publicEndpointHost: pe,
	}, nil
}

// PutObject stores an object through the server without requiring a direct browser PUT.
func (c *Client) PutObject(ctx context.Context, objectKey, contentType string, body io.Reader, size int64) error {
	if size <= 0 {
		return fmt.Errorf("s3 PutObject: invalid content length")
	}
	in := &s3.PutObjectInput{
		Bucket:        &c.bucket,
		Key:           &objectKey,
		Body:          body,
		ContentType:   &contentType,
		ContentLength: &size,
	}
	_, err := c.internal.PutObject(ctx, in)
	return err
}

func (c *Client) PresignPut(ctx context.Context, objectKey, contentType string, ttl time.Duration) (string, error) {
	out, err := c.presign.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket:      &c.bucket,
		Key:         &objectKey,
		ContentType: &contentType,
	}, s3.WithPresignExpires(ttl))
	if err != nil {
		return "", err
	}
	return out.URL, nil
}

func (c *Client) HeadObject(ctx context.Context, objectKey string) (ObjectMeta, error) {
	out, err := c.internal.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: &c.bucket,
		Key:    &objectKey,
	})
	if err != nil {
		return ObjectMeta{}, err
	}
	return ObjectMeta{
		ContentType:   aws.ToString(out.ContentType),
		ContentLength: aws.ToInt64(out.ContentLength),
		ETag:          aws.ToString(out.ETag),
		LastModified:  aws.ToTime(out.LastModified),
		CacheControl:  aws.ToString(out.CacheControl),
		AcceptRanges:  aws.ToString(out.AcceptRanges),
	}, nil
}

func (c *Client) GetObject(ctx context.Context, objectKey, byteRange string) (*ObjectReader, error) {
	in := &s3.GetObjectInput{
		Bucket: &c.bucket,
		Key:    &objectKey,
	}
	if strings.TrimSpace(byteRange) != "" {
		in.Range = aws.String(byteRange)
	}
	out, err := c.internal.GetObject(ctx, in)
	if err != nil {
		return nil, err
	}
	return &ObjectReader{
		ObjectMeta: ObjectMeta{
			ContentType:   aws.ToString(out.ContentType),
			ContentLength: aws.ToInt64(out.ContentLength),
			ETag:          aws.ToString(out.ETag),
			LastModified:  aws.ToTime(out.LastModified),
			CacheControl:  aws.ToString(out.CacheControl),
			ContentRange:  aws.ToString(out.ContentRange),
			AcceptRanges:  aws.ToString(out.AcceptRanges),
		},
		Body: out.Body,
	}, nil
}

func (c *Client) PublicURL(objectKey string) string {
	key := strings.TrimLeft(objectKey, "/")
	if c.usePathStyle {
		return fmt.Sprintf("%s/%s/%s", strings.TrimSuffix(c.publicBaseURL, "/"), c.bucket, key)
	}
	return fmt.Sprintf("https://%s.%s/%s", c.bucket, c.publicEndpointHost, key)
}

func IsNotFound(err error) bool {
	var apiErr smithy.APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	switch apiErr.ErrorCode() {
	case "NoSuchKey", "NotFound":
		return true
	default:
		return false
	}
}

func IsInvalidRange(err error) bool {
	var apiErr smithy.APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	return apiErr.ErrorCode() == "InvalidRange"
}
