package dfspb

import "testing"

func TestServiceName(t *testing.T) {
	if n := _Dfs_serviceDesc.ServiceName; n != "Dfs" {
		t.Fatalf("service name %q, want %q", n, "Dfs")
	}
}
