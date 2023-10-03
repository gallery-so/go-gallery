package store

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"net/http"

	"cloud.google.com/go/storage"
	"google.golang.org/api/googleapi"

	"github.com/mikeydub/go-gallery/util/retry"
)

var ObjAttrsOptions objectAttrsOptions

type BucketStorer struct {
	b *storage.BucketHandle
}

func NewBucketStorer(c *storage.Client, bucketName string) BucketStorer {
	return BucketStorer{c.Bucket(string(bucketName))}
}

// Exists checks if an object exists in the bucket
func (s BucketStorer) Exists(ctx context.Context, objName string) (bool, error) {
	_, err := s.Metadata(ctx, objName)
	if err != nil && err != storage.ErrObjectNotExist {
		return false, err
	}
	return err != storage.ErrObjectNotExist, nil
}

// ExistsRetry checks if an object exists, but retries on Unauthorized errors.
// While writes to GCP are strongly consistent, access is only eventually consistent: https://cloud.google.com/storage/docs/consistency.
// and can take up to several minutes to take effect.
func (s BucketStorer) ExistsRetry(ctx context.Context, objName string) (exists bool, err error) {
	retry.RetryFunc(ctx, func(ctx context.Context) error {
		exists, err = s.Exists(ctx, objName)
		return err
	}, isUnauthorizedError, retry.DefaultRetry)
	return exists, err
}

func (s BucketStorer) Metadata(ctx context.Context, objName string) (*storage.ObjectAttrs, error) {
	o := s.b.Object(objName)
	return o.Attrs(ctx)
}

func (s BucketStorer) NewReader(ctx context.Context, objName string) (*storage.Reader, error) {
	o := s.b.Object(objName)
	return o.NewReader(ctx)
}

func (s BucketStorer) NewWriter(ctx context.Context, objName string, opts ...func(*storage.ObjectAttrs)) *storage.Writer {
	o := s.b.Object(objName)
	w := o.NewWriter(ctx)
	for _, opt := range opts {
		opt(&w.ObjectAttrs)
	}
	return w
}

func (s BucketStorer) Write(ctx context.Context, objName string, b []byte, opts ...func(*storage.ObjectAttrs)) (int, error) {
	w := s.NewWriter(ctx, objName)
	defer w.Close()
	return w.Write(b)
}

func (s BucketStorer) WriteGzip(ctx context.Context, objName string, b []byte, opts ...func(*storage.ObjectAttrs)) (int, error) {
	w := s.NewWriter(ctx, objName, opts...)

	w.ObjectAttrs.ContentEncoding = "gzip"

	gz := gzip.NewWriter(w)
	buf := bytes.NewReader(b)

	n, err := io.Copy(gz, buf)
	if err != nil {
		return int(n), err
	}

	err = gz.Close()
	if err != nil {
		return int(n), err
	}

	err = w.Close()
	return int(n), err
}

type objectAttrsOptions struct{}

// WithContentType sets the Content-Type header of the object
func (objectAttrsOptions) WithContentType(typ string) func(*storage.ObjectAttrs) {
	return func(a *storage.ObjectAttrs) {
		a.ContentType = typ
	}
}

// WithCustomMetadata sets custom metadata on the object
func (objectAttrsOptions) WithCustomMetadata(m map[string]string) func(*storage.ObjectAttrs) {
	return func(a *storage.ObjectAttrs) {
		a.Metadata = m
	}
}

// WithContentEncoding sets the Content-Encoding header of the object
func (objectAttrsOptions) WithContentEncoding(enc string) func(*storage.ObjectAttrs) {
	return func(a *storage.ObjectAttrs) {
		a.ContentEncoding = enc
	}
}

func isUnauthorizedError(err error) bool {
	if gcpErr, ok := err.(*googleapi.Error); ok {
		if gcpErr.Code == http.StatusUnauthorized {
			return true
		}
	}
	return false
}
