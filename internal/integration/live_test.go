//go:build live

package integration

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/knightmare2600/vogter/internal/zabbix"
)

// TestLive only reads. Run it with:
//
//	VOGTER_LIVE_URL=https://zabbix/api_jsonrpc.php VOGTER_LIVE_TOKEN=... go test -tags live ./internal/integration
func TestLive(t *testing.T) {
	url, token := os.Getenv("VOGTER_LIVE_URL"), os.Getenv("VOGTER_LIVE_TOKEN")
	if url == "" || token == "" {
		t.Skip("VOGTER_LIVE_URL and VOGTER_LIVE_TOKEN are not set")
	}
	c := zabbix.New(zabbix.Options{URL: url, VerifyTLS: os.Getenv("VOGTER_LIVE_INSECURE") == ""})
	c.SetToken(token)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	v, err := c.APIVersion(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Zabbix %s", v)
	hosts, err := c.Hosts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	problems, err := c.Problems(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.ItemsByKey(ctx, nil, []string{"system.cpu.util"}); err != nil {
		t.Fatal(err)
	}
	t.Logf("%d hosts, %d problems", len(hosts), len(problems))
	if len(hosts) == 0 {
		return
	}
	items, err := c.HostItems(ctx, hosts[0].HostID)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) > 0 {
		from := time.Now().Add(-24 * time.Hour).Unix()
		if _, err := c.Trends(ctx, []string{items[0].ItemID}, from); err != nil {
			t.Fatal(err)
		}
	}
}
