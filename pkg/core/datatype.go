// Copyright © 2022 Kaleido, Inc.
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
	"context"

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/common/pkg/i18n"
)

type ValidatorType = fftypes.FFEnum

var (
	// ValidatorTypeJSON is the validator type for JSON Schema validation
	ValidatorTypeJSON = fftypes.FFEnumValue("validatortype", "json")
	// ValidatorTypeNone explicitly disables validation, even when a datatype is set. Allowing categorization of datatype without validation.
	ValidatorTypeNone = fftypes.FFEnumValue("validatortype", "none")
	// ValidatorTypeSystemDefinition is the validator type for system definitions
	ValidatorTypeSystemDefinition = fftypes.FFEnumValue("validatortype", "definition")
)

// Datatype is the structure defining a data definition, such as a JSON schema
type Datatype struct {
	ID        *fftypes.UUID    `ffstruct:"Datatype" json:"id,omitempty" ffexcludeinput:"true"`
	Message   *fftypes.UUID    `ffstruct:"Datatype" json:"message,omitempty" ffexcludeinput:"true"`
	Validator ValidatorType    `ffstruct:"Datatype" json:"validator" ffenum:"validatortype"`
	Namespace string           `ffstruct:"Datatype" json:"namespace,omitempty" ffexcludeinput:"true"`
	Name      string           `ffstruct:"Datatype" json:"name,omitempty"`
	Version   string           `ffstruct:"Datatype" json:"version,omitempty"`
	Hash      *fftypes.Bytes32 `ffstruct:"Datatype" json:"hash,omitempty" ffexcludeinput:"true"`
	Created   *fftypes.FFTime  `ffstruct:"Datatype" json:"created,omitempty" ffexcludeinput:"true"`
	Value     *fftypes.JSONAny `ffstruct:"Datatype" json:"value,omitempty"`
}

func (dt *Datatype) Validate(ctx context.Context, existing bool) (err error) {
	if dt.Validator != ValidatorTypeJSON {
		return i18n.NewError(ctx, i18n.MsgUnknownFieldValue, "validator", dt.Validator)
	}
	if err = fftypes.ValidateFFNameFieldNoUUID(ctx, dt.Name, "name"); err != nil {
		return err
	}
	if err = fftypes.ValidateFFNameField(ctx, dt.Version, "version"); err != nil {
		return err
	}
	if dt.Value == nil || len(*dt.Value) == 0 {
		return i18n.NewError(ctx, i18n.MsgMissingRequiredField, "value")
	}
	if existing {
		if dt.ID == nil {
			return i18n.NewError(ctx, i18n.MsgNilID)
		}
		hash := dt.Value.Hash()
		if dt.Hash == nil || *dt.Hash != *hash {
			return i18n.NewError(ctx, i18n.MsgDataInvalidHash, hash, dt.Hash)
		}
	}
	return nil
}

func (dt *Datatype) Topic() string {
	return fftypes.TypeNamespaceNameTopicHash("datatype", dt.Namespace, dt.Name)
}

func (dt *Datatype) SetBroadcastMessage(msgID *fftypes.UUID) {
	dt.Message = msgID
}
