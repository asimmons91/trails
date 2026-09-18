package pack

// Model[ID] is the usual way to satisfy Entity[ID]: embed it by value in a
// mapped struct to get an auto-increment primary key column named "id",
// plus PK and SetPK.
type Model[ID comparable] struct {
	ID ID `db:"id,pk,auto_increment"`
}

// PK returns the model's primary key value.
func (m Model[ID]) PK() ID { return m.ID }

// SetPK sets the model's primary key value.
func (m *Model[ID]) SetPK(v ID) { m.ID = v }

func (m *Model[ID]) pkFieldAddr() any {
	return &m.ID
}

// Entity[ID] is the contract ByID, Update, Save, and Delete require: a
// mapped struct with a readable primary key. Embedding Model[ID] satisfies
// it automatically; a type may also implement PK itself instead.
type Entity[ID comparable] interface {
	PK() ID
}

// pkAddressable is implemented (via promotion) by any type embedding
// Model[ID]. returningDest uses it to write a server-generated primary key
// straight through Model[ID]'s own field, rather than resolving the
// destination via reflect.Value.FieldByIndex the way every other returned
// column is.
type pkAddressable interface {
	pkFieldAddr() any
}
