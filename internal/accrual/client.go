package accrual

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type HTTPClient struct {
	baseURL    *url.URL
	httpClient *http.Client
	maxRetries int
}

func NewClient(rawBaseURL string, timeout time.Duration, maxRetries int) (*HTTPClient, error) {
	base := strings.TrimRight(strings.TrimSpace(rawBaseURL), "/")
	if base == "" {
		return nil, fmt.Errorf("accrual base url is empty")
	}

	u, err := url.Parse(base)
	if err != nil {
		return nil, fmt.Errorf("parse accrual base url: %w", err)
	}
	if u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("invalid accrual base url: %q", rawBaseURL)
	}
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	if maxRetries < 0 {
		maxRetries = 0
	}

	return &HTTPClient{
		baseURL: u,
		httpClient: &http.Client{
			Timeout:       timeout,
		},
		maxRetries: maxRetries,
	}, nil
}

func (cli *HTTPClient) GetOrder(ctx context.Context, number string) (Result, error) {
	orderPath := &url.URL{Path: "/api/orders/" + url.PathEscape(number)}
	orderURL := cli.baseURL.ResolveReference(orderPath).String()

	for attempt := 0; attempt <= cli.maxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, orderURL, nil)
		if err != nil {
			return Result{}, fmt.Errorf("build accrual request: %w", err)
		}

		resp, err := cli.httpClient.Do(req)
		if err != nil {
			if isRetryableNetworkError(err) && attempt < cli.maxRetries {
				continue
			}
			return Result{}, fmt.Errorf("send accrual request: %w", err)
		}

		switch resp.StatusCode {
		case http.StatusNoContent:
			_ = resp.Body.Close()
			return Result{Kind: ResultNotRegistered}, nil
		case http.StatusTooManyRequests:
			_ = resp.Body.Close()
			retryAfter, parseErr := parseRetryAfter(resp.Header.Get("Retry-After"))
			if parseErr != nil {
				return Result{}, parseErr
			}
			return Result{Kind: ResultRateLimited, RetryAfter: retryAfter}, nil
		case http.StatusOK:
			body, readErr := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if readErr != nil {
				return Result{}, fmt.Errorf("read accrual response: %w", readErr)
			}

			var payload struct {
				Status  string   `json:"status"`
				Accrual *float64 `json:"accrual"`
			}
			if err = json.Unmarshal(body, &payload); err != nil {
				return Result{}, fmt.Errorf("decode accrual response: %w", err)
			}

			switch payload.Status {
			case "REGISTERED", "PROCESSING":
				return Result{Kind: ResultProcessing}, nil
			case "INVALID":
				return Result{Kind: ResultInvalid}, nil
			case "PROCESSED":
				return Result{Kind: ResultProcessed, Accrual: payload.Accrual}, nil
			default:
				return Result{}, fmt.Errorf("unknown accrual status: %q", payload.Status)
			}
		default:
			_ = resp.Body.Close()
			return Result{}, fmt.Errorf("unexpected accrual status code: %d", resp.StatusCode)
		}
	}

	return Result{}, fmt.Errorf("exceeded retries for order %s", number)
}

// Разбираем заголовок чтобы вычленить время до ретрая
func parseRetryAfter(h string) (time.Duration, error) {
	v, err := strconv.Atoi(strings.TrimSpace(h))
	if err != nil || v <= 0 {
		return 0, fmt.Errorf("invalid retry after")
	}
	return time.Duration(v) * time.Second, nil
}

func isRetryableNetworkError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) {
		return false
	}
	var netErr net.Error
	return errors.As(err, &netErr)
}
