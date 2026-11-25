package pgstore

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/1995parham-learning/Pym/internal/shortener"
)

// TODO: migrate to a schema-versioning tool (e.g. golang-migrate) when the
// schema grows beyond a single table or needs ALTERs.
const schemaSQL = `CREATE TABLE IF NOT EXISTS short_urls (
	code       TEXT PRIMARY KEY,
	url        TEXT NOT NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now()
)`

const uniqueViolationCode = "23505"

type Store struct {
	pool *pgxpool.Pool
}

func New(ctx context.Context, dsn string) (*Store, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("pgxpool new: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}

	if _, err := pool.Exec(ctx, schemaSQL); err != nil {
		pool.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}

	return &Store{pool: pool}, nil
}

func (s *Store) Save(ctx context.Context, code, rawURL string) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO short_urls (code, url) VALUES ($1, $2)`, code, rawURL)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolationCode {
			return shortener.ErrCodeTaken
		}
		return err
	}
	return nil
}

func (s *Store) Get(ctx context.Context, code string) (string, error) {
	var url string
	err := s.pool.QueryRow(ctx, `SELECT url FROM short_urls WHERE code = $1`, code).Scan(&url)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", shortener.ErrNotFound
		}
		return "", err
	}
	return url, nil
}

func (s *Store) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

func (s *Store) Close() {
	s.pool.Close()
}
