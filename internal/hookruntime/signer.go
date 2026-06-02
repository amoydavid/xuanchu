// Package hookruntime 实现 webhook 投递的签名、请求构造和调度。
package hookruntime

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/dajee/taskg/internal/storage/sqlite"
)

// SignatureSHA256 按照 taskg webhook 签名规范计算 HMAC-SHA256。
// 签名输入: "<delivery_id>.<timestamp_unix_seconds>.<body>"
// 签名输出: "sha256=<hex>"
// 如果 secret 为空，返回空字符串。
func SignatureSHA256(secret, deliveryID string, timestamp int64, body []byte) string {
	if secret == "" {
		return ""
	}
	message := fmt.Sprintf("%s.%d.%s", deliveryID, timestamp, string(body))
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(message))
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// HeadersForDelivery 构造投递所需的 HTTP 请求头。
// 先恢复存储的 headers，再用运行时 headers 覆盖。
// 如果 hook 有 secret，额外添加 timestamp 和 signature 头。
func HeadersForDelivery(delivery sqlite.HookDelivery, hook sqlite.HookDefinition, body []byte, now int64, version string) (http.Header, error) {
	headers := http.Header{}

	// 恢复存储的 headers
	if delivery.HeadersJSON != "" {
		var stored map[string]string
		if err := json.Unmarshal([]byte(delivery.HeadersJSON), &stored); err != nil {
			return nil, err
		}
		for k, v := range stored {
			headers.Set(k, v)
		}
	}

	// 运行时 headers 覆盖存储的
	attempt := strconv.Itoa(delivery.AttemptCount)
	headers.Set("Content-Type", "application/json; charset=utf-8")
	headers.Set("X-Taskg-Delivery", delivery.ID)
	headers.Set("X-Taskg-Hook-Id", hook.ID)
	headers.Set("X-Taskg-Attempt", attempt)
	headers.Set("User-Agent", "taskg-webhook/"+version)

	// 签名相关 headers（仅当 secret 存在时）
	if hook.Secret != "" {
		ts := strconv.FormatInt(now, 10)
		headers.Set("X-Taskg-Timestamp", ts)
		sig := SignatureSHA256(hook.Secret, delivery.ID, now, body)
		headers.Set("X-Taskg-Signature-256", sig)
	}

	return headers, nil
}
