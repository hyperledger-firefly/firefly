// Copyright © 2025 Kaleido, Inc.
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

package tezos

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"

	"blockwatch.cc/tzgo/micheline"
	"github.com/go-resty/resty/v2"
	"github.com/hyperledger-firefly/common/pkg/config"
	"github.com/hyperledger-firefly/common/pkg/ffresty"
	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/common/pkg/i18n"
	"github.com/hyperledger-firefly/common/pkg/log"
	"github.com/hyperledger-firefly/common/pkg/retry"
	"github.com/hyperledger-firefly/common/pkg/wsclient"
	"github.com/hyperledger-firefly/firefly/internal/blockchain/common"
	"github.com/hyperledger-firefly/firefly/internal/cache"
	"github.com/hyperledger-firefly/firefly/internal/coreconfig"
	"github.com/hyperledger-firefly/firefly/internal/coremsgs"
	"github.com/hyperledger-firefly/firefly/internal/metrics"
	"github.com/hyperledger-firefly/firefly/pkg/blockchain"
	"github.com/hyperledger-firefly/firefly/pkg/core"
)

const (
	tezosTxStatusPending string = "Pending"
)

type Tezos struct {
	ctx                  context.Context
	cancelCtx            context.CancelFunc
	pluginTopic          string
	prefixShort          string
	prefixLong           string
	capabilities         *blockchain.Capabilities
	callbacks            common.BlockchainCallbacks
	client               *resty.Client
	streams              *streamManager
	streamID             string
	wsconn               wsclient.WSClient
	closed               chan struct{}
	addressResolveAlways bool
	addressResolver      *addressResolver
	metrics              metrics.Manager
	tezosconnectConf     config.Section
	subs                 common.FireflySubscriptions
	cache                cache.CInterface
	backgroundRetry      *retry.Retry
	backgroundStart      bool
}

type Location struct {
	Address string `json:"address"`
}

var batchPinEvent = "BatchPin"

type ListenerCheckpoint struct {
	Block                   int64 `json:"block"`
	TransactionBatchIndex   int64 `json:"transactionBatchIndex"`
	TransactionIndex        int64 `json:"transactionIndex"`
	MetaInternalResultIndex int64 `json:"metaInternalResultIndex"`
}

type ListenerStatus struct {
	Checkpoint ListenerCheckpoint `json:"checkpoint"`
	Catchup    bool               `json:"catchup"`
}

type subscriptionCheckpoint struct {
	Checkpoint ListenerCheckpoint `json:"checkpoint,omitempty"`
	Catchup    bool               `json:"catchup,omitempty"`
}

type TezosconnectMessageHeaders struct {
	Type string `json:"type,omitempty"`
	ID   string `json:"id,omitempty"`
}

type PayloadSchema struct {
	Type        string        `json:"type"`
	PrefixItems []*PrefixItem `json:"prefixItems"`
}

type PrefixItem struct {
	Name    string       `json:"name"`
	Type    string       `json:"type"`
	Details paramDetails `json:"details,omitempty"`
}

type paramDetails struct {
	Type           string             `json:"type"`
	InternalType   string             `json:"internalType"`
	InternalSchema fftypes.JSONObject `json:"internalSchema"`
	Kind           string             `json:"kind"`
	Variants       []string           `json:"variants"`
}
type ffiParamSchema struct {
	Type    string       `json:"type,omitempty"`
	Details paramDetails `json:"details,omitempty"`
}

type ffiMethodAndErrors struct {
	method *fftypes.FFIMethod
	errors []*fftypes.FFIError
}

type tezosWSCommandPayload struct {
	Type  string `json:"type"`
	Topic string `json:"topic,omitempty"`
}

var addressVerify = regexp.MustCompile("^(tz[1-4]|[Kk][Tt]1)[1-9A-Za-z]{33}$")

func (t *Tezos) Name() string {
	return "tezos"
}

func (t *Tezos) VerifierType() core.VerifierType {
	return core.VerifierTypeTezosAddress
}

