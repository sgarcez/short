package shortservice

import (
	"context"
	"crypto/md5"
	"encoding/base64"
	"errors"
	"fmt"
	"hash"
	"sync"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/metrics"
)

// Service describes a service that generates and stores URL safe short keys for strings.
type Service interface {
	Create(ctx context.Context, v string) (string, error)
	Lookup(ctx context.Context, k string) (string, error)
}

// NewInMemService returns a memory backed Service with all of the expected middlewares wired in.
func NewInMemService(logger log.Logger, inserts, lookups metrics.Counter) Service {
	var svc Service
	{
		svc = &inMemService{m: map[string]string{}}
		svc = LoggingMiddleware(logger)(svc)
		svc = InstrumentingMiddleware(inserts, lookups)(svc)
	}
	return svc
}

var (
	// ErrMaxSizeExceeded protects the Create method.
	ErrMaxSizeExceeded = errors.New("result exceeds maximum size")
	// ErrKeyNotFound represents a missing key.
	ErrKeyNotFound = errors.New("key not found")
)

type inMemService struct {
	m map[string]string
	sync.RWMutex
}

const (
	maxLen     = 2083
	minKeySize = 6
)

// Create implements Service.
func (s *inMemService) Create(_ context.Context, v string) (string, error) {
	if len(v) > maxLen {
		return "", ErrMaxSizeExceeded
	}

	var hasher hash.Hash
	{
		hasher = md5.New()
		if _, err := hasher.Write([]byte(v)); err != nil {
			return "", fmt.Errorf("failed to write hash: %v", err)
		}
	}

	vHash := base64.RawURLEncoding.EncodeToString(hasher.Sum(nil))

	s.Lock()
	defer s.Unlock()

	size := minKeySize
	offset := 0
	for {
		// If we've scanned the encoded hash and found no available slot
		// we increase the key size and start scanning again.
		// Worst hypothetical case is a 22 character key - the full MD5 hash in base64.
		if offset+size > len(vHash) {
			size++
			offset = 0
		}
		k := vHash[offset : offset+size]

		oldv, exists := s.m[k]
		if exists {
			if oldv == v { // same value
				return k, nil
			}
			offset++ // move key window
			continue
		}

		// found slot
		s.m[k] = v
		return k, nil
	}

}

// Lookup implements Service.
func (s *inMemService) Lookup(_ context.Context, k string) (string, error) {
	if len(k) > maxLen {
		return "", ErrMaxSizeExceeded
	}

	s.Lock()
	defer s.Unlock()

	v, ok := s.m[k]
	if !ok {
		return "", ErrKeyNotFound
	}
	return v, nil
}
