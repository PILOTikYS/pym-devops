package shortener

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
)

const (
	alphabet    = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	codeLength  = 7
	maxAttempts = 5
)

var (
	ErrNotFound   = errors.New("short code not found")
	ErrInvalidURL = errors.New("invalid url")
	ErrCodeTaken  = errors.New("short code already taken")
)

type Store interface {
	Save(ctx context.Context, code, rawURL string) error
	Get(ctx context.Context, code string) (string, error)
}

type MemoryStore struct {
	mu sync.RWMutex
	m  map[string]string
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{m: make(map[string]string)}
}

func (s *MemoryStore) Save(_ context.Context, code, rawURL string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.m[code]; ok {
		return ErrCodeTaken
	}
	s.m[code] = rawURL
	return nil
}

func (s *MemoryStore) Get(_ context.Context, code string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	u, ok := s.m[code]
	if !ok {
		return "", ErrNotFound
	}
	return u, nil
}

func (s *MemoryStore) Ping(context.Context) error { return nil }

type Service struct {
	store   Store
	baseURL string
}

func New(store Store, baseURL string) *Service {
	return &Service{
		store:   store,
		baseURL: strings.TrimRight(baseURL, "/"),
	}
}

type Result struct {
	Code     string
	ShortURL string
}

func (s *Service) Shorten(ctx context.Context, rawURL string) (Result, error) {
	normalized, err := normalizeURL(rawURL)
	if err != nil {
		return Result{}, err
	}

	for range maxAttempts {
		code, err := generateCode(codeLength)
		if err != nil {
			return Result{}, fmt.Errorf("generate code: %w", err)
		}
		err = s.store.Save(ctx, code, normalized)
		if err == nil {
			return Result{Code: code, ShortURL: s.baseURL + "/" + code}, nil
		}
		if errors.Is(err, ErrCodeTaken) {
			continue
		}
		return Result{}, fmt.Errorf("save: %w", err)
	}
	return Result{}, errors.New("could not allocate unique short code")
}

func (s *Service) Resolve(ctx context.Context, code string) (string, error) {
	return s.store.Get(ctx, code)
}

func normalizeURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", ErrInvalidURL
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", ErrInvalidURL
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", ErrInvalidURL
	}
	if u.Host == "" {
		return "", ErrInvalidURL
	}
	return u.String(), nil
}

func generateCode(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	out := make([]byte, n)
	for i, b := range buf {
		out[i] = alphabet[int(b)%len(alphabet)]
	}
	return string(out), nil
}
