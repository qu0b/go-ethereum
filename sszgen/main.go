// Copyright 2023 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	var (
		pkgdir     = flag.String("dir", ".", "input package")
		output     = flag.String("out", "-", "output file (default is stdout)")
		genEncoder = flag.Bool("encoder", true, "generate EncodeSSZ?")
		genDecoder = flag.Bool("decoder", false, "generate DecodeRLP?")
		typename   = flag.String("type", "", "type to generate methods for")
	)
	flag.Parse()

	cfg := Config{
		Dir:             *pkgdir,
		Type:            *typename,
		GenerateEncoder: *genEncoder,
		GenerateDecoder: *genDecoder,
	}
	code, err := cfg.process()
	if err != nil {
		fatal(err)
	}
	if *output == "-" {
		os.Stdout.Write(code)
	} else if err := os.WriteFile(*output, code, 0600); err != nil {
		fatal(err)
	}
}

func fatal(args ...interface{}) {
	fmt.Fprintln(os.Stderr, args...)
	os.Exit(1)
}

type Config struct {
	Dir  string // input package directory
	Type string

	GenerateEncoder bool
	GenerateDecoder bool
}

// process generates the Go code.
func (cfg *Config) process() (code []byte, err error) {
	return nil, nil
}
