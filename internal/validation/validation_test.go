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
	"testing"

	"github.com/stretchr/testify/assert"
)

// In order for 'go test' to run this suite, we need to create
// a normal test function and pass our suite to suite.Run
func TestValidation(t *testing.T) {
	t.Run("With NewChain", func(t *testing.T) {
		chain := New()
		assert.NotNil(t, chain)
	})
	t.Run("With new chain with options", func(t *testing.T) {
		chain := New(FailFast())
		assert.NotNil(t, chain)
		assert.True(t, chain.failFast)
		chain2 := New(AllErrors())
		assert.NotNil(t, chain2)
		assert.False(t, chain2.failFast)
	})
	t.Run("With Validator", func(t *testing.T) {
		chain := New()
		assert.NotNil(t, chain)
		assert.Empty(t, chain.validators)
		chain.AddValidator(NewBooleanValidator(true, ""))
		assert.NotEmpty(t, chain.validators)
		assert.Len(t, chain.validators, 1)
	})
	t.Run("With Assertion", func(t *testing.T) {
		chain := New()
		assert.NotNil(t, chain)
		assert.Empty(t, chain.validators)
		chain.AddAssertion(true, "")
		assert.NotEmpty(t, chain.validators)
		assert.Len(t, chain.validators, 1)
	})
	t.Run("With Validate", func(t *testing.T) {
		chain := New()
		assert.NotNil(t, chain)
		chain.AddValidator(NewEmptyStringValidator("field", ""))
		assert.Nil(t, chain.violations)
		err := chain.Validate()
		assert.NotNil(t, chain.violations)
		assert.Error(t, err)
		assert.EqualError(t, err, "the [field] is required")
	})
	t.Run("With multiple validators and FailFast option", func(t *testing.T) {
		chain := New(FailFast())
		assert.NotNil(t, chain)
		chain.
			AddValidator(NewEmptyStringValidator("field", "")).
			AddAssertion(false, "this is false")
		assert.Nil(t, chain.violations)
		err := chain.Validate()
		assert.Nil(t, chain.violations)
		assert.Error(t, err)
		assert.EqualError(t, err, "the [field] is required")
	})
	t.Run("With multiple validators and AllErrors option", func(t *testing.T) {
		chain := New(AllErrors())
		assert.NotNil(t, chain)
		chain.
			AddValidator(NewEmptyStringValidator("field", "")).
			AddAssertion(false, "this is false")
		assert.Nil(t, chain.violations)
		err := chain.Validate()
		assert.NotNil(t, chain.violations)
		assert.Error(t, err)
		assert.EqualError(t, err, "the [field] is required; this is false")
	})
}
