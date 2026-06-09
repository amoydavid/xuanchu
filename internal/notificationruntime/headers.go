// Package notificationruntime 实现定时通知 delivery 的 HTTP 投递运行时。
package notificationruntime

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

// SignatureSHA256 按照 Xuanchu notification 签名规范计算 HMAC-SHA256。
func SignatureSHA256(secret, deliveryID string, timestamp int64, body []byte) string {
	if secret == "" {
		return ""
	}
	message := fmt.Sprintf("%s.%d.%s", deliveryID, timestamp, string(body))
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(message))
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func makeBaseHeaders(delivery storage.NotificationDelivery, sink storage.NotificationSink, body []byte, now int64, version string) http.Header {
	headers, err := headersForDelivery(delivery, sink, body, now, version)
	if err != nil {
		return http.Header{}
	}
	return headers
}

func headersForDelivery(delivery storage.NotificationDelivery, sink storage.NotificationSink, body []byte, now int64, version string) (http.Header, error) {
	headers := http.Header{}
	if delivery.RenderedHeadersJSON != "" {
		var stored http.Header
		if err := json.Unmarshal([]byte(delivery.RenderedHeadersJSON), &stored); err != nil {
			return nil, err
		}
		for name, values := range stored {
			for _, value := range values {
				headers.Add(name, value)
			}
		}
	}
	if delivery.RenderedContentType != "" {
		headers.Set("Content-Type", delivery.RenderedContentType)
	} else if headers.Get("Content-Type") == "" {
		headers.Set("Content-Type", "application/json; charset=utf-8")
	}

	headers.Set("X-Xuanchu-Delivery", delivery.ID)
	headers.Set("X-Xuanchu-Event", delivery.EventType)
	headers.Set("X-Xuanchu-Event-Id", delivery.EventID)
	headers.Set("X-Xuanchu-Event-Version", "1")
	headers.Set("X-Xuanchu-Attempt", strconv.Itoa(delivery.AttemptCount))
	headers.Set("User-Agent", "xuanchu-notification/"+version)

	if sink.Secret != "" {
		ts := strconv.FormatInt(now, 10)
		headers.Set("X-Xuanchu-Timestamp", ts)
		headers.Set("X-Xuanchu-Signature-256", SignatureSHA256(sink.Secret, delivery.ID, now, body))
	}
	return headers, nil
}
