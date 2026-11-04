package ot

import (
	"fmt"
	"unicode/utf8"
)

func Validate(op Operation) error {
	var currentBaseLength uint64
	var currentTargetLength uint64

	for _, component := range op.Components {
		switch component.kind {
		case RetainKind:
			if component.count == 0 {
				return fmt.Errorf("retain component with zero count")
			}
			currentBaseLength += uint64(component.count)
			currentTargetLength += uint64(component.count)
		case DeleteKind:
			if component.count == 0 {
				return fmt.Errorf("delete component with zero count")
			}
			currentBaseLength += uint64(component.count)
		case InsertKind:
			if component.text == "" {
				return fmt.Errorf("insert component with empty text")
			}
			if !utf8.ValidString(component.text) {
				return fmt.Errorf("insert component with invalid UTF-8")
			}
			currentTargetLength += uint64(utf8.RuneCountInString(component.text))
		default:
			return fmt.Errorf("component with invalid kind %d", component.kind)
		}
	}

	if currentBaseLength != uint64(op.BaseLength) {
		return fmt.Errorf("base length mismatch: expected %d, got %d", op.BaseLength, currentBaseLength)
	}
	if currentTargetLength != uint64(op.TargetLength) {
		return fmt.Errorf("target length mismatch: expected %d, got %d", op.TargetLength, currentTargetLength)
	}
	return nil
}
