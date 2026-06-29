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

package fabric

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/go-resty/resty/v2"
	"github.com/hyperledger-firefly/common/pkg/config"
	"github.com/hyperledger-firefly/common/pkg/ffresty"
	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/common/pkg/i18n"
	"github.com/hyperledger-firefly/common/pkg/log"
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
	broadcastBatchEventName = "BatchPin"
)

type Fabric struct {
	ctx            context.Context
	cancelCtx      context.CancelFunc
	pluginTopic    string
	defaultChannel string
	signer         string
	prefixShort    string
	prefixLong     string
	capabilities   *blockchain.Capabilities
	callbacks      common.BlockchainCallbacks
	client         *resty.Client
	streams        *streamManager
	streamID       map[string]string
	idCache        map[string]*fabIdentity
	wsconn         map[string]wsclient.WSClient
	wsConfig       *wsclient.WSConfig
	closed         map[string]chan struct{}
	metrics        metrics.Manager
	fabconnectConf config.Section
	subs           common.FireflySubscriptions
	cache          cache.CInterface
}

type eventStreamWebsocket struct {
	Topic string `json:"topic"`
}

type fabTxInputHeaders struct {
	ID            string         `json:"id,omitempty"`
	Type          string         `json:"type"`
	PayloadSchema *PayloadSchema `json:"payloadSchema,omitempty"`
	Signer        string         `json:"signer,omitempty"`
	Channel       string         `json:"channel,omitempty"`
	Chaincode     string         `json:"chaincode,omitempty"`
}

type PayloadSchema struct {
	Type        string        `json:"type"`
	PrefixItems []*PrefixItem `json:"prefixItems"`
}

type PrefixItem struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type fabQueryNamedOutput struct {
	Headers *fabTxInputHeaders `json:"headers"`
	Result  interface{}        `json:"result"`
}

type ffiParamSchema struct {
	Type string `json:"type,omitempty"`
}

type fabWSCommandPayload struct {
	Type  string `json:"type"`
	Topic string `json:"topic,omitempty"`
}

type fabIdentity struct {
	MSPID  string `json:"mspId"`
	ECert  string `json:"enrollmentCert"`
	CACert string `json:"caCert"`
}

type Location struct {
	Channel   string `json:"channel"`
	Chaincode string `json:"chaincode"`
}

type ContractOptions struct {
	CustomPinSupport bool `json:"customPinSupport"`
}

var batchPinEvent = "BatchPin"
var batchPinMethodName = "PinBatch"
var networkActionMethodName = "NetworkAction"
var batchPinPrefixItemsV1 = []*PrefixItem{
	{
		Name: "namespace",
		Type: "string",
	},
	{
		Name: "uuids",
		Type: "string",
	},
	{
		Name: "batchHash",
		Type: "string",
	},
	{
		Name: "payloadRef",
		Type: "string",
	},
	{
		Name: "contexts",
		Type: "string",
	},
}
var batchPinPrefixItems = []*PrefixItem{
	{
		Name: "uuids",
		Type: "string",
	},
	{
		Name: "batchHash",
		Type: "string",
	},
	{
		Name: "payloadRef",
		Type: "string",
	},
	{
		Name: "contexts",
		Type: "string",
	},
}
var networkActionPrefixItems = []*PrefixItem{
	{
		Name: "action",
		Type: "string",
	},
	{
		Name: "payload",
		Type: "string",
	},
}
var networkVersionMethodName = "NetworkVersion"

var fullIdentityPattern = regexp.MustCompile(".+::x509::(.+)::.+")

var cnPattern = regexp.MustCompile("CN=([^,]+)")

func (f *Fabric) Name() string {
	return "fabric"
}

func (f *Fabric) VerifierType() core.VerifierType {
	return core.VerifierTypeMSPIdentity
}

