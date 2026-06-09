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

func SignatureSHA256(secret, deliveryID string, timestamp int64, body []byte) string {
	if secret == "" {
		return ""
	}
	message := fmt.Sprintf("%s.%d.%s", deliveryID, timestamp, string(body))
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(message))
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func HeadersForDelivery(delivery storage.NotificationDelivery, sink storage.NotificationSink, body []byte, now int64, version string) (http.Header, error) {
	headers := http.Header{}
	if delivery.RenderedHeadersJSON != "" {
		var stored map[string]string
		if err := json.Unmarshal([]byte(delivery.RenderedHeadersJSON), &stored); err != nil {
			return nil, err
		}
		for k, v := range stored {
			headers.Set(k, v)
		}
	}
	if delivery.RenderedContentType != "" {
		headers.Set("Content-Type", delivery.RenderedContentType)
	} else {
		headers.Set("Content-Type", "application/json")
	}
	headers.Set("X-Xuanchu-Delivery", delivery.ID)
	headers.Set("X-Xuanchu-Event", delivery.EventType)
	headers.Set("X-Xuanchu-Event-Id", delivery.EventID)
	headers.Set("X-Xuanchu-Event-Version", "1")
	headers.Set("X-Xuanchu-Timestamp", strconv.FormatInt(now, 10))
	headers.Set("User-Agent", "xuanchu-notification/"+version)
	if sink.Type == "webhook" && sink.Secret != "" {
		headers.Set("X-Xuanchu-Signature-256", SignatureSHA256(sink.Secret, delivery.ID, now, body))
	}
	return headers, nil
}
