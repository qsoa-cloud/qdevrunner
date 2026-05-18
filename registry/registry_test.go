package registry

import (
	"testing"
)

type mockInstance struct {
	name      string
	grpcAddr  string
	grpcReady bool
	httpAddr  string
	httpReady bool
}

func (m *mockInstance) GetName() string              { return m.name }
func (m *mockInstance) GetGrpcToServiceAddr() string  { return m.grpcAddr }
func (m *mockInstance) IsGrpcReady() bool             { return m.grpcReady }
func (m *mockInstance) GetHttpToServiceAddr() string   { return m.httpAddr }
func (m *mockInstance) IsHttpReady() bool              { return m.httpReady }

func TestRegisterAndListAll(t *testing.T) {
	r := New()
	r.Register(&mockInstance{name: "svc-a"})
	r.Register(&mockInstance{name: "svc-b"})

	names := r.ListAll()
	if len(names) != 2 {
		t.Fatalf("expected 2 services, got %d", len(names))
	}
}

func TestUnregister(t *testing.T) {
	r := New()
	inst := &mockInstance{name: "svc-a"}
	r.Register(inst)
	r.Unregister(inst)

	names := r.ListAll()
	if len(names) != 0 {
		t.Fatalf("expected 0 services after unregister, got %d", len(names))
	}
}

func TestUnregisterNonexistent(t *testing.T) {
	r := New()
	inst := &mockInstance{name: "svc-a"}
	r.Unregister(inst) // should not panic
}

func TestGetGrpcInstance(t *testing.T) {
	r := New()
	r.Register(&mockInstance{name: "svc-a", grpcAddr: "/tmp/grpc.sock", grpcReady: true})

	inst, err := r.GetGrpcInstance("svc-a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inst.GetName() != "svc-a" {
		t.Errorf("expected svc-a, got %s", inst.GetName())
	}
}

func TestGetGrpcInstanceNotFound(t *testing.T) {
	r := New()

	_, err := r.GetGrpcInstance("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent service")
	}
}

func TestGetGrpcInstanceNotReady(t *testing.T) {
	r := New()
	r.Register(&mockInstance{name: "svc-a", grpcReady: false})

	_, err := r.GetGrpcInstance("svc-a")
	if err == nil {
		t.Fatal("expected error for not-ready service")
	}
}

func TestGetHttpInstance(t *testing.T) {
	r := New()
	r.Register(&mockInstance{name: "svc-a", httpReady: true})

	inst, err := r.GetHttpInstance("svc-a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inst.GetName() != "svc-a" {
		t.Errorf("expected svc-a, got %s", inst.GetName())
	}
}

func TestGetHttpInstanceNotReady(t *testing.T) {
	r := New()
	r.Register(&mockInstance{name: "svc-a", httpReady: false})

	_, err := r.GetHttpInstance("svc-a")
	if err == nil {
		t.Fatal("expected error for not-ready HTTP service")
	}
}

func TestMultipleInstances(t *testing.T) {
	r := New()
	r.Register(&mockInstance{name: "svc-a", grpcReady: true})
	r.Register(&mockInstance{name: "svc-a", grpcReady: true})

	// Should return one of them.
	inst, err := r.GetGrpcInstance("svc-a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inst.GetName() != "svc-a" {
		t.Errorf("expected svc-a")
	}

	names := r.ListAll()
	if len(names) != 1 {
		t.Errorf("expected 1 unique name, got %d", len(names))
	}
}

func TestUnregisterOneOfMultiple(t *testing.T) {
	r := New()
	inst1 := &mockInstance{name: "svc-a", grpcReady: true}
	inst2 := &mockInstance{name: "svc-a", grpcReady: true}
	r.Register(inst1)
	r.Register(inst2)

	r.Unregister(inst1)

	// Should still find svc-a.
	_, err := r.GetGrpcInstance("svc-a")
	if err != nil {
		t.Fatalf("expected to find svc-a after partial unregister: %v", err)
	}
}
