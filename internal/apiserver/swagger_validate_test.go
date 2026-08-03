// Copyright © 2026 Kaleido, Inc.
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

package apiserver

import (
	"context"
	"errors"

	"github.com/getkin/kin-openapi/openapi3"
)

// allowedConflictingPaths lists the specific, pre-existing path pairs that
// are known to trip kin-openapi's ConflictingPathsError, and are allowed to
// keep failing that check.
var allowedConflictingPaths = [][2]string{
	{"/apis/{id}", "/apis/{apiName}"},
	{"/namespaces/{ns}/apis/{id}", "/namespaces/{ns}/apis/{apiName}"},
	{"/identities/{iid}", "/identities/{did}"},
	{"/namespaces/{ns}/identities/{iid}", "/namespaces/{ns}/identities/{did}"},
}

func isAllowedConflict(e *openapi3.ConflictingPathsError) bool {
	for _, pair := range allowedConflictingPaths {
		if (e.Path1 == pair[0] && e.Path2 == pair[1]) || (e.Path1 == pair[1] && e.Path2 == pair[0]) {
			return true
		}
	}
	return false
}

// validateOpenAPIDoc runs the standard kin-openapi validation, but tolerates
// the specific ConflictingPathsError findings listed in
// allowedConflictingPaths. Any other validation finding - including a new,
// previously-unseen conflicting path - still fails as normal.
// MultiError allows the validate to return all errors individually.
func validateOpenAPIDoc(doc *openapi3.T) error {
	err := doc.Validate(context.Background(), openapi3.EnableMultiError())
	if err == nil {
		return nil
	}

	var multi openapi3.MultiError
	if !errors.As(err, &multi) {
		multi = openapi3.MultiError{err}
	}

	var remaining openapi3.MultiError
	for _, e := range multi {
		var conflictErr *openapi3.ConflictingPathsError
		if !errors.As(e, &conflictErr) || !isAllowedConflict(conflictErr) {
			remaining = append(remaining, e)
		}
	}
	if len(remaining) == 0 {
		return nil
	}
	return remaining
}
