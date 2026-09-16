package models

// Widget has no embedded pack.Model[...] at all — it's discovered purely
// via its TableName() method (discovery path 2).
type Widget struct {
	ID    int64  `db:"id,pk,auto_increment"`
	Label string `db:"label,not_null"`
}

func (Widget) TableName() string { return "widgets" }
