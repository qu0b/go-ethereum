package main

import (
	"fmt"
	"testing"
)

func TestEncode(t *testing.T) {
	varField := VariableField{
		Name:   "variable",
		Offset: 12,
	}

	fixedFields := []FixedField{
		{
			Name:   "key",
			Offset: 0,
		},
		{
			Name:   "value",
			Offset: 8,
		},
	}

	object := Object{
		Name:             "testObj",
		FixedFieldLength: 2,
		FixedFields:      fixedFields,
		VariableFields:   []VariableField{varField},
	}

	objects := make(map[string]sszObj, 0)
	objects["one"] = newSSZObj(object)

	input := data{
		Package: "sszgenerated",
		Objects: objects,
	}
	fmt.Println(input.Encode())
}
