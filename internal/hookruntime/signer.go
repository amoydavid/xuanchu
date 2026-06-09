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

	"git.dajee.net/dajee/xuanchu/internal/storage"
)

// SignatureSHA256 按照 xuanchu webhook 签名规范计算 HMAC-SHA256。
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
// 先恢复入队时冻结的模板 headers 和事件 headers，再用运行时 headers 覆盖。
// 如果 sink 有 secret，额外添加 timestamp 和 signature 头。
func HeadersForDelivery(delivery storage.HookDelivery, hookID string, secret string, body []byte, now int64, version string) (http.Header, error) {
	headers := http.Header{}

	if delivery.RenderedHeadersJSON != "" {
		if err := mergeHeaderJSON(headers, []byte(delivery.RenderedHeadersJSON)); err != nil {
			return nil, err
		}
	}

	// 恢复存储的 headers
	if delivery.HeadersJSON != "" {
		if err := mergeHeaderJSON(headers, []byte(delivery.HeadersJSON)); err != nil {
			return nil, err
		}
	}

	// 运行时 headers 覆盖存储的
	attempt := strconv.Itoa(delivery.AttemptCount)
	contentType := delivery.RenderedContentType
	if contentType == "" {
		contentType = "application/json; charset=utf-8"
	}
	headers.Set("Content-Type", contentType)
	headers.Set("X-Xuanchu-Delivery", delivery.ID)
	headers.Set("X-Xuanchu-Hook-Id", hookID)
	headers.Set("X-Xuanchu-Attempt", attempt)
	headers.Set("User-Agent", "xuanchu-webhook/"+version)

	// 签名相关 headers（仅当 secret 存在时）
	if secret != "" {
		ts := strconv.FormatInt(now, 10)
		headers.Set("X-Xuanchu-Timestamp", ts)
		sig := SignatureSHA256(secret, delivery.ID, now, body)
		headers.Set("X-Xuanchu-Signature-256", sig)
	}

	return headers, nil
}

func mergeHeaderJSON(headers http.Header, raw []byte) error {
	var multi map[string][]string
	if err := json.Unmarshal(raw, &multi); err == nil {
		for k, values := range multi {
			headers.Del(k)
			for _, value := range values {
				headers.Add(k, value)
			}
		}
		return nil
	}
	var single map[string]string
	if err := json.Unmarshal(raw, &single); err != nil {
		return err
	}
	for k, value := range single {
		headers.Set(k, value)
	}
	return nil
}
