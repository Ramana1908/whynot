package history

import "testing"

func TestBuilder_RegisterHistory(t *testing.T) {
	h := NewBuilder("register").Init(0).
		Write(0, 1, 0, 10).
		Read(1, 0, 20, 30).
		Build()

	if h.Model != "register" || h.Init != 0 {
		t.Fatalf("model/init wrong: %+v", h)
	}
	if len(h.Ops) != 2 {
		t.Fatalf("expected 2 ops, got %d", len(h.Ops))
	}
	if h.Ops[0].ID != 0 || h.Ops[1].ID != 1 {
		t.Errorf("auto-IDs wrong: %d %d", h.Ops[0].ID, h.Ops[1].ID)
	}
	if h.Ops[0].Type != OpWrite || h.Ops[0].Value != 1 {
		t.Errorf("op 0 wrong: %+v", h.Ops[0])
	}
	if h.Ops[1].Type != OpRead || h.Ops[1].Value != 0 {
		t.Errorf("op 1 wrong: %+v", h.Ops[1])
	}
	if err := h.validate(); err != nil {
		t.Errorf("built history fails validate: %v", err)
	}
}

func TestBuilder_KVHistory(t *testing.T) {
	h := NewBuilder("kv").
		WriteKey(0, "a", 1, 0, 10).
		ReadKey(1, "a", 1, 20, 30).
		Build()

	if h.Ops[0].Key != "a" || h.Ops[1].Key != "a" {
		t.Errorf("keys not preserved: %+v", h.Ops)
	}
	if err := h.validate(); err != nil {
		t.Errorf("built KV history fails validate: %v", err)
	}
}

func TestBuilder_BuildIsIndependentOfFurtherAppends(t *testing.T) {
	b := NewBuilder("register").Write(0, 1, 0, 10)
	first := b.Build()
	b.Read(0, 1, 20, 30)
	second := b.Build()

	if len(first.Ops) != 1 {
		t.Errorf("first build mutated by later append: %d ops", len(first.Ops))
	}
	if len(second.Ops) != 2 {
		t.Errorf("second build wrong: %d ops", len(second.Ops))
	}
}