func (t *Tezos) Init(ctx context.Context, cancelCtx context.CancelFunc, conf config.Section, metrics metrics.Manager, cacheManager cache.Manager) (err error) {
	t.InitConfig(conf)
	tezosconnectConf := t.tezosconnectConf
	addressResolverConf := conf.SubSection(AddressResolverConfigKey)

	t.ctx = log.WithLogFields(ctx, "proto", "tezos")
	t.cancelCtx = cancelCtx
	t.metrics = metrics
	t.capabilities = &blockchain.Capabilities{}
	t.callbacks = common.NewBlockchainCallbacks()
	t.subs = common.NewFireflySubscriptions()

	if addressResolverConf.GetString(AddressResolverURLTemplate) != "" {
		// Check if we need to invoke the address resolver (without caching) on every call
		t.addressResolveAlways = addressResolverConf.GetBool(AddressResolverAlwaysResolve)
		if t.addressResolver, err = newAddressResolver(ctx, addressResolverConf, cacheManager, !t.addressResolveAlways); err != nil {
			return err
		}
	}

	if tezosconnectConf.GetString(ffresty.HTTPConfigURL) == "" {
		return i18n.NewError(ctx, coremsgs.MsgMissingPluginConfig, "url", tezosconnectConf)
	}

	wsConfig, err := wsclient.GenerateConfig(ctx, tezosconnectConf)
	if err == nil {
		t.client, err = ffresty.New(t.ctx, tezosconnectConf)
	}

	if err != nil {
		return err
	}

	t.pluginTopic = tezosconnectConf.GetString(TezosconnectConfigTopic)
	if t.pluginTopic == "" {
		return i18n.NewError(ctx, coremsgs.MsgMissingPluginConfig, "topic", "blockchain.tezos.tezosconnect")
	}
	t.prefixShort = tezosconnectConf.GetString(TezosconnectPrefixShort)
	t.prefixLong = tezosconnectConf.GetString(TezosconnectPrefixLong)

	if wsConfig.WSKeyPath == "" {
		wsConfig.WSKeyPath = "/ws"
	}
	t.wsconn, err = wsclient.New(ctx, wsConfig, nil, t.afterConnect)
	if err != nil {
		return err
	}
	cache, err := cacheManager.GetCache(
		cache.NewCacheConfig(
			ctx,
			coreconfig.CacheBlockchainLimit,
			coreconfig.CacheBlockchainTTL,
			"",
		),
	)
	if err != nil {
		return err
	}
	t.cache = cache

	t.streams = newStreamManager(t.client, t.cache, t.tezosconnectConf.GetUint(TezosconnectConfigBatchSize), t.tezosconnectConf.GetDuration(TezosconnectConfigBatchTimeout).Milliseconds())

	t.backgroundStart = t.tezosconnectConf.GetBool(TezosconnectBackgroundStart)
	if t.backgroundStart {
		t.backgroundRetry = &retry.Retry{
			InitialDelay: t.tezosconnectConf.GetDuration(TezosconnectBackgroundStartInitialDelay),
			MaximumDelay: t.tezosconnectConf.GetDuration(TezosconnectBackgroundStartMaxDelay),
			Factor:       t.tezosconnectConf.GetFloat64(TezosconnectBackgroundStartFactor),
		}

		return nil
	}

	stream, err := t.streams.ensureEventStream(t.ctx, t.pluginTopic)
	if err != nil {
		return err
	}

	t.streamID = stream.ID
	log.L(t.ctx).Infof("Event stream: %s (pluginTopic=%s)", t.streamID, t.pluginTopic)

	t.closed = make(chan struct{})
	go t.eventLoop()

	return nil
}

func (t *Tezos) StartNamespace(ctx context.Context, namespace string) (err error) {
	// TODO: Implement
	return nil
}

func (t *Tezos) StopNamespace(ctx context.Context, namespace string) (err error) {
	// TODO: Implement
	return nil
}

func (t *Tezos) SetHandler(namespace string, handler blockchain.Callbacks) {
	t.callbacks.SetHandler(namespace, handler)
}

func (t *Tezos) SetOperationHandler(namespace string, handler core.OperationCallbacks) {
	t.callbacks.SetOperationalHandler(namespace, handler)
}

func (t *Tezos) Start() (err error) {
	if t.backgroundStart {
		go t.startBackgroundLoop()
		return nil
	}

	return t.wsconn.Connect()
}

func (t *Tezos) Capabilities() *blockchain.Capabilities {
	return t.capabilities
}

