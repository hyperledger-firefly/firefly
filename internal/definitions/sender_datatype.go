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

package definitions

import (
	"context"

	"github.com/hyperledger/firefly-common/pkg/fftypes"
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/hyperledger/firefly/internal/coremsgs"
	"github.com/hyperledger/firefly/pkg/core"
)

func (ds *definitionSender) DefineDatatype(ctx context.Context, datatype *core.Datatype, waitConfirm bool) error {
	// Validate the input data definition data
	datatype.ID = fftypes.NewUUID()
	datatype.Created = fftypes.Now()
	if datatype.Validator == "" {
		datatype.Validator = core.ValidatorTypeJSON
	}
	datatype.Hash = datatype.Value.Hash()

	if ds.multiparty {
		if err := datatype.Validate(ctx, false); err != nil {
			return err
		}
		// Verify the data type is now all valid, before we broadcast it
		if err := ds.data.CheckDatatype(ctx, datatype); err != nil {
			return err
		}

		datatype.Namespace = ""
		msg, err := ds.getSenderDefault(ctx, datatype, core.SystemTagDefineDatatype, nil).send(ctx, waitConfirm)
		if msg != nil {
			datatype.Message = msg.Header.ID
		}
		datatype.Namespace = ds.namespace
		return err
	}

	return i18n.NewError(ctx, coremsgs.MsgActionNotSupported)
}
