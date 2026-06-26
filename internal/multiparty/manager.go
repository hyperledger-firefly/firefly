// Copyright © 2024 Kaleido, Inc.
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

package multiparty

import (
	"context"
	"fmt"

	"github.com/hyperledger/firefly-common/pkg/fftypes"
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/hyperledger/firefly-common/pkg/log"
	"github.com/hyperledger/firefly/internal/coremsgs"
	"github.com/hyperledger/firefly/internal/metrics"
	"github.com/hyperledger/firefly/internal/operations"
	"github.com/hyperledger/firefly/internal/txcommon"
	"github.com/hyperledger/firefly/pkg/blockchain"
	"github.com/hyperledger/firefly/pkg/core"
	"github.com/hyperledger/firefly/pkg/database"
)

type Manager interface {
	core.Named

	// RootOrg returns configuration details for the root organization identity
	RootOrg() RootOrg

	// LocalNode returns configuration details for the local node identity
	LocalNode() LocalNode

	// ConfigureContract initializes the subscription to the FireFly contract
	// - Determines the active multiparty contract entry from the config, and updates the namespace with contract info
	// - Resolves the multiparty contract address and version, and initializes subscriptions for contract events
	ConfigureContract(ctx context.Context) (err error)

	// TerminateContract marks the given event as the last one to be parsed on the current FireFly contract
	// - Validates that the event came from the currently active multiparty contract
	// - Re-initializes the plugin against the next configured multiparty contract
	// - Updates the namespace contract info to record the point of termination and the newly active contract
	TerminateContract(ctx context.Context, location *fftypes.JSONAny, termination *blockchain.Event) (err error)

	// GetNetworkVersion returns the network version of the active FireFly contract
	GetNetworkVersion() int

	// SubmitBatchPin sequences a batch of message globally to all viewers of a given ledger
	SubmitBatchPin(ctx context.Context, batch *core.BatchPersisted, contexts []*fftypes.Bytes32, payloadRef string, idempotentSubmit bool) error

	// SubmitNetworkAction writes a special "BatchPin" event which signals the plugin to take an action
	SubmitNetworkAction(ctx context.Context, signingKey string, action *core.NetworkAction, idempotentSubmit bool) error

	// From operations.OperationHandler
	PrepareOperation(ctx context.Context, op *core.Operation) (*core.PreparedOperation, error)
	RunOperation(ctx context.Context, op *core.PreparedOperation) (outputs fftypes.JSONObject, phase core.OpPhase, err error)
}

type Config struct {
	Enabled   bool
	Org       RootOrg
	Node      LocalNode
	Contracts []blockchain.MultipartyContract
}

type RootOrg struct {
	Name        string
	Description string
	Key         string
}

type LocalNode struct {
	Name        string
	Description string
}

type multipartyManager struct {
	namespace  *core.Namespace
	database   database.Plugin
	blockchain blockchain.Plugin
	operations operations.Manager
	metrics    metrics.Manager
	txHelper   txcommon.Helper
	config     Config
}

func NewMultipartyManager(ctx context.Context, ns *core.Namespace, config Config, di database.Plugin, bi blockchain.Plugin, om operations.Manager, mm metrics.Manager, th txcommon.Helper) (Manager, error) {
	if di == nil || bi == nil || mm == nil || om == nil || th == nil {
		return nil, i18n.NewError(ctx, coremsgs.MsgInitializationNilDepError, "MultipartyManager")
	}
	mp := &multipartyManager{
		namespace:  ns,
		config:     config,
		database:   di,
		blockchain: bi,
		operations: om,
		metrics:    mm,
		txHelper:   th,
	}
	om.RegisterHandler(ctx, mp, []core.OpType{
		core.OpTypeBlockchainPinBatch,
		core.OpTypeBlockchainNetworkAction,
	})
	return mp, nil
}

func (mm *multipartyManager) Name() string {
	return "MultipartyManager"
}

func (mm *multipartyManager) RootOrg() RootOrg {
	return mm.config.Org
}

func (mm *multipartyManager) LocalNode() LocalNode {
	return mm.config.Node
}

func (mm *multipartyManager) ConfigureContract(ctx context.Context) (err error) {
	return mm.configureContractCommon(ctx, false)
}

func (mm *multipartyManager) configureContractCommon(ctx context.Context, migration bool) (err error) {
	if mm.namespace.Contracts == nil {
		mm.namespace.Contracts = &core.MultipartyContracts{}
	}
	if mm.namespace.Contracts.Active == nil {
		mm.namespace.Contracts.Active = &core.MultipartyContract{}
	}
	active := mm.namespace.Contracts.Active
	log.L(ctx).Infof("Resolving FireFly contract at index %d", active.Index)
	current, err := mm.resolveFireFlyContract(ctx, active.Index)
	if err != nil {
		return err
	}

	version, err := mm.blockchain.GetNetworkVersion(ctx, current.Location)
	if err != nil {
		return err
	}

	if !migration {
		if !active.Location.IsNil() && active.Location.String() != current.Location.String() {
			log.L(ctx).Warnf("FireFly contract location changed from %s to %s", active.Location, current.Location)
		}
	}

	// For the case that we're establishing a listener from "latest" we obtain the protocol ID
	// of the latest event confirmed from the blockchain for a given subscription.
	// This protocolID should be parsed and used by the blockchain plugin if SubOptsFirstEventNewest
	// is passed through, and the listener does not exist.
	fb := database.BlockchainEventQueryFactory.NewFilter(ctx).Sort("-protocolid").Limit(1)
	latestEvents, _, err := mm.database.GetBlockchainEvents(ctx, mm.namespace.Name, fb.Eq(
		"listener", nil,
	))
	if err != nil {
		return err
	}
	lastProtocolID := ""
	if len(latestEvents) > 0 {
		lastProtocolID = latestEvents[0].ProtocolID
	}

	subID, err := mm.blockchain.AddFireflySubscription(ctx, mm.namespace, current, lastProtocolID)
	if err == nil {
		active.Location = current.Location
		active.FirstEvent = current.FirstEvent
		active.Info.Subscription = subID
		active.Info.Version = version
		err = mm.database.UpsertNamespace(ctx, mm.namespace, true)
	}
	return err
}