func (t *Tezos) AddFireflySubscription(ctx context.Context,
	namespace *core.Namespace,
	contract *blockchain.MultipartyContract,
	_ string, // Tezos lexicographically sortable protocol IDs for not yet implemented for events
) (string, error) {
	tezosLocation, err := t.parseContractLocation(ctx, contract.Location)
	if err != nil {
		return "", err
	}

	version, _ := t.GetNetworkVersion(ctx, contract.Location)

	sub, err := t.streams.ensureFireFlySubscription(ctx, namespace.Name, version, tezosLocation.Address, contract.FirstEvent, t.streamID, batchPinEvent)
	if err != nil {
		return "", err
	}

	t.subs.AddSubscription(ctx, namespace, version, sub.ID, nil)
	return sub.ID, nil
}

func (t *Tezos) RemoveFireflySubscription(ctx context.Context, subID string) {
	t.subs.RemoveSubscription(ctx, subID)
}

func (t *Tezos) ResolveSigningKey(ctx context.Context, key string, intent blockchain.ResolveKeyIntent) (resolved string, err error) {
	// Key is always required
	if key == "" {
		return "", i18n.NewError(ctx, coremsgs.MsgNodeMissingBlockchainKey)
	}
	if !t.addressResolveAlways {
		// If there's no address resolver plugin, or addressResolveAlways is false,
		// we check if it's already an tezos address - in which case we can just return it.
		resolved, err = formatTezosAddress(ctx, key)
	}
	if t.addressResolveAlways || (err != nil && t.addressResolver != nil) {
		// Either it's not a valid tezos address,
		// or we've been configured to invoke the address resolver on every call
		resolved, err = t.addressResolver.ResolveSigningKey(ctx, key, intent)
		if err == nil {
			log.L(ctx).Infof("Key '%s' resolved to '%s'", key, resolved)
			return resolved, nil
		}
	}
	return resolved, err
}

func (t *Tezos) SubmitBatchPin(ctx context.Context, nsOpID, networkNamespace, signingKey string, batch *blockchain.BatchPin, location *fftypes.JSONAny) error {
	// TODO: impl
	return nil
}

func (t *Tezos) SubmitNetworkAction(ctx context.Context, nsOpID string, signingKey string, action core.NetworkActionType, location *fftypes.JSONAny) error {
	// TODO: impl
	return nil
}

func (t *Tezos) DeployContract(ctx context.Context, nsOpID, signingKey string, definition, contract *fftypes.JSONAny, input []interface{}, options map[string]interface{}) (submissionRejected bool, err error) {
	if t.metrics.IsMetricsEnabled() {
		t.metrics.BlockchainContractDeployment()
	}
	headers := TezosconnectMessageHeaders{
		Type: core.DeployContract,
		ID:   nsOpID,
	}
	body := map[string]interface{}{
		"headers":  &headers,
		"contract": contract,
		"from":     signingKey,
	}

	body, err = t.applyOptions(ctx, body, options)
	if err != nil {
		return true, err
	}

	var resErr common.BlockchainRESTError
	res, err := t.client.R().
		SetContext(ctx).
		SetBody(body).
		SetError(&resErr).
		Post("/")
	if err != nil || !res.IsSuccess() {
		return resErr.SubmissionRejected, common.WrapRESTError(ctx, &resErr, res, err, coremsgs.MsgTezosconnectRESTErr)
	}

	return false, nil
}

func (t *Tezos) ValidateInvokeRequest(ctx context.Context, parsedMethod interface{}, input map[string]interface{}, hasMessage bool) error {
	// No additional validation beyond what is enforced by Contract Manager
	_, _, err := t.recoverFFI(ctx, parsedMethod)
	return err
}

func (t *Tezos) InvokeContract(ctx context.Context, nsOpID string, signingKey string, location *fftypes.JSONAny, parsedMethod interface{}, input map[string]interface{}, options map[string]interface{}, batch *blockchain.BatchPin) (bool, error) {
	tezosLocation, err := t.parseContractLocation(ctx, location)
	if err != nil {
		return true, err
	}

	methodName, michelsonInput, err := t.prepareRequest(ctx, parsedMethod, input)
	if err != nil {
		return true, err
	}

	// TODO: add batch pin support

	return t.invokeContractMethod(ctx, tezosLocation.Address, methodName, signingKey, nsOpID, michelsonInput, options)
}