func (f *Fabric) Init(ctx context.Context, cancelCtx context.CancelFunc, conf config.Section, metrics metrics.Manager, cacheManager cache.Manager) (err error) {
	f.InitConfig(conf)
	fabconnectConf := f.fabconnectConf

	f.ctx = log.WithLogField(ctx, "proto", "fabric")
	f.cancelCtx = cancelCtx
	f.idCache = make(map[string]*fabIdentity)
	f.metrics = metrics
	f.capabilities = &blockchain.Capabilities{}
	f.callbacks = common.NewBlockchainCallbacks()
	f.subs = common.NewFireflySubscriptions()

	if fabconnectConf.GetString(ffresty.HTTPConfigURL) == "" {
		return i18n.NewError(ctx, coremsgs.MsgMissingPluginConfig, "url", "blockchain.fabric.fabconnect")
	}

	f.wsConfig, err = wsclient.GenerateConfig(ctx, fabconnectConf)
	if err == nil {
		f.client, err = ffresty.New(f.ctx, fabconnectConf)
	}

	if err != nil {
		return err
	}

	f.defaultChannel = fabconnectConf.GetString(FabconnectConfigDefaultChannel)
	// the org identity is guaranteed to be configured by the core
	f.signer = fabconnectConf.GetString(FabconnectConfigSigner)
	f.pluginTopic = fabconnectConf.GetString(FabconnectConfigTopic)
	if f.pluginTopic == "" {
		return i18n.NewError(ctx, coremsgs.MsgMissingPluginConfig, "topic", "blockchain.fabric.fabconnect")
	}
	f.prefixShort = fabconnectConf.GetString(FabconnectPrefixShort)
	f.prefixLong = fabconnectConf.GetString(FabconnectPrefixLong)

	if f.wsConfig.WSKeyPath == "" {
		f.wsConfig.WSKeyPath = "/ws"
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
	f.cache = cache

	f.streamID = make(map[string]string)
	f.closed = make(map[string]chan struct{})
	f.wsconn = make(map[string]wsclient.WSClient)
	f.streams = newStreamManager(f.client, f.signer, f.cache, f.fabconnectConf.GetUint(FabconnectConfigBatchSize), f.fabconnectConf.GetDuration(FabconnectConfigBatchTimeout).Milliseconds())

	return nil
}

func (f *Fabric) getTopic(namespace string) string {
	return fmt.Sprintf("%s/%s", f.pluginTopic, namespace)
}

func (f *Fabric) StartNamespace(ctx context.Context, namespace string) (err error) {
	log.L(f.ctx).Debugf("Starting namespace: %s", namespace)
	topic := f.getTopic(namespace)

	f.wsconn[namespace], err = wsclient.New(ctx, f.wsConfig, nil, func(ctx context.Context, w wsclient.WSClient) error {
		// Send a subscribe to our topic after each connect/reconnect
		b, _ := json.Marshal(&fabWSCommandPayload{
			Type:  "listen",
			Topic: topic,
		})
		err := w.Send(ctx, b)
		if err == nil {
			b, _ = json.Marshal(&fabWSCommandPayload{
				Type: "listenreplies",
			})
			err = w.Send(ctx, b)
		}
		return err
	})
	if err != nil {
		return err
	}
	// Make sure that our event stream is in place
	stream, err := f.streams.ensureEventStream(ctx, topic, f.pluginTopic)
	if err != nil {
		return err
	}
	log.L(f.ctx).Infof("Event stream: %s (topic=%s)", stream.ID, topic)
	f.streamID[namespace] = stream.ID

	err = f.wsconn[namespace].Connect()
	if err != nil {
		return err
	}

	f.closed[namespace] = make(chan struct{})

	go f.eventLoop(namespace, f.wsconn[namespace], f.closed[namespace])

	return nil
}

func (f *Fabric) StopNamespace(ctx context.Context, namespace string) (err error) {
	wsconn, ok := f.wsconn[namespace]
	if ok {
		wsconn.Close()
	}
	delete(f.wsconn, namespace)
	delete(f.streamID, namespace)
	delete(f.closed, namespace)

	return nil
}

func (f *Fabric) SetHandler(namespace string, handler blockchain.Callbacks) {
	f.callbacks.SetHandler(namespace, handler)
}

func (f *Fabric) SetOperationHandler(namespace string, handler core.OperationCallbacks) {
	f.callbacks.SetOperationalHandler(namespace, handler)
}

func (f *Fabric) Capabilities() *blockchain.Capabilities {
	return f.capabilities
}

func decodeJSONPayload(ctx context.Context, payloadString string) *fftypes.JSONObject {
	bytes, err := base64.StdEncoding.DecodeString(payloadString)
	if err != nil {
		log.L(ctx).Errorf("BatchPin event is not valid - bad payload content: %s", payloadString)
		return nil
	}
	dataBytes := fftypes.JSONAnyPtrBytes(bytes)
	payload, ok := dataBytes.JSONObjectOk()
	if !ok {
		log.L(ctx).Errorf("BatchPin event is not valid - bad JSON payload: %s", bytes)
		return nil
	}
	return &payload
}

func (f *Fabric) parseBlockchainEvent(ctx context.Context, msgJSON fftypes.JSONObject) *blockchain.Event {
	payloadString := msgJSON.GetString("payload")
	payload := decodeJSONPayload(ctx, payloadString)
	if payload == nil {
		return nil // move on
	}

	// Fabric events are dispatched by the underlying fabric client to FabConnect with just the block number
	// and the transaction hash. The index of the transaction in the block, or the index of the action within
	// the transaction are not available. So we cannot generate an alphanumerically sortable string
	// into the protocol ID. Instead we can only do this (which is according to Fabric rules assured to be
	// unique, as Fabric only allows one event per transaction):
	sTransactionHash := msgJSON.GetString("transactionId")
	blockNumber := msgJSON.GetInt64("blockNumber")
	protocolID := fmt.Sprintf("%.12d/%s", blockNumber, sTransactionHash)

	name := msgJSON.GetString("eventName")
	timestamp := msgJSON.GetInt64("timestamp")
	chaincode := msgJSON.GetString("chaincodeId")

	delete(msgJSON, "payload")
	return &blockchain.Event{
		BlockchainTXID: sTransactionHash,
		Source:         f.Name(),
		Name:           name,
		ProtocolID:     protocolID,
		Output:         *payload,
		Info:           msgJSON,
		Timestamp:      fftypes.UnixTime(timestamp),
		Location:       f.buildEventLocationString(chaincode),
		Signature:      name,
	}
}

func (f *Fabric) processBatchPinEvent(ctx context.Context, events common.EventsToDispatch, location *fftypes.JSONAny, subInfo *common.SubscriptionInfo, msgJSON fftypes.JSONObject) {
	event := f.parseBlockchainEvent(ctx, msgJSON)
	if event == nil {
		return // move on
	}

	signer := event.Output.GetString("signer")
	nsOrAction := event.Output.GetString("namespace")
	params := &common.BatchPinParams{
		UUIDs:      event.Output.GetString("uuids"),
		BatchHash:  event.Output.GetString("batchHash"),
		PayloadRef: event.Output.GetString("payloadRef"),
		Contexts:   event.Output.GetStringArray("contexts"),
		NsOrAction: nsOrAction,
	}

	verifier := &core.VerifierRef{
		Type:  core.VerifierTypeMSPIdentity,
		Value: signer,
	}

	f.callbacks.PrepareBatchPinOrNetworkAction(ctx, events, subInfo, location, event, verifier, params)
}

func (f *Fabric) buildEventLocationString(chaincode string) string {
	return fmt.Sprintf("chaincode=%s", chaincode)
}

func (f *Fabric) processContractEvent(ctx context.Context, events common.EventsToDispatch, msgJSON fftypes.JSONObject) (err error) {
	subID := msgJSON.GetString("subId")
	subName, err := f.streams.getSubscriptionName(ctx, subID, false)
	if err != nil {
		return err // this is a problem - we should be able to find the listener that dispatched this to us
	}
	namespace := common.GetNamespaceFromSubName(subName)
	event := f.parseBlockchainEvent(ctx, msgJSON)
	if event != nil {
		f.callbacks.PrepareBlockchainEvent(ctx, events, namespace, &blockchain.EventForListener{
			Event:      event,
			ListenerID: subID,
		})
	}
	return nil
}

func (f *Fabric) AddFireflySubscription(ctx context.Context, namespace *core.Namespace, contract *blockchain.MultipartyContract, lastProtocolID string) (string, error) {
	fabricOnChainLocation, err := parseContractLocation(ctx, contract.Location)
	if err != nil {
		return "", err
	}

	version, err := f.GetNetworkVersion(ctx, contract.Location)
	if err != nil {
		return "", err
	}

	var options ContractOptions
	optionBytes := contract.Options.Bytes()
	if optionBytes != nil {
		if err = json.Unmarshal(optionBytes, &options); err != nil {
			log.L(ctx).Warnf("Could not parse multiparty contract options (%s): %s", err, optionBytes)
		}
	}
	if options.CustomPinSupport {
		fabricOnChainLocation.Chaincode = ""
	}

	streamID, ok := f.streamID[namespace.Name]
	if !ok {
		return "", i18n.NewError(ctx, coremsgs.MsgInternalServerError, "eventstream ID not found")
	}
	sub, err := f.streams.ensureFireFlySubscription(ctx, namespace.Name, version, fabricOnChainLocation, contract.FirstEvent, streamID, batchPinEvent, lastProtocolID)
	if err != nil {
		return "", err
	}

	f.subs.AddSubscription(ctx, namespace, version, sub.ID, fabricOnChainLocation.Channel)
	return sub.ID, nil
}

func (f *Fabric) RemoveFireflySubscription(ctx context.Context, subID string) {
	// Don't actually delete the subscription from fabconnect, as this may be called while processing
	// events from the subscription (and handling that scenario cleanly could be difficult for fabconnect).
	// TODO: can old subscriptions be somehow cleaned up later?
	f.subs.RemoveSubscription(ctx, subID)
}

func (f *Fabric) handleMessageBatch(ctx context.Context, messages []interface{}) error {
	// Build the set of events that need handling
	events := make(common.EventsToDispatch)
	count := len(messages)
	for i, msgI := range messages {
		msgMap, ok := msgI.(map[string]interface{})
		if !ok {
			log.L(ctx).Errorf("Message cannot be parsed as JSON: %+v", msgI)
			return nil // Swallow this and move on
		}
		msgJSON := fftypes.JSONObject(msgMap)

		eventName := msgJSON.GetString("eventName")
		sub := msgJSON.GetString("subId")
		logger := log.L(ctx)
		logger.Infof("[Fabric:%d/%d]: '%s' on '%s'", i+1, count, eventName, sub)
		logger.Tracef("Message: %+v", msgJSON)

		// Matches one of the active FireFly BatchPin subscriptions
		if subInfo := f.subs.GetSubscription(sub); subInfo != nil {
			location, err := encodeContractLocation(ctx, blockchain.NormalizeCall, &Location{
				Chaincode: msgJSON.GetString("chaincodeId"),
				Channel:   subInfo.Extra.(string),
			})
			if err != nil {
				return err
			}

			switch eventName {
			case broadcastBatchEventName:
				f.processBatchPinEvent(ctx, events, location, subInfo, msgJSON)
			default:
				log.L(ctx).Infof("Ignoring event with unknown name: %s", eventName)
			}
		} else {
			// Subscription not recognized - assume it's from a custom contract listener
			// (event manager will reject it if it's not)
			if err := f.processContractEvent(ctx, events, msgJSON); err != nil {
				return err
			}
		}
	}
	// Dispatch all the events from this patch that were successfully parsed and routed to namespaces
	// (could be zero - that's ok)
	return f.callbacks.DispatchBlockchainEvents(ctx, events)
}

func (f *Fabric) eventLoop(namespace string, wsconn wsclient.WSClient, closed chan struct{}) {
	topic := f.getTopic(namespace)
	defer wsconn.Close()
	defer close(closed)
	l := log.L(f.ctx).WithField("role", "event-loop").WithField("namespace", namespace)
	ctx := log.WithLogger(f.ctx, l)
	log.L(ctx).Debugf("Starting event loop for namespace '%s'", namespace)
	for {
		select {
		case <-ctx.Done():
			l.Debugf("Event loop exiting (context cancelled)")
			return
		case msgBytes, ok := <-wsconn.Receive():
			if !ok {
				l.Debugf("Event loop exiting (receive channel closed). Terminating server!")
				f.cancelCtx()
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
				err = f.handleMessageBatch(ctx, msgTyped)
				var ackOrNack []byte
				if err == nil {
					ackOrNack, _ = json.Marshal(map[string]string{"type": "ack", "topic": topic})
				} else {
					log.L(ctx).Errorf("Rejecting batch due error: %s", err)
					ackOrNack, _ = json.Marshal(map[string]string{"type": "error", "topic": topic, "message": err.Error()})
				}
				err = wsconn.Send(ctx, ackOrNack)
			case map[string]interface{}:
				var receipt common.BlockchainReceiptNotification
				_ = json.Unmarshal(msgBytes, &receipt)

				err := common.HandleReceipt(ctx, namespace, f, &receipt, f.callbacks)
				if err != nil {
					l.Errorf("Failed to process receipt: %+v", msgTyped)
				}
			default:
				l.Errorf("Message unexpected: %+v", msgTyped)
				continue
			}

			if err != nil {
				l.Errorf("Event loop exiting (%s). Terminating server!", err)
				f.cancelCtx()
				return
			}
		}
	}
}

func (f *Fabric) ResolveSigningKey(ctx context.Context, signingKeyInput string, intent blockchain.ResolveKeyIntent) (string, error) {
	// Note: "intent" is not currently used for Fabric, as the identity resolution is not
	//       currently pluggable to external identity resolution systems (as is the case for
	//       ethereum blockchain connectors).

	// Key is always required
	if signingKeyInput == "" {
		return "", i18n.NewError(ctx, coremsgs.MsgNodeMissingBlockchainKey)
	}

	// we expand the short user name into the fully qualified onchain identity:
	// mspid::x509::{ecert DN}::{CA DN}	return signingKeyInput, nil
	if !fullIdentityPattern.MatchString(signingKeyInput) {
		existingID := f.idCache[signingKeyInput]
		if existingID == nil {
			var idRes fabIdentity
			res, err := f.client.R().SetContext(f.ctx).SetResult(&idRes).Get(fmt.Sprintf("/identities/%s", signingKeyInput))
			if err != nil || !res.IsSuccess() {
				return "", i18n.NewError(f.ctx, coremsgs.MsgFabconnectRESTErr, err)
			}
			f.idCache[signingKeyInput] = &idRes
			existingID = &idRes
		}

		ecertDN, err := getDNFromCertString(existingID.ECert)
		if err != nil {
			return "", i18n.NewError(f.ctx, coremsgs.MsgFailedToDecodeCertificate, err)
		}
		cacertDN, err := getDNFromCertString(existingID.CACert)
		if err != nil {
			return "", i18n.NewError(f.ctx, coremsgs.MsgFailedToDecodeCertificate, err)
		}
		resolvedSigningKey := fmt.Sprintf("%s::x509::%s::%s", existingID.MSPID, ecertDN, cacertDN)
		log.L(f.ctx).Debugf("Resolved signing key: %s", resolvedSigningKey)
		return resolvedSigningKey, nil
	}
	return signingKeyInput, nil
}

func (f *Fabric) invokeContractMethod(ctx context.Context, channel, chaincode, methodName, signingKey, requestID string, prefixItems []*PrefixItem, input map[string]interface{}, options map[string]interface{}) (submissionRejected bool, err error) {
	body, err := f.buildFabconnectRequestBody(ctx, channel, chaincode, methodName, signingKey, requestID, prefixItems, input, options)
	if err != nil {
		return true, err
	}
	var resErr common.BlockchainRESTError
	res, err := f.client.R().
		SetContext(ctx).
		SetHeader("x-firefly-sync", "false").
		SetBody(body).
		SetError(&resErr).
		Post("/transactions")
	if err != nil || !res.IsSuccess() {
		return resErr.SubmissionRejected, common.WrapRESTError(ctx, &resErr, res, err, coremsgs.MsgFabconnectRESTErr)
	}
	return false, nil
}

func (f *Fabric) queryContractMethod(ctx context.Context, channel, chaincode, methodName, signingKey, requestID string, prefixItems []*PrefixItem, input map[string]interface{}, options map[string]interface{}) (*resty.Response, error) {
	body, err := f.buildFabconnectRequestBody(ctx, channel, chaincode, methodName, signingKey, requestID, prefixItems, input, options)
	if err != nil {
		return nil, err
	}
	var resErr common.BlockchainRESTError
	res, err := f.client.R().
		SetContext(ctx).
		SetBody(body).
		SetError(&resErr).
		Post("/query")
	if err != nil || !res.IsSuccess() {
		return res, common.WrapRESTError(ctx, &resErr, res, err, coremsgs.MsgFabconnectRESTErr)
	}
	return res, nil
}

func getUserName(fullIDString string) string {
	matches := fullIdentityPattern.FindStringSubmatch(fullIDString)
	if len(matches) == 0 {
		return fullIDString
	}
	matches = cnPattern.FindStringSubmatch(matches[1])
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

func hexFormatB32(b *fftypes.Bytes32) string {
	if b == nil {
		return "0x0000000000000000000000000000000000000000000000000000000000000000"
	}
	return "0x" + hex.EncodeToString(b[0:32])
}

func (f *Fabric) buildBatchPinInput(version int, namespace string, batch *blockchain.BatchPin) (prefixItems []*PrefixItem, pinInput map[string]interface{}) {
	hashes := make([]string, len(batch.Contexts))
	for i, v := range batch.Contexts {
		hashes[i] = hexFormatB32(v)
	}
	var uuids fftypes.Bytes32
	copy(uuids[0:16], (*batch.TransactionID)[:])
	copy(uuids[16:32], (*batch.BatchID)[:])

	if version == 1 {
		prefixItems = batchPinPrefixItemsV1
		pinInput = map[string]interface{}{
			"namespace":  namespace,
			"uuids":      hexFormatB32(&uuids),
			"batchHash":  hexFormatB32(batch.BatchHash),
			"payloadRef": batch.BatchPayloadRef,
			"contexts":   hashes,
		}
	} else {
		prefixItems = batchPinPrefixItems
		pinInput = map[string]interface{}{
			"uuids":      hexFormatB32(&uuids),
			"batchHash":  hexFormatB32(batch.BatchHash),
			"payloadRef": batch.BatchPayloadRef,
			"contexts":   hashes,
		}
	}
	return prefixItems, pinInput
}

func (f *Fabric) SubmitBatchPin(ctx context.Context, nsOpID, networkNamespace, signingKey string, batch *blockchain.BatchPin, location *fftypes.JSONAny) error {
	fabricOnChainLocation, err := parseContractLocation(ctx, location)
	if err != nil {
		return err
	}

	version, err := f.GetNetworkVersion(ctx, location)
	if err != nil {
		return err
	}

	prefixItems, pinInput := f.buildBatchPinInput(version, networkNamespace, batch)

	input, _ := jsonEncodeInput(pinInput)
	_, err = f.invokeContractMethod(ctx, fabricOnChainLocation.Channel, fabricOnChainLocation.Chaincode, batchPinMethodName, signingKey, nsOpID, prefixItems, input, nil)
	return err
}

func (f *Fabric) SubmitNetworkAction(ctx context.Context, nsOpID string, signingKey string, action core.NetworkActionType, location *fftypes.JSONAny) error {
	fabricOnChainLocation, err := parseContractLocation(ctx, location)
	if err != nil {
		return err
	}

	version, err := f.GetNetworkVersion(ctx, location)
	if err != nil {
		return err
	}

	var methodName string
	var prefixItems []*PrefixItem
	var pinInput map[string]interface{}

	if version == 1 {
		methodName = batchPinMethodName
		prefixItems = batchPinPrefixItemsV1
		pinInput = map[string]interface{}{
			"namespace":  "firefly:" + action,
			"uuids":      hexFormatB32(nil),
			"batchHash":  hexFormatB32(nil),
			"payloadRef": "",
			"contexts":   []string{},
		}
	} else {
		methodName = networkActionMethodName
		prefixItems = networkActionPrefixItems
		pinInput = map[string]interface{}{
			"action":  "firefly:" + action,
			"payload": "",
		}
	}

	input, _ := jsonEncodeInput(pinInput)
	_, err = f.invokeContractMethod(ctx, fabricOnChainLocation.Channel, fabricOnChainLocation.Chaincode, methodName, signingKey, nsOpID, prefixItems, input, nil)
	return err
}

func (f *Fabric) buildFabconnectRequestBody(ctx context.Context, channel, chaincode, methodName, signingKey, requestID string, prefixItems []*PrefixItem, input map[string]interface{}, options map[string]interface{}) (map[string]interface{}, error) {
	// All arguments must be JSON serialized
	args, err := jsonEncodeInput(input)
	if err != nil {
		return nil, i18n.WrapError(ctx, err, i18n.MsgJSONObjectParseFailed, "params")
	}
	body := map[string]interface{}{
		"headers": &fabTxInputHeaders{
			ID: requestID,
			PayloadSchema: &PayloadSchema{
				Type:        "array",
				PrefixItems: prefixItems,
			},
			Channel:   channel,
			Chaincode: chaincode,
			Signer:    getUserName(signingKey),
		},
		"func": methodName,
		"args": args,
	}
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

func (f *Fabric) DeployContract(ctx context.Context, nsOpID, signingKey string, definition, contract *fftypes.JSONAny, input []interface{}, options map[string]interface{}) (submissionRejected bool, err error) {
	return true, i18n.NewError(ctx, coremsgs.MsgNotSupportedByBlockchainPlugin)
}

func (f *Fabric) ValidateInvokeRequest(ctx context.Context, parsedMethod interface{}, input map[string]interface{}, hasMessage bool) error {
	// No additional validation beyond what is enforced by Contract Manager
	_, _, err := f.recoverFFI(ctx, parsedMethod)
	return err
}

func (f *Fabric) InvokeContract(ctx context.Context, nsOpID string, signingKey string, location *fftypes.JSONAny, parsedMethod interface{}, input map[string]interface{}, options map[string]interface{}, batch *blockchain.BatchPin) (bool, error) {

	method, _, err := f.recoverFFI(ctx, parsedMethod)
	if err != nil {
		return true, err
	}

	fabricOnChainLocation, err := parseContractLocation(ctx, location)
	if err != nil {
		return true, err
	}

	// Build the payload schema for the method parameters
	prefixItems := make([]*PrefixItem, len(method.Params))
	for i, param := range method.Params {
		var paramSchema ffiParamSchema
		if err := json.Unmarshal(param.Schema.Bytes(), &paramSchema); err != nil {
			return true, i18n.WrapError(ctx, err, i18n.MsgJSONObjectParseFailed, fmt.Sprintf("%s.schema", param.Name))
		}

		prefixItems[i] = &PrefixItem{
			Name: param.Name,
			Type: paramSchema.Type,
		}
	}

	if batch != nil {
		_, batchPin := f.buildBatchPinInput(2, "", batch)
		if input == nil {
			input = make(map[string]interface{})
		}
		batchPinBytes, _ := json.Marshal(batchPin)
		lastParam := method.Params[len(method.Params)-1]
		input[lastParam.Name] = string(batchPinBytes)
	}

	return f.invokeContractMethod(ctx, fabricOnChainLocation.Channel, fabricOnChainLocation.Chaincode, method.Name, signingKey, nsOpID, prefixItems, input, options)
}

type ffiMethodAndErrors struct {
	method *fftypes.FFIMethod
	errors []*fftypes.FFIError
}

func (f *Fabric) ParseInterface(ctx context.Context, method *fftypes.FFIMethod, errors []*fftypes.FFIError) (interface{}, error) {
	// Not very sophisticated here - we don't need to parse the FFIMethod/FFIError for Fabric,
	// as there is no underlying schema to map them to. So we just use them directly.
	return &ffiMethodAndErrors{
		method: method,
		errors: errors,
	}, nil
}

func (f *Fabric) recoverFFI(ctx context.Context, parsedMethod interface{}) (*fftypes.FFIMethod, []*fftypes.FFIError, error) {
	methodInfo, ok := parsedMethod.(*ffiMethodAndErrors)
	if !ok || methodInfo.method == nil {
		return nil, nil, i18n.NewError(ctx, coremsgs.MsgUnexpectedInterfaceType, parsedMethod)
	}
	return methodInfo.method, methodInfo.errors, nil
}

func (f *Fabric) QueryContract(ctx context.Context, signingKey string, location *fftypes.JSONAny, parsedMethod interface{}, input map[string]interface{}, options map[string]interface{}) (interface{}, error) {
	method, _, err := f.recoverFFI(ctx, parsedMethod)
	if err != nil {
		return nil, err
	}

	fabricOnChainLocation, err := parseContractLocation(ctx, location)
	if err != nil {
		return nil, err
	}

	// Build the payload schema for the method parameters
	prefixItems := make([]*PrefixItem, len(method.Params))
	for i, param := range method.Params {
		prefixItems[i] = &PrefixItem{
			Name: param.Name,
			Type: "string",
		}
	}

	res, err := f.queryContractMethod(ctx, fabricOnChainLocation.Channel, fabricOnChainLocation.Chaincode, method.Name, signingKey, "", prefixItems, input, options)
	if err != nil {
		return nil, err
	}
	output := &fabQueryNamedOutput{}
	if err = json.Unmarshal(res.Body(), output); err != nil {
		return nil, err
	}
	return output.Result, nil
}

func jsonEncodeInput(params map[string]interface{}) (output map[string]interface{}, err error) {
	output = make(map[string]interface{}, len(params))
	for field, value := range params {
		switch v := value.(type) {
		case string:
			output[field] = v
		default:
			encodedValue, err := json.Marshal(v)
			if err != nil {
				return nil, err
			}
			output[field] = string(encodedValue)
		}

	}
	return
}

func (f *Fabric) CheckOverlappingLocations(ctx context.Context, left *fftypes.JSONAny, right *fftypes.JSONAny) (bool, error) {
	parsedLeft, err := parseContractLocation(ctx, left)
	if err != nil {
		return false, err
	}

	parsedRight, err := parseContractLocation(ctx, right)
	if err != nil {
		return false, err
	}

	// Different channel so not overlapping
	if parsedLeft.Channel != parsedRight.Channel {
		return false, nil
	}

	if parsedLeft.Chaincode == "" || parsedRight.Chaincode == "" {
		// Either of them location is the whole channel
		return true, nil
	}

	// No just compare chaincodes
	return parsedLeft.Chaincode == parsedRight.Chaincode, nil
}

func (f *Fabric) NormalizeContractLocation(ctx context.Context, ntype blockchain.NormalizeType, location *fftypes.JSONAny) (result *fftypes.JSONAny, err error) {
	parsed, err := parseContractLocation(ctx, location)
	if err != nil {
		return nil, err
	}
	return encodeContractLocation(ctx, ntype, parsed)
}

func (f *Fabric) stringifyContractLocation(ctx context.Context, location *fftypes.JSONAny) (string, error) {
	parsed, err := parseContractLocation(ctx, location)
	if err != nil {
		return "", err
	}

	// Concatinate channel and chaincode if present
	result := fmt.Sprintf("%s-*", parsed.Channel)
	if parsed.Chaincode != "" {
		result = fmt.Sprintf("%s-%s", parsed.Channel, parsed.Chaincode)
	}

	return result, nil
}

func parseContractLocation(ctx context.Context, location *fftypes.JSONAny) (*Location, error) {
	if location == nil {
		return nil, i18n.NewError(ctx, coremsgs.MsgContractLocationInvalid, "'channel' not set")
	}
	fabricLocation := Location{}
	if err := json.Unmarshal(location.Bytes(), &fabricLocation); err != nil {
		return nil, i18n.NewError(ctx, coremsgs.MsgContractLocationInvalid, err)
	}
	if fabricLocation.Channel == "" {
		return nil, i18n.NewError(ctx, coremsgs.MsgContractLocationInvalid, "'channel' not set")
	}
	return &fabricLocation, nil
}

func encodeContractLocation(ctx context.Context, ntype blockchain.NormalizeType, location *Location) (result *fftypes.JSONAny, err error) {
	if location.Channel == "" {
		return nil, i18n.NewError(ctx, coremsgs.MsgContractLocationInvalid, "'channel' not set")
	}
	if ntype == blockchain.NormalizeCall && location.Chaincode == "" {
		return nil, i18n.NewError(ctx, coremsgs.MsgContractLocationInvalid, "'chaincode' not set")
	}
	normalized, err := json.Marshal(location)
	if err == nil {
		result = fftypes.JSONAnyPtrBytes(normalized)
	}
	return result, err
}

func (f *Fabric) AddContractListener(ctx context.Context, listener *core.ContractListener, lastProtocolID string) error {
	namespace := listener.Namespace

	if len(listener.Filters) == 0 {
		return i18n.NewError(ctx, coremsgs.MsgFiltersEmpty, listener.Name)
	}

	if len(listener.Filters) > 1 {
		return i18n.NewError(ctx, coremsgs.MsgContractListenerBlockchainFilterLimit, listener.Name)
	}

	filter := listener.Filters[0]

	location, err := parseContractLocation(ctx, filter.Location)
	if err != nil {
		return err
	}

	subName := fmt.Sprintf("ff-sub-%s-%s", listener.Namespace, listener.ID)
	result, err := f.streams.createSubscription(ctx, location, f.streamID[namespace], subName, filter.Event.Name, listener.Options.FirstEvent, lastProtocolID)
	if err != nil {
		return err
	}
	listener.BackendID = result.ID
	return nil
}

func (f *Fabric) DeleteContractListener(ctx context.Context, subscription *core.ContractListener, okNotFound bool) error {
	return f.streams.deleteSubscription(ctx, subscription.BackendID, okNotFound)
}

func (f *Fabric) GetContractListenerStatus(ctx context.Context, namespace, subID string, okNotFound bool) (bool, interface{}, core.ContractListenerStatus, error) {
	// Fabconnect does not currently provide any additional status info for listener subscriptions.
	// But we check for existence of the subscription
	sub, err := f.streams.getSubscription(ctx, subID, okNotFound)
	if err != nil || sub == nil {
		return false, nil, core.ContractListenerStatusUnknown, err
	}

	return true, nil, core.ContractListenerStatusUnknown, err
}

func (f *Fabric) GetFFIParamValidator(ctx context.Context) (fftypes.FFIParamValidator, error) {
	// Fabconnect does not require any additional validation beyond "JSON Schema correctness" at this time
	return nil, nil
}

func (f *Fabric) GenerateFFI(ctx context.Context, generationRequest *fftypes.FFIGenerationRequest) (*fftypes.FFI, error) {
	return nil, i18n.NewError(ctx, coremsgs.MsgFFIGenerationUnsupported)
}

func (f *Fabric) GenerateEventSignature(ctx context.Context, event *fftypes.FFIEventDefinition) (string, error) {
	return event.Name, nil
}

func (f *Fabric) GenerateEventSignatureWithLocation(ctx context.Context, event *fftypes.FFIEventDefinition, location *fftypes.JSONAny) (string, error) {
	strLocation, err := f.stringifyContractLocation(ctx, location)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s:%s", strLocation, event.Name), nil
}

func (f *Fabric) GenerateErrorSignature(ctx context.Context, event *fftypes.FFIErrorDefinition) string {
	// not relevant to Fabric blockchains
	return ""
}

func (f *Fabric) GetNetworkVersion(ctx context.Context, location *fftypes.JSONAny) (version int, err error) {
	fabricOnChainLocation, err := parseContractLocation(ctx, location)
	if err != nil {
		return 0, err
	}

	cacheKey := "version:" + fabricOnChainLocation.Channel + ":" + fabricOnChainLocation.Chaincode
	if cachedValue := f.cache.GetInt(cacheKey); cachedValue != 0 {
		return cachedValue, nil
	}

	version, err = f.queryNetworkVersion(ctx, fabricOnChainLocation.Channel, fabricOnChainLocation.Chaincode)
	if err == nil {
		f.cache.SetInt(cacheKey, version)
	}
	return version, err
}

func (f *Fabric) queryNetworkVersion(ctx context.Context, channel, chaincode string) (version int, err error) {
	res, err := f.queryContractMethod(ctx, channel, chaincode, networkVersionMethodName, f.signer, "", []*PrefixItem{}, map[string]interface{}{}, nil)
	if err != nil || !res.IsSuccess() {
		// "Function not found" is interpreted as "default to version 1"
		notFoundError := fmt.Sprintf("Function %s not found", networkVersionMethodName)
		if strings.Contains(err.Error(), notFoundError) {
			return 1, nil
		}
		return 0, err
	}
	output := &fabQueryNamedOutput{}
	if err = json.Unmarshal(res.Body(), output); err != nil {
		return 0, err
	}

	switch result := output.Result.(type) {
	case float64:
		version = int(result)
	default:
		err = i18n.NewError(ctx, coremsgs.MsgBadNetworkVersion, output.Result)
	}
	return version, err
}

func (f *Fabric) GetAndConvertDeprecatedContractConfig(ctx context.Context) (location *fftypes.JSONAny, fromBlock string, err error) {
	// Old config (attributes under "fabconnect")
	chaincode := f.fabconnectConf.GetString(FabconnectConfigChaincodeDeprecated)
	if chaincode != "" {
		log.L(ctx).Warnf("The %s.%s config key has been deprecated. Please use namespaces.predefined[].multiparty.contract[].location.address",
			FabconnectConfigKey, FabconnectConfigChaincodeDeprecated)
	} else {
		return nil, "", i18n.NewError(ctx, coremsgs.MsgMissingPluginConfig, "chaincode", "blockchain.fabric.fabconnect")
	}
	fromBlock = string(core.SubOptsFirstEventOldest)

	location, err = encodeContractLocation(ctx, blockchain.NormalizeListener, &Location{
		Chaincode: chaincode,
		Channel:   f.defaultChannel,
	})
	return location, fromBlock, err
}

func (f *Fabric) GetTransactionStatus(ctx context.Context, operation *core.Operation) (interface{}, error) {
	txHash := operation.Output.GetString("transactionHash")

	defaultChannel := f.fabconnectConf.GetString(FabconnectConfigDefaultChannel)
	if defaultChannel == "" {
		return nil, i18n.NewError(ctx, coremsgs.MsgDefaultChannelNotConfigured)
	}
	defaultSigner := f.fabconnectConf.GetString(FabconnectConfigSigner)
	if defaultSigner == "" {
		return nil, i18n.NewError(ctx, coremsgs.MsgNodeMissingBlockchainKey)
	}

	transactionRequestPath := fmt.Sprintf("/transactions/%s", txHash)
	client := f.client
	var resErr common.BlockchainRESTError
	var statusResponse fftypes.JSONObject
	res, err := client.R().
		SetContext(ctx).
		SetError(&resErr).
		SetResult(&statusResponse).
		SetQueryParam("fly-channel", defaultChannel).
		SetQueryParam("fly-signer", defaultSigner).
		Get(transactionRequestPath)
	if err != nil || !res.IsSuccess() {
		if res.StatusCode() == http.StatusNotFound {
			return nil, nil
		}
		return nil, common.WrapRESTError(ctx, &resErr, res, err, coremsgs.MsgFabconnectRESTErr)
	}

	// TODO - could implement the same enhancement ethconnect has, and build a mock WS receipt if an API query
	// happens to update the status of a pending transaction in our store.

	return statusResponse, nil
}
