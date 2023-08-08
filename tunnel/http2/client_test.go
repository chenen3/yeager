package http2

import (
	"testing"
)

func TestTunnelClient_GetClient(t *testing.T) {
	c := NewTunnelClient(":", nil)
	dst := "fakedst"
	hc := c.getClient(dst)
	c.putClient(dst, hc)
	if len(c.idle) == 0 {
		t.Fatalf("expected idle client")
	}

	c.getClient(dst)
	if len(c.idle) > 0 {
		t.Fatalf("unexpected idle client")
	}
}
