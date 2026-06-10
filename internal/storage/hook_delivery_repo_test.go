package storage

import (
	"strings"
	"testing"
)

func TestHookDeliveryClaimDueSQLAvoidsPostgresUntypedMax(t *testing.T) {
	sql := hookClaimExpiresAtSQL()
	if strings.Contains(sql, "MAX(?") {
		t.Fatalf("claim expiry SQL contains untyped MAX parameter: %s", sql)
	}
	if !strings.Contains(sql, "CAST(? AS BIGINT)") || !strings.Contains(sql, "CASE") {
		t.Fatalf("claim expiry SQL should cast bind params and use CASE: %s", sql)
	}
}
