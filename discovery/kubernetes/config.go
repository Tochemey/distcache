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

package kubernetes

import (
	"time"

	"github.com/tochemey/distcache/internal/validation"
)

// DefaultTimeout is the default timeout applied to the kubernetes API calls
// when Config.Timeout is not set.
const DefaultTimeout = 10 * time.Second

// Config defines the kubernetes discovery configuration
type Config struct {
	// Namespace specifies the kubernetes namespace
	Namespace string
	// DiscoveryPortName specifies the discovery port name
	DiscoveryPortName string
	// PodLabels specifies the pod labels
	PodLabels map[string]string
	// Timeout specifies the maximum duration of a single kubernetes API call
	// performed while discovering peers. When zero, DefaultTimeout is used.
	Timeout time.Duration
}

// Validate checks whether the given discovery configuration is valid
func (x Config) Validate() error {
	return validation.New(validation.FailFast()).
		AddValidator(validation.NewEmptyStringValidator("Namespace", x.Namespace)).
		AddValidator(validation.NewEmptyStringValidator("DiscoveryPortName", x.DiscoveryPortName)).
		AddAssertion(len(x.PodLabels) > 0, "PodLabels are required").
		AddAssertion(x.Timeout >= 0, "Timeout is invalid").
		Validate()
}

// timeout returns the effective API call timeout
func (x Config) timeout() time.Duration {
	if x.Timeout <= 0 {
		return DefaultTimeout
	}
	return x.Timeout
}
