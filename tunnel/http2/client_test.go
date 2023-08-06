package http2

import (
	"testing"
)

func TestTunnelClient_Geth2Client(t *testing.T) {
	c := NewTunnelClient(":", nil)
	dst := "fakedst"
	_, untrack := c.h2Client(dst)
	untrack()
	if len(c.idleCli) == 0 {
		t.Fatalf("expected idle client")
	}

	_, _ = c.h2Client(dst)
	if len(c.idleCli) > 0 {
		t.Fatalf("unexpected idle client")
	}
}
