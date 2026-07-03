// Package directory 是 yaoguang 通讯录 API 的纯 Go 客户端，只负责 HTTP 拉取与 JSON 解析，不做持久化。
package directory

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// Member 是从 yaoguang directory 接口解析出的单个成员。
type Member struct {
	ID                 string
	Sub                string // "yaoguang_member:{id}"
	DisplayName        string
	Role               string // owner|admin|member
	Status             string // active|disabled
	ExternalIdentities []Identity
}

type Identity struct {
	Provider string // feishu|wecom|dingtalk
	Value    string // IM user_id
}

// NewClient 用给定 *http.Client 构造客户端；传 nil 则用 http.DefaultClient。
func NewClient(httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{http: httpClient}
}

type Client struct {
	http *http.Client
}

// ListMembers 拉取某 org 的全量成员（yaoguang directory API 无分页）。
func (c *Client) ListMembers(baseURL, orgID, accessToken string) ([]Member, error) {
	u := fmt.Sprintf("%s/api/orgs/%s/directory/members", strings.TrimRight(baseURL, "/"), url.PathEscape(orgID))
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("directory api status %d: %s", resp.StatusCode, string(body))
	}

	var payload struct {
		OK   bool `json:"ok"`
		Data struct {
			Members []struct {
				ID            string `json:"id"`
				Sub           string `json:"sub"`
				DisplayName   string `json:"display_name"`
				Role          string `json:"role"`
				Status        string `json:"status"`
				ExtIdentities []struct {
					Provider string `json:"provider"`
					UserType string `json:"user_type"`
					Value    string `json:"value"`
				} `json:"external_identities"`
			} `json:"members"`
		} `json:"data"`
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode directory response: %w", err)
	}
	if !payload.OK {
		return nil, fmt.Errorf("directory api error: %s %s", payload.Error.Code, payload.Error.Message)
	}

	members := make([]Member, 0, len(payload.Data.Members))
	for _, m := range payload.Data.Members {
		ext := make([]Identity, 0, len(m.ExtIdentities))
		for _, e := range m.ExtIdentities {
			if e.UserType != "user_id" {
				continue
			}
			ext = append(ext, Identity{Provider: e.Provider, Value: e.Value})
		}
		members = append(members, Member{
			ID:                 m.ID,
			Sub:                m.Sub,
			DisplayName:        m.DisplayName,
			Role:               m.Role,
			Status:             m.Status,
			ExternalIdentities: ext,
		})
	}
	return members, nil
}