func (t *Tezos) QueryContract(ctx context.Context, signingKey string, location *fftypes.JSONAny, parsedMethod interface{}, input map[string]interface{}, options map[string]interface{}) (interface{}, error) {
	tezosLocation, err := t.parseContractLocation(ctx, location)
	if err != nil {
		return nil, err
	}

	methodName, michelsonInput, err := t.prepareRequest(ctx, parsedMethod, input)
	if err != nil {
		return nil, err
	}

	res, err := t.queryContractMethod(ctx, tezosLocation.Address, methodName, signingKey, michelsonInput, options)
	if err != nil || !res.IsSuccess() {
		return nil, err
	}

	var output interface{}
	if err = json.Unmarshal(res.Body(), &output); err != nil {
		return nil, err
	}
	return output, nil
}

func (t *Tezos) ParseInterface(ctx context.Context, method *fftypes.FFIMethod, errors []*fftypes.FFIError) (interface{}, error) {
	return &ffiMethodAndErrors{
		method: method,
		errors: errors,
	}, nil
}

func (t *Tezos) NormalizeContractLocation(ctx context.Context, ntype blockchain.NormalizeType, location *fftypes.JSONAny) (result *fftypes.JSONAny, err error) {
	parsed, err := t.parseContractLocation(ctx, location)
	if err != nil {
		return nil, err
	}
	return t.encodeContractLocation(ctx, parsed)
}

func (t *Tezos) CheckOverlappingLocations(ctx context.Context, left *fftypes.JSONAny, right *fftypes.JSONAny) (bool, error) {
	if left == nil || right == nil {
		// No location on either side so overlapping
		// as means listening to everything
		return true, nil
	}

	parsedLeft, err := t.parseContractLocation(ctx, left)
	if err != nil {
		return false, err
	}

	parsedRight, err := t.parseContractLocation(ctx, right)
	if err != nil {
		return false, err
	}

	// For Ethereum just compared addresses
	return parsedLeft.Address == parsedRight.Address, nil
}

func (t *Tezos) StringifyContractLocation(ctx context.Context, location *fftypes.JSONAny) (string, error) {
	parsed, err := t.parseContractLocation(ctx, location)
	if err != nil {
		return "", err
	}

	return parsed.Address, nil
}

func (t *Tezos) AddContractListener(
	ctx context.Context,
	listener *core.ContractListener,
	_ string, // Tezos lexicographically sortable protocol IDs for not yet implemented for events
) (err error) {
	if len(listener.Filters) == 0 {
		return i18n.NewError(ctx, coremsgs.MsgFiltersEmpty, listener.Name)
	}

	if len(listener.Filters) > 1 {
		return i18n.NewError(ctx, coremsgs.MsgContractListenerBlockchainFilterLimit, listener.Name)
	}

	filter := listener.Filters[0]

	var location *Location
	if filter.Location != nil {
		location, err = t.parseContractLocation(ctx, filter.Location)
		if err != nil {
			return err
		}
	}

	subName := fmt.Sprintf("ff-sub-%s-%s", listener.Namespace, listener.ID)
	firstEvent := string(core.SubOptsFirstEventNewest)
	if listener.Options != nil {
		firstEvent = listener.Options.FirstEvent
	}
	result, err := t.streams.createSubscription(ctx, location, t.streamID, subName, filter.Event.Name, firstEvent)
	if err != nil {
		return err
	}
	listener.BackendID = result.ID
	return nil
}

func (t *Tezos) DeleteContractListener(ctx context.Context, subscription *core.ContractListener, okNotFound bool) error {
	return t.streams.deleteSubscription(ctx, subscription.BackendID, okNotFound)
}

// Note: In state of development. Approach can be changed.
func (t *Tezos) GetContractListenerStatus(ctx context.Context, namespace, subID string, okNotFound bool) (found bool, detail interface{}, status core.ContractListenerStatus, err error) {
	sub, err := t.streams.getSubscription(ctx, subID, okNotFound)
	if err != nil || sub == nil {
		return false, nil, core.ContractListenerStatusUnknown, err
	}

	checkpoint := &ListenerStatus{
		Catchup: sub.Catchup,
		Checkpoint: ListenerCheckpoint{
			Block:                   sub.Checkpoint.Block,
			TransactionBatchIndex:   sub.Checkpoint.TransactionBatchIndex,
			TransactionIndex:        sub.Checkpoint.TransactionIndex,
			MetaInternalResultIndex: sub.Checkpoint.MetaInternalResultIndex,
		},
	}

	// reduce checkpoint data to a single enum
	status = core.ContractListenerStatusSynced
	if sub.Catchup {
		status = core.ContractListenerStatusSyncing
	}

	return true, checkpoint, status, nil
}

