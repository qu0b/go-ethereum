package main

import (
	"bytes"
	"errors"
	"go/types"
	"html/template"
)

type Object struct {
	Name             string
	FixedFieldLength int
	FixedFields      []FixedField
	VariableFields   []VariableField
}

func (d *Object) Encode() string {
	tmpl, err := template.New("encoder").Parse(encodeTmpl)
	if err != nil {
		panic(err)
	}
	buf := new(bytes.Buffer)
	if err = tmpl.Execute(buf, d); err != nil {
		panic(err)
	}
	return buf.String()
}

type FixedField struct {
	Name   string
	Offset int
}

type VariableField struct {
	Name   string
	Offset uint32
}

func NewObject(scope *types.Scope, name string) (*Object, error) {
	typ, err := lookup(scope, name)
	if err != nil {
		return nil, err
	}
	var (
		fixedFieldLength = 0
		fixedFields      []FixedField
		varFields        []VariableField
		currentOffset    = 0
	)

	s, ok := typ.Underlying().(*types.Struct)
	if !ok {
		panic("should never happen")
	}
	for i := 0; i < s.NumFields(); i++ {
		field := s.Field(i)
		switch field.Type() {
		case &types.Array{}:
			fallthrough
		case &types.Struct{}:
			fallthrough
		case &types.Slice{}:
			fallthrough
		case &types.Tuple{}:
			fallthrough
		case &types.Union{}:
			v := VariableField{
				Name:   field.Name(),
				Offset: uint32(currentOffset),
			}
			varFields = append(varFields, v)
			currentOffset += 4 // Offsets are 4 bytes each
		default:
			f := FixedField{
				Name:   field.Name(),
				Offset: currentOffset,
			}
			fixedFields = append(fixedFields, f)
			// TODO: fix this
			currentOffset += len(EncodeBasic(field.Type().Underlying()))
		}
	}

	return &Object{
		Name:             typ.Obj().Name(),
		FixedFieldLength: fixedFieldLength,
		FixedFields:      fixedFields,
		VariableFields:   varFields,
	}, nil
}

func lookup(scope *types.Scope, name string) (*types.Named, error) {
	obj := scope.Lookup(name)
	if obj == nil {
		return nil, errors.New("no such identifier")
	}
	typ, ok := obj.(*types.TypeName)
	if !ok {
		return nil, errors.New("not a type")
	}
	_, ok = typ.Type().Underlying().(*types.Struct)
	if !ok {
		return nil, errors.New("not a struct type")
	}
	return typ.Type().(*types.Named), nil
}
