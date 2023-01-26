package main

import (
	"bytes"
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
	Offset int
}

func NewObject() Object {
	return Object{}
}