func (t *Tezos) GetFFIParamValidator(ctx context.Context) (fftypes.FFIParamValidator, error) {
	// Tezosconnect does not require any additional validation beyond "JSON Schema correctness" at this time
	return nil, nil
}

func (t *Tezos) GenerateEventSignature(ctx context.Context, event *fftypes.FFIEventDefinition) (string, error) {
	return event.Name, nil
}

func (t *Tezos) GenerateEventSignatureWithLocation(ctx context.Context, event *fftypes.FFIEventDefinition, location *fftypes.JSONAny) (string, error) {
	eventSignature, _ := t.GenerateEventSignature(ctx, event)

	// No location set
	if location == nil {
		return fmt.Sprintf("*:%s", eventSignature), nil
	}

	parsed, err := t.parseContractLocation(ctx, location)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s:%s", parsed.Address, eventSignature), nil
}

func (t *Tezos) GenerateErrorSignature(ctx context.Context, event *fftypes.FFIErrorDefinition) string {
	// TODO: impl
	return ""
}

func (t *Tezos) GenerateFFI(ctx context.Context, generationRequest *fftypes.FFIGenerationRequest) (*fftypes.FFI, error) {
	return nil, i18n.NewError(ctx, coremsgs.MsgFFIGenerationUnsupported)
}

func (t *Tezos) GetNetworkVersion(ctx context.Context, location *fftypes.JSONAny) (version int, err error) {
	// Part of the FIR-12. https://github.com/hyperledger-firefly/fir/pull/12
	// Not actual for the Tezos as it's batch pin contract was after the proposal.
	// TODO: get the network version from the batch pin contract
	return 2, nil
}

func (t *Tezos) GetAndConvertDeprecatedContractConfig(ctx context.Context) (location *fftypes.JSONAny, fromBlock string, err error) {
	return nil, "", nil
}

func (t *Tezos) GetTransactionStatus(ctx context.Context, operation *core.Operation) (interface{}, error) {
	txnID := (&core.PreparedOperation{ID: operation.ID, Namespace: operation.Namespace}).NamespacedIDString()

	transactionRequestPath := fmt.Sprintf("/transactions/%s", txnID)
	client := t.client
	var resErr common.BlockchainRESTError
	var statusResponse fftypes.JSONObject
	res, err := client.R().
		SetContext(ctx).
		SetError(&resErr).
		SetResult(&statusResponse).
		Get(transactionRequestPath)
	if err != nil || !res.IsSuccess() {
		if res.StatusCode() == http.StatusNotFound {
			return nil, nil
		}
		return nil, common.WrapRESTError(ctx, &resErr, res, err, coremsgs.MsgTezosconnectRESTErr)
	}

	receiptInfo := statusResponse.GetObject("receipt")
	txStatus := statusResponse.GetString("status")

	if txStatus != "" {
		var replyType string
		if txStatus == "Succeeded" {
			replyType = common.ReceiptTransactionSuccess
		} else {
			replyType = common.ReceiptTransactionFailed
		}
		// If the status has changed, mock up blockchain receipt as if we'd received it
		// as a web socket notification
		if (operation.Status == core.OpStatusPending || operation.Status == core.OpStatusInitialized) && txStatus != tezosTxStatusPending {
			receipt := &common.BlockchainReceiptNotification{
				Headers: common.BlockchainReceiptHeaders{
					ReceiptID: statusResponse.GetString("id"),
					ReplyType: replyType,
				},
				TxHash:     statusResponse.GetString("transactionHash"),
				Message:    statusResponse.GetString("errorMessage"),
				ProtocolID: receiptInfo.GetString("protocolId")}
			err := common.HandleReceipt(ctx, operation.Namespace, t, receipt, t.callbacks)
			if err != nil {
				log.L(ctx).Warnf("Failed to handle receipt")
			}
		}
	} else {
		// Don't expect to get here so issue a warning
		log.L(ctx).Warnf("Transaction status didn't include txStatus information")
	}

	return statusResponse, nil
}

func (t *Tezos) afterConnect(ctx context.Context, w wsclient.WSClient) error {
	// Send a subscribe to our topic after each connect/reconnect
	b, _ := json.Marshal(&tezosWSCommandPayload{
		Type:  "listen",
		Topic: t.pluginTopic,
	})
	err := w.Send(ctx, b)
	if err == nil {
		b, _ = json.Marshal(&tezosWSCommandPayload{
			Type: "listenreplies",
		})
		err = w.Send(ctx, b)
	}
	return err
}

