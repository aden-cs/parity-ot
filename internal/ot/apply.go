package ot

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

func Apply(op Operation, text string) (string, error) {
	if err := Validate(op); err != nil {
		return "", err
	}
	if !utf8.ValidString(text) {
		return "", fmt.Errorf("text is not valid UTF-8")
	}

	runes := []rune(text)
	if uint64(len(runes)) != uint64(op.BaseLength) {
		return "", fmt.Errorf("text length mismatch: base length %d, text has %d runes", op.BaseLength, len(runes))
	}

	var updatedDocument strings.Builder
	var currentIndex uint32 = 0

	for _, component := range op.Components {
		switch component.kind {
		case RetainKind:
			updatedDocument.WriteString(string(runes[currentIndex : currentIndex+component.count]))
			currentIndex += component.count
		case DeleteKind:
			currentIndex += component.count
		case InsertKind:
			updatedDocument.WriteString(component.text)
		}
	}
	return updatedDocument.String(), nil
}
