package ot

type Kind uint8

const (
	InvalidKind Kind = iota
	RetainKind
	DeleteKind
	InsertKind
)

type Component struct {
	kind  Kind
	count uint32
	text  string
}

func Retain(count uint32) Component {
	return Component{
		kind:  RetainKind,
		count: count,
	}
}

func Delete(count uint32) Component {
	return Component{
		kind:  DeleteKind,
		count: count,
	}
}

func Insert(text string) Component {
	return Component{
		kind: InsertKind,
		text: text,
	}
}

type Operation struct {
	Components   []Component
	BaseLength   uint32
	TargetLength uint32
}