func (t *Tezos) recoverFFI(ctx context.Context, parsedMethod interface{}) (*fftypes.FFIMethod, []*fftypes.FFIError, error) {
	methodInfo, ok := parsedMethod.(*ffiMethodAndErrors)
	if !ok || methodInfo.method == nil {
		return nil, nil, i18n.NewError(ctx, coremsgs.MsgUnexpectedInterfaceType, parsedMethod)
	}
	return methodInfo.method, methodInfo.errors, nil
}

func (t *Tezos) invokeContractMethod(ctx context.Context, address, methodName, signingKey, requestID string, michelsonInput micheline.Parameters, options map[string]interface{}) (submissionRejected bool, err error) {
	if t.metrics.IsMetricsEnabled() {
		t.metrics.BlockchainTransaction(address, methodName)
	}
	messageType := "SendTransaction"
	body, err := t.buildTezosconnectRequestBody(ctx, messageType, address, methodName, signingKey, requestID, michelsonInput, options)
	if err != nil {
		return true, err
	}

	var resErr common.BlockchainRESTError
	res, err := t.client.R().
		SetContext(ctx).
		SetBody(body).
		SetError(&resErr).
		Post("/")
	if err != nil || !res.IsSuccess() {
		return resErr.SubmissionRejected, common.WrapRESTError(ctx, &resErr, res, err, coremsgs.MsgTezosconnectRESTErr)
	}
	return false, nil
}

func (t *Tezos) queryContractMethod(ctx context.Context, address, methodName, signingKey string, michelsonInput micheline.Parameters, options map[string]interface{}) (*resty.Response, error) {
	if t.metrics.IsMetricsEnabled() {
		t.metrics.BlockchainQuery(address, methodName)
	}
	messageType := "Query"
	body, err := t.buildTezosconnectRequestBody(ctx, messageType, address, methodName, signingKey, "", michelsonInput, options)
	if err != nil {
		return nil, err
	}

	var resErr common.BlockchainRESTError
	res, err := t.client.R().
		SetContext(ctx).
		SetBody(body).
		SetError(&resErr).
		Post("/")
	if err != nil || !res.IsSuccess() {
		return res, common.WrapRESTError(ctx, &resErr, res, err, coremsgs.MsgTezosconnectRESTErr)
	}
	return res, nil
}

func (t *Tezos) buildTezosconnectRequestBody(ctx context.Context, messageType, address, methodName, signingKey, requestID string, michelsonInput micheline.Parameters, options map[string]interface{}) (map[string]interface{}, error) {
	headers := TezosconnectMessageHeaders{
		Type: messageType,
	}
	if requestID != "" {
		headers.ID = requestID
	}

	body := map[string]interface{}{
		"headers": &headers,
		"to":      address,
		"method":  methodName,
		"params":  []interface{}{michelsonInput},
	}
	if signingKey != "" {
		body["from"] = signingKey
	}

	return t.applyOptions(ctx, body, options)
}

func (t *Tezos) applyOptions(ctx context.Context, body, options map[string]interface{}) (map[string]interface{}, error) {
	for k, v := range options {
		// Set the new field if it's not already set. Do not allow overriding of existing fields
		if _, ok := body[k]; !ok {
			body[k] = v
		} else {
			return nil, i18n.NewError(ctx, coremsgs.MsgOverrideExistingFieldCustomOption, k)
		}
	}
	return body, nil
}

func (t *Tezos) prepareRequest(ctx context.Context, parsedMethod interface{}, input map[string]interface{}) (string, micheline.Parameters, error) {
	method, _, err := t.recoverFFI(ctx, parsedMethod)
	if err != nil {
		return "", micheline.Parameters{}, err
	}

	// Build the payload schema for the method parameters
	prefixItems := make([]*PrefixItem, len(method.Params))
	for i, param := range method.Params {
		var paramSchema ffiParamSchema
		if err := json.Unmarshal(param.Schema.Bytes(), &paramSchema); err != nil {
			return "", micheline.Parameters{}, i18n.WrapError(ctx, err, i18n.MsgJSONObjectParseFailed, fmt.Sprintf("%s.schema", param.Name))
		}

		prefixItems[i] = &PrefixItem{
			Name:    param.Name,
			Type:    paramSchema.Type,
			Details: paramSchema.Details,
		}
	}

	payloadSchema := &PayloadSchema{
		Type:        "array",
		PrefixItems: prefixItems,
	}

	schemaBytes, _ := json.Marshal(payloadSchema)
	var processSchemaReq map[string]interface{}
	_ = json.Unmarshal(schemaBytes, &processSchemaReq)

	michelineInput, err := processArgs(processSchemaReq, input, method.Name)

	return method.Name, michelineInput, err
}

