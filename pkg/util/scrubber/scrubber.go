// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package scrubber implements support for cleaning sensitive information out of strings
// and files.
//
// Compatibility
//
// This module's API is not yet stable, and may change incompatibly from version to version.
package scrubber

import (
	"bufio"
	"bytes"
	"io"
	"os"
	"regexp"
	"strings"
)

// Replacer represents a replacement of sensitive information with a "clean" version.
type Replacer struct {
	// Regex must match the sensitive information
	Regex *regexp.Regexp
	// Hints, if given, are strings which must also be present in the text for the regexp to match.
	// Especially in single-line replacers, this can be used to limit the contexts where an otherwise
	// very broad Regex is actually replaced.
	Hints []string
	// Repl is the text to replace the substring matching Regex.  It can use the regexp package's
	// replacement characters ($1, etc.) (see regexp#Regexp.ReplaceAll).
	Repl []byte
	// ReplFunc, if set, is called with the matched bytes (see regexp#Regexp.ReplaceAllFunc). Only
	// one of Repl and ReplFunc should be set.
	ReplFunc func(b []byte) []byte
}

// ReplacerKind modifies how a Replacer is applied
type ReplacerKind int

const (
	// SingleLine indicates to Cleaner#AddReplacer that the replacer applies to
	// single lines.
	SingleLine ReplacerKind = iota
	// MultiLine indicates to Cleaner#AddReplacer that the replacer applies to
	// entire multiline text values.
	MultiLine
)

var commentRegex = regexp.MustCompile(`^\s*#.*$`)
var blankRegex = regexp.MustCompile(`^\s*$`)

// Scrubber implements support for cleaning sensitive information out of strings
// and files.  Its intended use is to "clean" data before it is logged or
// transmitted to a remote system, so that the meaning of the data remains
// clear without disclosing any sensitive information.
//
// Scrubber works by applying a set of replacers, in order.  It first applies
// all SingleLine replacers to each non-comment, non-blank line of the input.
//
// Comments and blank lines are omitted. Comments are considered to begin with `#`.
//
// It then applies all MultiLine replacers to the entire text of the input.
type Scrubber struct {
	singleLineReplacers, multiLineReplacers []Replacer
}

// New creates a new scrubber with no replacers installed.
func New() *Scrubber {
	return &Scrubber{
		singleLineReplacers: make([]Replacer, 0),
		multiLineReplacers:  make([]Replacer, 0),
	}
}

// AddReplacer adds a replacer of the given kind to the scrubber.
func (c *Scrubber) AddReplacer(kind ReplacerKind, replacer Replacer) {
	switch kind {
	case SingleLine:
		c.singleLineReplacers = append(c.singleLineReplacers, replacer)
	case MultiLine:
		c.multiLineReplacers = append(c.multiLineReplacers, replacer)
	}
}

// ScrubFile scrubs credentials from file given by pathname
func (c *Scrubber) ScrubFile(filePath string) ([]byte, error) {
	file, err := os.Open(filePath)
	defer file.Close()
	if err != nil {
		return nil, err
	}
	return c.scrubReader(file)
}

// ScrubBytes scrubs credentials from slice of bytes
func (c *Scrubber) ScrubBytes(file []byte) ([]byte, error) {
	r := bytes.NewReader(file)
	return c.scrubReader(r)
}

// ScrubLine scrubs credentials from a single line of text.  It can be safely
// applied to URLs or to strings containing URLs.  It does not run multi-line
// replacers, and should not be used on multi-line inputs.
func (c *Scrubber) ScrubLine(message string) string {
	return string(c.scrub([]byte(message), c.singleLineReplacers))
}

// scrubReader applies the cleaning algorithm to a Reader
func (c *Scrubber) scrubReader(file io.Reader) ([]byte, error) {
	var cleanedFile []byte

	scanner := bufio.NewScanner(file)

	// First, we go through the file line by line, applying any
	// single-line replacer that matches the line.
	first := true
	for scanner.Scan() {
		b := scanner.Bytes()
		if !commentRegex.Match(b) && !blankRegex.Match(b) && string(b) != "" {
			b = c.scrub(b, c.singleLineReplacers)
			if !first {
				cleanedFile = append(cleanedFile, byte('\n'))
			}

			cleanedFile = append(cleanedFile, b...)
			first = false
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	// Then we apply multiline replacers on the cleaned file
	cleanedFile = c.scrub(cleanedFile, c.multiLineReplacers)

	return cleanedFile, nil
}

// scrub applies the given replacers to the given data.
func (c *Scrubber) scrub(data []byte, replacers []Replacer) []byte {
	for _, repl := range replacers {
		containsHint := false
		for _, hint := range repl.Hints {
			if strings.Contains(string(data), hint) {
				containsHint = true
				break
			}
		}
		if len(repl.Hints) == 0 || containsHint {
			if repl.ReplFunc != nil {
				data = repl.Regex.ReplaceAllFunc(data, repl.ReplFunc)
			} else {
				data = repl.Regex.ReplaceAll(data, repl.Repl)
			}
		}
	}
	return data
}
