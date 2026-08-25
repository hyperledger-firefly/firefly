// Copyright © 2023 Kaleido, Inc.
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package core

import (
	"crypto/tls"

	"github.com/go-resty/resty/v2"
	"github.com/hyperledger-firefly/common/pkg/fftypes"
)

// WebhookConfirmationMode is the delivery-confirmation mode for a webhook subscription, selected
// via the `confirmationMode` option. These control whether - and when - a webhook delivery causes
// the subscription checkpoint to advance. See WebhookSubOptions.ConfirmationMode.
type WebhookConfirmationMode = fftypes.FFEnum

var (
	// WebhookConfirmationModeFastAck acknowledges each event before the delivery is attempted.
	// The delivery runs detached, and its outcome never affects the subscription checkpoint.
	// This is the behavior of the deprecated `fastack: true` option.
	WebhookConfirmationModeFastAck = fftypes.FFEnumValue("webhookconfirmationmode", "fastack")
	// WebhookConfirmationModeBestEffort delivers synchronously - holding the checkpoint while any
	// client-side retries configured via WebhookRetryOptions run - then acknowledges each event once
	// that completes, regardless of the response status or any error. This is the default when
	// unset, and matches the behavior of all releases prior to the introduction of this option.
	WebhookConfirmationModeBestEffort = fftypes.FFEnumValue("webhookconfirmationmode", "besteffort")
	// WebhookConfirmationModeAssured delivers synchronously and only acknowledges on a 2xx
	// response. Anything else holds the subscription checkpoint, so the event(s) are redelivered.
	WebhookConfirmationModeAssured = fftypes.FFEnumValue("webhookconfirmationmode", "assured")
)

// WebhookConfirmationModes is the set of valid values for the `confirmationMode` option.
var WebhookConfirmationModes = []WebhookConfirmationMode{
	WebhookConfirmationModeFastAck,
	WebhookConfirmationModeBestEffort,
	WebhookConfirmationModeAssured,
}

type WebhookSubOptions struct {
	// DeprecatedFastack is retained for backwards compatibility with subscriptions created before
	// ConfirmationMode existed. When ConfirmationMode is unset, `fastack: true` resolves to
	// WebhookConfirmationModeFastAck. Setting both, with ConfirmationMode anything other than
	// "fastack", is a validation error.
	DeprecatedFastack bool `ffstruct:"WebhookSubOptions" json:"fastack,omitempty"`
	// ConfirmationMode names the delivery-confirmation behavior explicitly. Defaults to
	// WebhookConfirmationModeBestEffort when unset (and when unset with `fastack: true`,
	// to WebhookConfirmationModeFastAck).
	ConfirmationMode WebhookConfirmationMode `ffstruct:"WebhookSubOptions" json:"confirmationMode,omitempty" ffenum:"webhookconfirmationmode"`
	URL              string                  `ffstruct:"WebhookSubOptions" json:"url,omitempty"`
	Method           string                  `ffstruct:"WebhookSubOptions" json:"method,omitempty"`
	JSON             bool                    `ffstruct:"WebhookSubOptions" json:"json,omitempty"`
	Reply            bool                    `ffstruct:"WebhookSubOptions" json:"reply,omitempty"`
	ReplyTag         string                  `ffstruct:"WebhookSubOptions" json:"replytag,omitempty"`
	ReplyTX          string                  `ffstruct:"WebhookSubOptions" json:"replytx,omitempty"`
	Headers          map[string]string       `ffstruct:"WebhookSubOptions" json:"headers,omitempty"`
	Query            map[string]string       `ffstruct:"WebhookSubOptions" json:"query,omitempty"`
	TLSConfigName    string                  `ffstruct:"WebhookSubOptions" json:"tlsConfigName,omitempty"`
	TLSConfig        *tls.Config             `ffstruct:"WebhookSubOptions" json:"-" ffexcludeinput:"true"`
	Input            WebhookInputOptions     `ffstruct:"WebhookSubOptions" json:"input,omitempty"`
	Retry            WebhookRetryOptions     `ffstruct:"WebhookSubOptions" json:"retry,omitempty"`
	HTTPOptions      WebhookHTTPOptions      `ffstruct:"WebhookSubOptions" json:"httpOptions,omitempty"`
	RestyClient      *resty.Client           `ffstruct:"WebhookSubOptions" json:"-" ffexcludeinput:"true"`
}

type WebhookRetryOptions struct {
	Enabled      bool   `ffstruct:"WebhookRetryOptions" json:"enabled,omitempty"`
	Count        int    `ffstruct:"WebhookRetryOptions" json:"count,omitempty"`
	InitialDelay string `ffstruct:"WebhookRetryOptions" json:"initialDelay,omitempty"`
	MaximumDelay string `ffstruct:"WebhookRetryOptions" json:"maxDelay,omitempty"`
}

type WebhookHTTPOptions struct {
	HTTPProxyURL              *string `ffstruct:"WebhookHTTPOptions" json:"proxyURL,omitempty"`
	HTTPTLSHandshakeTimeout   string  `ffstruct:"WebhookHTTPOptions" json:"tlsHandshakeTimeout,omitempty"`
	HTTPRequestTimeout        string  `ffstruct:"WebhookHTTPOptions" json:"requestTimeout,omitempty"`
	HTTPMaxIdleConns          int     `ffstruct:"WebhookHTTPOptions" json:"maxIdleConns,omitempty"`
	HTTPIdleConnTimeout       string  `ffstruct:"WebhookHTTPOptions" json:"idleTimeout,omitempty"`
	HTTPConnectionTimeout     string  `ffstruct:"WebhookHTTPOptions" json:"connectionTimeout,omitempty"`
	HTTPExpectContinueTimeout string  `ffstruct:"WebhookHTTPOptions" json:"expectContinueTimeout,omitempty"`
}

type WebhookInputOptions struct {
	Query   string `ffstruct:"WebhookInputOptions" json:"query,omitempty"`
	Headers string `ffstruct:"WebhookInputOptions" json:"headers,omitempty"`
	Body    string `ffstruct:"WebhookInputOptions" json:"body,omitempty"`
	Path    string `ffstruct:"WebhookInputOptions" json:"path,omitempty"`
	ReplyTX string `ffstruct:"WebhookInputOptions" json:"replytx,omitempty"`
}