func (t *Tezos) parseContractLocation(ctx context.Context, location *fftypes.JSONAny) (*Location, error) {
	tezosLocation := Location{}
	if err := json.Unmarshal(location.Bytes(), &tezosLocation); err != nil {
		return nil, i18n.NewError(ctx, coremsgs.MsgContractLocationInvalid, err)
	}
	if tezosLocation.Address == "" {
		return nil, i18n.NewError(ctx, coremsgs.MsgContractLocationInvalid, "'address' not set")
	}
	return &tezosLocation, nil
}

func (t *Tezos) encodeContractLocation(ctx context.Context, location *Location) (result *fftypes.JSONAny, err error) {
	location.Address, err = formatTezosAddress(ctx, location.Address)
	if err != nil {
		return nil, err
	}
	normalized, err := json.Marshal(location)
	if err == nil {
		result = fftypes.JSONAnyPtrBytes(normalized)
	}
	return result, err
}

func (t *Tezos) startBackgroundLoop() {
	_ = t.backgroundRetry.Do(t.ctx, fmt.Sprintf("tezos connector %s", t.Name()), func(attempt int) (retry bool, err error) {
		stream, err := t.streams.ensureEventStream(t.ctx, t.pluginTopic)
		if err != nil {
			return true, err
		}

		t.streamID = stream.ID
		log.L(t.ctx).Infof("Event stream: %s (topic=%s)", t.streamID, t.pluginTopic)

		err = t.wsconn.Connect()
		if err != nil {
			return true, err
		}

		t.closed = make(chan struct{})
		go t.eventLoop()

		return false, nil
	})
}

func (t *Tezos) eventLoop() {
	defer t.wsconn.Close()
	defer close(t.closed)
	l := log.L(t.ctx).WithField("role", "event-loop")
	ctx := log.WithLogger(t.ctx, l)
	for {
		select {
		case <-ctx.Done():
			l.Debugf("Event loop exiting (context cancelled)")
			return
		case msgBytes, ok := <-t.wsconn.Receive():
			if !ok {
				l.Debugf("Event loop exiting (receive channel closed). Terminating server!")
				t.cancelCtx()
				return
			}

			var msgParsed interface{}
			err := json.Unmarshal(msgBytes, &msgParsed)
			if err != nil {
				l.Errorf("Message cannot be parsed as JSON: %s\n%s", err, string(msgBytes))
				continue // Swallow this and move on
			}
			switch msgTyped := msgParsed.(type) {
			case []interface{}:
				err = t.handleMessageBatch(ctx, 0, msgTyped)
				if err == nil {
					ack, _ := json.Marshal(&tezosWSCommandPayload{
						Type:  "ack",
						Topic: t.pluginTopic,
					})
					err = t.wsconn.Send(ctx, ack)
				}
			case map[string]interface{}:
				var receipt common.BlockchainReceiptNotification
				_ = json.Unmarshal(msgBytes, &receipt)

				err := common.HandleReceipt(ctx, "", t, &receipt, t.callbacks) // TODO: should be specific to a namespace
				if err != nil {
					l.Errorf("Failed to process receipt: %+v", msgTyped)
				}
			default:
				l.Errorf("Message unexpected: %+v", msgTyped)
				continue
			}

			if err != nil {
				l.Errorf("Event loop exiting (%s). Terminating server!", err)
				t.cancelCtx()
				return
			}
		}
	}
}

func (t *Tezos) handleMessageBatch(ctx context.Context, batchID int64, messages []interface{}) error {
	// TODO:
	return nil
}

func formatTezosAddress(ctx context.Context, key string) (string, error) {
	if addressVerify.MatchString(key) {
		return key, nil
	}
	return "", i18n.NewError(ctx, coremsgs.MsgInvalidTezosAddress)
}
