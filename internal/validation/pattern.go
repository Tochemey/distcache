// MIT License
//
// Copyright (c) 2025-2026 Arsene Tochemey Gandote
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package validation

import (
	"errors"
	"fmt"
	"regexp"
)

// patternValidator is used to perform a validation
// provided a given pattern
type patternValidator struct {
	pattern    string
	expression string
	customErr  error

	// regex is compiled once at construction time
	regex *regexp.Regexp
	// compileErr holds the pattern compilation error, if any
	compileErr error
}

var _ Validator = (*patternValidator)(nil)

// NewPatternValidator creates an instance of the validator
// The given pattern should be valid regular expression
func NewPatternValidator(pattern, expression string, customErr error) Validator {
	regex, err := regexp.Compile(pattern)
	return &patternValidator{
		pattern:    pattern,
		expression: expression,
		customErr:  customErr,
		regex:      regex,
		compileErr: err,
	}
}

// Validate executes the validation
func (x *patternValidator) Validate() error {
	if x.compileErr != nil {
		return fmt.Errorf("invalid pattern=(%s): %w", x.pattern, x.compileErr)
	}

	if !x.regex.MatchString(x.expression) {
		if x.customErr != nil {
			return x.customErr
		}
		return errors.New("invalid expression")
	}
	return nil
}