func (mm *multipartyManager) resolveFireFlyContract(ctx context.Context, contractIndex int) (contract *blockchain.MultipartyContract, err error) {
	if len(mm.config.Contracts) > 0 || contractIndex > 0 {
		if contractIndex >= len(mm.config.Contracts) {
			return nil, i18n.NewError(ctx, coremsgs.MsgInvalidFireFlyContractIndex,
				fmt.Sprintf("%s.multiparty.contracts[%d]", mm.namespace.Name, contractIndex))
		}
		return &mm.config.Contracts[contractIndex], nil
	}

	// handle deprecated config here
	location, firstEvent, err := mm.blockchain.GetAndConvertDeprecatedContractConfig(ctx)
	if err != nil {
		return nil, err
	}
	return &blockchain.MultipartyContract{
		Location:   location,
		FirstEvent: firstEvent,
	}, nil
}

func (mm *multipartyManager) TerminateContract(ctx context.Context, location *fftypes.JSONAny, termination *blockchain.Event) (err error) {
	contracts := mm.namespace.Contracts
	if contracts.Active.Location.String() != location.String() {
		log.L(ctx).Warnf("Ignoring termination event from contract at '%s', which does not match active '%s'", location, contracts.Active.Location)
		return nil
	}
	log.L(ctx).Infof("Processing termination of contract #%d at '%s'", contracts.Active.Index, contracts.Active.Location)
	mm.blockchain.RemoveFireflySubscription(ctx, contracts.Active.Info.Subscription)
	contracts.Active.Info.FinalEvent = termination.ProtocolID
	contracts.Terminated = append(contracts.Terminated, contracts.Active)
	contracts.Active = &core.MultipartyContract{Index: contracts.Active.Index + 1}
	return mm.configureContractCommon(ctx, true)
}

func (mm *multipartyManager) GetNetworkVersion() int {
	return mm.namespace.Contracts.Active.Info.Version
}

func (mm *multipartyManager) SubmitNetworkAction(ctx context.Context, signingKey string, action *core.NetworkAction, idempotentSubmit bool) error {
	if action.Type != core.NetworkActionTerminate {
		return i18n.NewError(ctx, coremsgs.MsgUnrecognizedNetworkAction, action.Type)
	}

	txid, err := mm.txHelper.SubmitNewTransaction(ctx, core.TransactionTypeNetworkAction, "")
	if err != nil {
		return err
	}

	op := core.NewOperation(
		mm.blockchain,
		mm.namespace.Name,
		txid,
		core.OpTypeBlockchainNetworkAction)
	addNetworkActionInputs(op, action.Type, signingKey)
	if err := mm.operations.AddOrReuseOperation(ctx, op); err != nil {
		return err
	}

	_, err = mm.operations.RunOperation(ctx, opNetworkAction(op, action.Type, signingKey), idempotentSubmit)
	return err
}

func (mm *multipartyManager) prepareInvokeOperation(ctx context.Context, batch *core.BatchPersisted, contexts []*fftypes.Bytes32, payloadRef string) (*core.PreparedOperation, error) {
	op, err := mm.txHelper.FindOperationInTransaction(ctx, batch.TX.ID, core.OpTypeBlockchainInvoke)
	if err != nil || op == nil {
		return nil, err
	}
	req, err := txcommon.RetrieveBlockchainInvokeInputs(ctx, op)
	if err != nil {
		return nil, err
	}
	return txcommon.OpBlockchainInvoke(op, req, &txcommon.BatchPinData{
		Batch:      batch,
		Contexts:   contexts,
		PayloadRef: payloadRef,
	}), nil
}

func (mm *multipartyManager) SubmitBatchPin(ctx context.Context, batch *core.BatchPersisted, contexts []*fftypes.Bytes32, payloadRef string, idempotentSubmit bool) error {
	if batch.TX.Type == core.TransactionTypeContractInvokePin {
		preparedOp, err := mm.prepareInvokeOperation(ctx, batch, contexts, payloadRef)
		if err != nil {
			return err
		} else if preparedOp != nil {
			_, err = mm.operations.RunOperation(ctx, preparedOp, idempotentSubmit)
			return err
		}
		log.L(ctx).Warnf("No invoke operation found on transaction %s", batch.TX.ID)
	}

	op := core.NewOperation(
		mm.blockchain,
		mm.namespace.Name,
		batch.TX.ID,
		core.OpTypeBlockchainPinBatch)
	addBatchPinInputs(op, batch.ID, contexts, payloadRef)
	if err := mm.operations.AddOrReuseOperation(ctx, op); err != nil {
		return err
	}

	if mm.metrics.IsMetricsEnabled() {
		mm.metrics.CountBatchPin(mm.namespace.Name)
	}
	_, err := mm.operations.RunOperation(ctx, opBatchPin(op, batch, contexts, payloadRef), idempotentSubmit)
	return err
}
