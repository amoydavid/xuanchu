package remote

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"git.dajee.net/dajee/xuanchu/internal/auth"
)

type Client struct {
	baseURL    string
	token      string
	asUser     string
	httpClient *http.Client
}

type Options struct {
	BaseURL    string
	Token      string
	AsUser     string
	HTTPClient *http.Client
}

type apiEnvelope[T any] struct {
	Data T `json:"data"`
}

type apiErrorEnvelope struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

type APIError struct {
	Status  int
	Code    string
	Message string
}

func (e APIError) Error() string {
	if e.Code == "" {
		return e.Message
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func NewClient(opts Options) (*Client, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(opts.BaseURL), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("remote server is required")
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, APIError{Code: "remote_server_invalid", Message: "remote server URL must start with http:// or https://"}
	}
	baseURL = strings.TrimSuffix(baseURL, "/api/v1")
	token := strings.TrimSpace(opts.Token)
	if token == "" {
		return nil, fmt.Errorf("remote token is required")
	}
	// acting token 是浏览器短期委托凭证，不开放给 remote CLI / 长期自动化。
	// 这里在客户端侧拒绝 xuanchu_act_ 前缀，避免用户把浏览器委托误当机器 token。
	// 调用者绕过 CLI 直接构造 HTTP 请求时，服务端仍按普通 HTTP acting 规则处理。
	if strings.HasPrefix(token, auth.ActingTokenPrefix) {
		return nil, APIError{Code: "remote_acting_token_not_allowed", Message: "acting token is not allowed for remote CLI; use a PAT or agent token"}
	}
	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{
		baseURL:    baseURL,
		token:      token,
		asUser:     strings.TrimSpace(opts.AsUser),
		httpClient: httpClient,
	}, nil
}

func (c *Client) get(ctx context.Context, path string, values url.Values, out any) error {
	endpoint := c.baseURL + path
	if len(values) > 0 {
		endpoint += "?" + values.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	return c.do(req, out)
}

func (c *Client) post(ctx context.Context, path string, body any, out any) error {
	return c.doJSON(ctx, http.MethodPost, path, body, out)
}

func (c *Client) patch(ctx context.Context, path string, body any, out any) error {
	return c.doJSON(ctx, http.MethodPatch, path, body, out)
}

func (c *Client) delete(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	return c.do(req, out)
}

func (c *Client) doJSON(ctx context.Context, method, path string, body any, out any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return c.do(req, out)
}

func (c *Client) do(req *http.Request, out any) error {
	req.Header.Set("Authorization", "Bearer "+c.token)
	if c.asUser != "" {
		req.Header.Set("X-Xuanchu-As", c.asUser)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		var payload apiErrorEnvelope
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			return APIError{Status: resp.StatusCode, Message: resp.Status}
		}
		return APIError{Status: resp.StatusCode, Code: payload.Error.Code, Message: payload.Error.Message}
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
