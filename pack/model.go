package pack

type Model[ID comparable] struct {
	ID ID `db:"id,pk,auto_increment"`
}

func (m Model[ID]) PK() ID { return m.ID }

func (m *Model[ID]) SetPK(v ID) { m.ID = v }

func (m *Model[ID]) pkFieldAddr() any {
	return &m.ID
}

type Entity[ID comparable] interface {
	PK() ID
}

type pkAddressable interface {
	pkFieldAddr() any
}
