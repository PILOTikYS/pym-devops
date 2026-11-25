package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"

	"github.com/1995parham-learning/Pym/internal/shortener"
)

const readyzTimeout = 500 * time.Millisecond

type Pinger interface {
	Ping(ctx context.Context) error
}

type Server struct {
	e      *echo.Echo
	svc    *shortener.Service
	pinger Pinger
}

func New(svc *shortener.Service, pinger Pinger, logger *slog.Logger) *Server {
	e := echo.New()
	e.Logger = logger

	e.Use(middleware.Recover())
	e.Use(middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		Skipper: func(c *echo.Context) bool {
			p := c.Path()
			return p == "/healthz" || p == "/readyz"
		},
		LogStatus:  true,
		LogMethod:  true,
		LogURIPath: true,
		LogLatency: true,
		LogValuesFunc: func(c *echo.Context, v middleware.RequestLoggerValues) error {
			logger.LogAttrs(c.Request().Context(), slog.LevelInfo, "request",
				slog.String("method", v.Method),
				slog.String("path", v.URIPath),
				slog.Int("status", v.Status),
				slog.Duration("latency", v.Latency),
			)
			return nil
		},
	}))

	s := &Server{e: e, svc: svc, pinger: pinger}

	e.GET("/healthz", s.handleHealthz)
	e.GET("/readyz", s.handleReadyz)
	e.POST("/api/v1/shorten", s.handleShorten)
	e.GET("/:code", s.handleRedirect)

	return s
}

func (s *Server) Run(ctx context.Context, addr string, shutdownTimeout time.Duration) error {
	cfg := echo.StartConfig{
		Address:         addr,
		GracefulTimeout: shutdownTimeout,
		HideBanner:      true,
		HidePort:        true,
	}
	return cfg.Start(ctx, s.e)
}

func (s *Server) handleHealthz(c *echo.Context) error {
	return c.NoContent(http.StatusOK)
}

func (s *Server) handleReadyz(c *echo.Context) error {
	ctx, cancel := context.WithTimeout(c.Request().Context(), readyzTimeout)
	defer cancel()
	if err := s.pinger.Ping(ctx); err != nil {
		return c.NoContent(http.StatusServiceUnavailable)
	}
	return c.NoContent(http.StatusOK)
}

type shortenRequest struct {
	URL string `json:"url"`
}

type shortenResponse struct {
	Code     string `json:"code"`
	ShortURL string `json:"short_url"`
}

func (s *Server) handleShorten(c *echo.Context) error {
	var req shortenRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	res, err := s.svc.Shorten(c.Request().Context(), req.URL)
	if err != nil {
		if errors.Is(err, shortener.ErrInvalidURL) {
			return echo.NewHTTPError(http.StatusBadRequest, "url must be a valid http(s) url")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to shorten url")
	}

	return c.JSON(http.StatusCreated, shortenResponse{
		Code:     res.Code,
		ShortURL: res.ShortURL,
	})
}

func (s *Server) handleRedirect(c *echo.Context) error {
	code := c.Param("code")
	target, err := s.svc.Resolve(c.Request().Context(), code)
	if err != nil {
		if errors.Is(err, shortener.ErrNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "short code not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to resolve short code")
	}
	return c.Redirect(http.StatusFound, target)
}
