package history

// Builder assembles a History programmatically with auto-incrementing op
// IDs and a fluent API. Use this when instrumenting client-side test code
// rather than emitting JSON.
//
//	h := history.NewBuilder("register").Init(0).
//	    Write(0, 1, 0, 10).         // client 0: write(1) over [0,10]
//	    Read(1, 0, 20, 30).         // client 1: read()->0 over [20,30]
//	    Build()
type Builder struct {
	model  string
	init   int
	ops    []Op
	nextID int
}

// NewBuilder starts a builder for the given model name
// ("register" | "counter" | "kv").
func NewBuilder(model string) *Builder {
	return &Builder{model: model}
}

// Init sets the initial value of the model. Defaults to 0 if not called.
func (b *Builder) Init(v int) *Builder {
	b.init = v
	return b
}

// Write appends a write op for a single-register / counter history.
func (b *Builder) Write(client, value int, callT, returnT int64) *Builder {
	b.append(Op{
		Client: client,
		Type:   OpWrite,
		Value:  value,
		Call:   callT,
		Return: returnT,
	})
	return b
}

// Read appends a read op for a single-register / counter history.
func (b *Builder) Read(client, value int, callT, returnT int64) *Builder {
	b.append(Op{
		Client: client,
		Type:   OpRead,
		Value:  value,
		Call:   callT,
		Return: returnT,
	})
	return b
}

// WriteKey appends a put op for a KV history.
func (b *Builder) WriteKey(client int, key string, value int, callT, returnT int64) *Builder {
	b.append(Op{
		Client: client,
		Type:   OpWrite,
		Key:    key,
		Value:  value,
		Call:   callT,
		Return: returnT,
	})
	return b
}

// ReadKey appends a get op for a KV history.
func (b *Builder) ReadKey(client int, key string, value int, callT, returnT int64) *Builder {
	b.append(Op{
		Client: client,
		Type:   OpRead,
		Key:    key,
		Value:  value,
		Call:   callT,
		Return: returnT,
	})
	return b
}

// Op appends a fully-specified op. The caller is responsible for op.ID;
// if zero, a fresh ID is assigned. Use this for CAS or models the
// convenience methods don't cover.
func (b *Builder) Op(op Op) *Builder {
	if op.ID == 0 && len(b.ops) > 0 {
		op.ID = b.nextID
	}
	b.append(op)
	return b
}

// Build returns the assembled History. The builder may be reused after
// Build (subsequent appends produce a new History when Build is called
// again).
func (b *Builder) Build() *History {
	ops := make([]Op, len(b.ops))
	copy(ops, b.ops)
	return &History{Model: b.model, Init: b.init, Ops: ops}
}

func (b *Builder) append(op Op) {
	op.ID = b.nextID
	b.nextID++
	b.ops = append(b.ops, op)
}
