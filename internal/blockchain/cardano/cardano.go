// Copyright © 2025 IOG Singapore and SundaeSwap, Inc.
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

package cardano

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/go-resty/resty/v2"
	"github.com/hyperledger/firefly-common/pkg/config"
	"github.com/hyperledger/firefly-common/pkg/ffresty"
	"github.com/hyperledger/firefly-common/pkg/fftypes"
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/hyperledger/firefly-common/pkg/log"
	"github.com/hyperledger/firefly-common/pkg/wsclient"
	"github.com/hyperledger/firefly/internal/blockchain/common"
	"github.com/hyperledger/firefly/internal/cache"
	"github.com/hyperledger/firefly/internal/coremsgs"
	"github.com/hyperledger/firefly/internal/metrics"
	"github.com/hyperledger/firefly/pkg/blockchain"
	"github.com/hyperledger/firefly/pkg/core"
)

type Cardano struct {
	ctx                context.Context
	cancelCtx          context.CancelFunc
	pluginTopic        string
	metrics            metrics.Manager
	capabilities       *blockchain.Capabilities
	callbacks          common.BlockchainCallbacks
	client             *resty.Client
	streams            *streamManager
	streamIDs          map[string]string
	closed             map[string]chan struct{}
	wsconns            map[string]wsclient.WSClient
	wsConfig           *wsclient.WSConfig
	cardanoconnectConf config.Section
	subs               common.FireflySubscriptions
}

type ffiMethodAndErrors struct {
	method *fftypes.FFIMethod
	errors []*fftypes.FFIError
}

type cardanoWSCommandPayload struct {
	Type        string `json:"type"`
	Topic       string `json:"topic,omitempty"`
	BatchNumber int64  `json:"batchNumber,omitempty"`
	Message     string `json:"message,omitempty"`
}

type Location struct {
	Address string `json:"address"`
}

var addressVerify = regexp.MustCompile("^addr1|^addr_test1|^stake1|^stake_test1")

func (c *Cardano) Name() string {
	return "cardano"
}

func (c *Cardano) VerifierType() core.VerifierType {
	return core.VerifierTypeCardanoAddress
}

func (c *Cardano) Init(ctx context.Context, cancelCtx context.CancelFunc, conf config.Section, metrics metrics.Manager, cacheManager cache.Manager) (err error) {
	c.InitConfig(conf)
	cardanoconnectConf := c.cardanoconnectConf

	c.ctx = log.WithLogField(ctx, "proto", "cardano")
	c.cancelCtx = cancelCtx
	c.metrics = metrics
	c.capabilities = &blockchain.Capabilities{}
	c.callbacks = common.NewBlockchainCallbacks()
	c.subs = common.NewFireflySubscriptions()

	if cardanoconnectConf.GetString(ffresty.HTTPConfigURL) == "" {
		return i18n.NewError(ctx, coremsgs.MsgMissingPluginConfig, "url", cardanoconnectConf)
	}

	c.wsConfig, err = wsclient.GenerateConfig(ctx, cardanoconnectConf)
	if err == nil {
		c.client, err = ffresty.New(c.ctx, cardanoconnectConf)
	}

	if err != nil {
		return err
	}

	c.pluginTopic = cardanoconnectConf.GetString(CardanoconnectConfigTopic)
	if c.pluginTopic == "" {
		return i18n.NewError(ctx, coremsgs.MsgMissingPluginConfig, "topic", "blockchain.cardano.cardanoconnect")
	}

	if c.wsConfig.WSKeyPath == "" {
		c.wsConfig.WSKeyPath = "/api/v1/ws"
	}

	c.streamIDs = make(map[string]string)
	c.closed = make(map[string]chan struct{})
	c.wsconns = make(map[string]wsclient.WSClient)
	c.streams = newStreamManager(c.client, c.cardanoconnectConf.GetUint(CardanoconnectConfigBatchSize), c.cardanoconnectConf.GetDuration(CardanoconnectConfigBatchTimeout).Milliseconds())

	return nil
}

func (c *Cardano) getTopic(namespace string) string {
	return fmt.Sprintf("%s/%s", c.pluginTopic, namespace)
}

func (c *Cardano) StartNamespace(ctx context.Context, namespace string) (err error) {
	logger := log.L(c.ctx)
	logger.Debugf("Starting namespace: %s", namespace)
	topic := c.getTopic(namespace)

	c.wsconns[namespace], err = wsclient.New(ctx, c.wsConfig, nil, func(ctx context.Context, w wsclient.WSClient) error {
		b, _ := json.Marshal(&cardanoWSCommandPayload{
			Type:  "listen",
			Topic: topic,
		})
		err := w.Send(ctx, b)
		if err == nil {
			b, _ = json.Marshal(&cardanoWSCommandPayload{
				Type: "listenreplies",
			})
			err = w.Send(ctx, b)
		}
		return err
	})
	if err != nil {
		return err
	}

	// Ensure that our event stream is in place
	stream, err := c.streams.ensureEventStream(ctx, topic)
	if err != nil {
		return err
	}
	logger.Infof("Event stream: %s (topic=%s)", stream.ID, topic)
	c.streamIDs[namespace] = stream.ID

	err = c.wsconns[namespace].Connect()
	if err != nil {
		return err
	}

	c.closed[namespace] = make(chan struct{})

	go c.eventLoop(namespace)

	return nil
}

func (c *Cardano) StopNamespace(ctx context.Context, namespace string) (err error) {
	wsconn, ok := c.wsconns[namespace]
	if ok {
		wsconn.Close()
	}
	delete(c.wsconns, namespace)
	delete(c.streamIDs, namespace)
	delete(c.closed, namespace)

	return nil
}

func (c *Cardano) SetHandler(namespace string, handler blockchain.Callbacks) {
	c.callbacks.SetHandler(namespace, handler)
}

func (c *Cardano) SetOperationHandler(namespace string, handler core.OperationCallbacks) {
	c.callbacks.SetOperationalHandler(namespace, handler)
}

func (c *Cardano) Capabilities() *blockchain.Capabilities {
	return c.capabilities
}

func (c *Cardano) AddFireflySubscription(ctx context.Context, namespace *core.Namespace, contract *blockchain.MultipartyContract, lastProtocolID string) (string, error) {
	location, err := c.parseContractLocation(ctx, contract.Location)
	if err != nil {
		return "", err
	}

	version, _ := c.GetNetworkVersion(ctx, contract.Location)

	l, err := c.streams.ensureFireFlyListener(ctx, namespace.Name, version, location.Address, contract.FirstEvent, c.streamIDs[namespace.Name])
	if err != nil {
		return "", err
	}

	c.subs.AddSubscription(ctx, namespace, version, l.ID, nil)
	return l.ID, nil
}

func (c *Cardano) RemoveFireflySubscription(ctx context.Context, subID string) {
	c.subs.RemoveSubscription(ctx, subID)
}

func (c *Cardano) ResolveSigningKey(ctx context.Context, key string, intent blockchain.ResolveKeyIntent) (resolved string, err error) {
	if key == "" {
		return "", i18n.NewError(ctx, coremsgs.MsgNodeMissingBlockchainKey)
	}
	resolved, err = formatCardanoAddress(ctx, key)
	return resolved, err
}

func (c *Cardano) SubmitBatchPin(ctx context.Context, nsOpID, networkNamespace, signingKey string, batch *blockchain.BatchPin, location *fftypes.JSONAny) error {
	log.L(ctx).Warn("SubmitBatchPin is not supported")
	return i18n.NewError(ctx, coremsgs.MsgNotSupportedByBlockchainPlugin)
}

func (c *Cardano) SubmitNetworkAction(ctx context.Context, nsOpID string, signingKey string, action core.NetworkActionType, location *fftypes.JSONAny) error {
	log.L(ctx).Warn("SubmitNetworkAction is not supported")
	return i18n.NewError(ctx, coremsgs.MsgNotSupportedByBlockchainPlugin)
}

func (c *Cardano) DeployContract(ctx context.Context, nsOpID, signingKey string, definition, contract *fftypes.JSONAny, input []interface{}, options map[string]interface{}) (submissionRejected bool, err error) {
	body := map[string]interface{}{
		"id":         nsOpID,
		"contract":   contract,
		"definition": definition,
	}
	var resErr common.BlockchainRESTError
	res, err := c.client.R().
		SetContext(ctx).
		SetBody(body).
		SetError(&resErr).
		Post("/api/v1/contracts/deploy")
	if err != nil || !res.IsSuccess() {
		return resErr.SubmissionRejected, common.WrapRESTError(ctx, &resErr, res, err, coremsgs.MsgCardanoconnectRESTErr)
	}
	return false, nil
}

func (c *Cardano) ValidateInvokeRequest(ctx context.Context, parsedMethod interface{}, input map[string]interface{}, hasMessage bool) error {
	// No additional validation beyond what is enforced by Contract Manager
	_, _, err := c.recoverFFI(ctx, parsedMethod)
	return err
}

func (c *Cardano) InvokeContract(ctx context.Context, nsOpID string, signingKey string, location *fftypes.JSONAny, parsedMethod interface{}, input map[string]interface{}, options map[string]interface{}, batch *blockchain.BatchPin) (bool, error) {
	cardanoLocation, err := c.parseContractLocation(ctx, location)
	if err != nil {
		return true, err
	}

	methodInfo, ok := parsedMethod.(*ffiMethodAndErrors)
	if !ok || methodInfo.method == nil || methodInfo.method.Name == "" {
		return true, i18n.NewError(ctx, coremsgs.MsgUnexpectedInterfaceType, parsedMethod)
	}
	method := methodInfo.method
	params := make([]interface{}, 0)
	for _, param := range method.Params {
		params = append(params, input[param.Name])
	}

	body := map[string]interface{}{
		"id":      nsOpID,
		"address": cardanoLocation.Address,
		"method":  method,
		"params":  params,
	}
	if signingKey != "" {
		body["from"] = signingKey
	}

	var resErr common.BlockchainRESTError
	res, err := c.client.R().
		SetContext(ctx).
		SetBody(body).
		SetError(&resErr).
		Post("/api/v1/contracts/invoke")
	if err != nil || !res.IsSuccess() {
		return resErr.SubmissionRejected, common.WrapRESTError(ctx, &resErr, res, err, coremsgs.MsgCardanoconnectRESTErr)
	}
	return false, nil
}

func (c *Cardano) QueryContract(ctx context.Context, signingKey string, location *fftypes.JSONAny, parsedMethod interface{}, input map[string]interface{}, options map[string]interface{}) (interface{}, error) {
	cardanoLocation, err := c.parseContractLocation(ctx, location)
	if err != nil {
		return nil, err
	}

	methodInfo, ok := parsedMethod.(*ffiMethodAndErrors)
	if !ok || methodInfo.method == nil || methodInfo.method.Name == "" {
		return nil, i18n.NewError(ctx, coremsgs.MsgUnexpectedInterfaceType, parsedMethod)
	}
	method := methodInfo.method
	params := make([]interface{}, 0)
	for _, param := range method.Params {
		params = append(params, input[param.Name])
	}

	body := map[string]interface{}{
		"address": cardanoLocation.Address,
		"method":  method,
		"params":  params,
	}
	if signingKey != "" {
		body["from"] = signingKey
	}

	var resErr common.BlockchainRESTError
	res, err := c.client.R().
		SetContext(ctx).
		SetBody(body).
		SetError(&resErr).
		Post("/api/v1/contracts/query")
	if err != nil || !res.IsSuccess() {
		return nil, common.WrapRESTError(ctx, &resErr, res, err, coremsgs.MsgCardanoconnectRESTErr)
	}
	var output interface{}
	if err = json.Unmarshal(res.Body(), &output); err != nil {
		return nil, err
	}
	return output, nil
}

func (c *Cardano) ParseInterface(ctx context.Context, method *fftypes.FFIMethod, errors []*fftypes.FFIError) (interface{}, error) {
	return &ffiMethodAndErrors{
		method: method,
		errors: errors,
	}, nil
}

func (c *Cardano) NormalizeContractLocation(ctx context.Context, ntype blockchain.NormalizeType, location *fftypes.JSONAny) (result *fftypes.JSONAny, err error) {
	parsed, err := c.parseContractLocation(ctx, location)
	if err != nil {
		return nil, err
	}
	return c.encodeContractLocation(ctx, parsed)
}

func (c *Cardano) CheckOverlappingLocations(ctx context.Context, left *fftypes.JSONAny, right *fftypes.JSONAny) (bool, error) {
	if left == nil || right == nil {
		// No location on either side so overlapping
		return true, nil
	}

	parsedLeft, err := c.parseContractLocation(ctx, left)
	if err != nil {
		return false, err
	}

	parsedRight, err := c.parseContractLocation(ctx, right)
	if err != nil {
		return false, err
	}

	// For cardano just compare addresses
	return strings.EqualFold(parsedLeft.Address, parsedRight.Address), nil
}

func (c *Cardano) parseContractLocation(ctx context.Context, location *fftypes.JSONAny) (*Location, error) {
	cardanoLocation := Location{}
	if err := json.Unmarshal(location.Bytes(), &cardanoLocation); err != nil {
		return nil, i18n.NewError(ctx, coremsgs.MsgContractLocationInvalid, err)
	}
	if cardanoLocation.Address == "" {
		return nil, i18n.NewError(ctx, coremsgs.MsgContractLocationInvalid, "'address' not set")
	}
	return &cardanoLocation, nil
}

func (c *Cardano) encodeContractLocation(_ context.Context, location *Location) (result *fftypes.JSONAny, err error) {
	normalized, err := json.Marshal(location)
	if err == nil {
		result = fftypes.JSONAnyPtrBytes(normalized)
	}
	return result, err
}

func (c *Cardano) AddContractListener(ctx context.Context, listener *core.ContractListener, lastProtocolID string) (err error) {
	namespace := listener.Namespace

	if len(listener.Filters) == 0 {
		return i18n.NewError(ctx, coremsgs.MsgFiltersEmpty, listener.Name)
	}

	subName := fmt.Sprintf("ff-sub-%s-%s", namespace, listener.ID)
	firstEvent := string(core.SubOptsFirstEventNewest)
	if listener.Options != nil {
		firstEvent = listener.Options.FirstEvent
	}

	filters := make([]filter, len(listener.Filters))
	for _, f := range listener.Filters {
		location, err := c.parseContractLocation(ctx, f.Location)
		if err != nil {
			return err
		}
		signature, _ := c.GenerateEventSignature(ctx, &f.Event.FFIEventDefinition)
		filters = append(filters, filter{
			eventfilter{
				Contract:  location.Address,
				EventPath: signature,
			},
		})
	}

	result, err := c.streams.createListener(ctx, c.streamIDs[namespace], subName, firstEvent, filters)
	if result != nil {
		listener.BackendID = result.ID
	}
	return err
}

func (c *Cardano) DeleteContractListener(ctx context.Context, subscription *core.ContractListener, okNotFound bool) error {
	return c.streams.deleteListener(ctx, c.streamIDs[subscription.Namespace], subscription.BackendID)
}

func (c *Cardano) GetContractListenerStatus(ctx context.Context, namespace, subID string, okNotFound bool) (found bool, detail interface{}, status core.ContractListenerStatus, err error) {
	l, err := c.streams.getListener(ctx, c.streamIDs[namespace], subID, okNotFound)
	if err != nil || l == nil {
		return false, nil, core.ContractListenerStatusUnknown, err
	}
	return true, nil, core.ContractListenerStatusUnknown, nil
}

func (c *Cardano) GetFFIParamValidator(ctx context.Context) (fftypes.FFIParamValidator, error) {
	// Cardanoconnect does not require any additional validation beyond "JSON Schema correctness" at this time
	return nil, nil
}

func (c *Cardano) GenerateEventSignature(ctx context.Context, event *fftypes.FFIEventDefinition) (string, error) {
	params := []string{}
	for _, param := range event.Params {
		params = append(params, param.Schema.JSONObject().GetString("type"))
	}
	return fmt.Sprintf("%s(%s)", event.Name, strings.Join(params, ",")), nil
}

func (c *Cardano) GenerateEventSignatureWithLocation(ctx context.Context, event *fftypes.FFIEventDefinition, location *fftypes.JSONAny) (string, error) {
	eventSignature, _ := c.GenerateEventSignature(ctx, event)

	if location == nil {
		return fmt.Sprintf("*:%s", eventSignature), nil
	}

	parsed, err := c.parseContractLocation(ctx, location)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s:%s", parsed.Address, eventSignature), nil
}

func (c *Cardano) GenerateErrorSignature(ctx context.Context, event *fftypes.FFIErrorDefinition) string {
	params := []string{}
	for _, param := range event.Params {
		params = append(params, param.Schema.JSONObject().GetString("type"))
	}
	return fmt.Sprintf("%s(%s)", event.Name, strings.Join(params, ","))
}

func (c *Cardano) GenerateFFI(ctx context.Context, generationRequest *fftypes.FFIGenerationRequest) (*fftypes.FFI, error) {
	return nil, i18n.NewError(ctx, coremsgs.MsgFFIGenerationUnsupported)
}

func (c *Cardano) GetNetworkVersion(ctx context.Context, location *fftypes.JSONAny) (version int, err error) {
	// Part of the FIR-12. https://github.com/hyperledger/firefly-fir/pull/12
	// Cardano doesn't support any of this yet, so just pretend we're on the new hotness
	return 2, nil
}

func (c *Cardano) GetAndConvertDeprecatedContractConfig(ctx context.Context) (location *fftypes.JSONAny, fromBlock string, err error) {
	return nil, "", nil
}

func (c *Cardano) GetTransactionStatus(ctx context.Context, operation *core.Operation) (interface{}, error) {
	txnID := (&core.PreparedOperation{ID: operation.ID, Namespace: operation.Namespace}).NamespacedIDString()

	transactionRequestPath := fmt.Sprintf("/api/v1/transactions/%s", txnID)
	client := c.client
	var resErr common.BlockchainRESTError
	var statusResponse fftypes.JSONObject
	res, err := client.R().
		SetContext(ctx).
		SetError(&resErr).
		SetResult(&statusResponse).
		Get(transactionRequestPath)
	if err != nil || !res.IsSuccess() {
		if res.StatusCode() == 404 {
			return nil, nil
		}
		return nil, common.WrapRESTError(ctx, &resErr, res, err, coremsgs.MsgCardanoconnectRESTErr)
	}

	return statusResponse, nil
}

func (c *Cardano) recoverFFI(ctx context.Context, parsedMethod interface{}) (*fftypes.FFIMethod, []*fftypes.FFIError, error) {
	methodInfo, ok := parsedMethod.(*ffiMethodAndErrors)
	if !ok || methodInfo.method == nil || methodInfo.method.Name == "" {
		return nil, nil, i18n.NewError(ctx, coremsgs.MsgUnexpectedInterfaceType, parsedMethod)
	}
	return methodInfo.method, methodInfo.errors, nil
}

func (c *Cardano) eventLoop(namespace string) {
	topic := c.getTopic(namespace)
	wsconn := c.wsconns[namespace]
	closed := c.closed[namespace]

	defer wsconn.Close()
	defer close(closed)
	l := log.L(c.ctx).WithField("role", "event-loop")
	ctx := log.WithLogger(c.ctx, l)
	for {
		select {
		case <-ctx.Done():
			l.Debugf("Event loop exiting (context cancelled)")
			return
		case msgBytes, ok := <-wsconn.Receive():
			if !ok {
				l.Debugf("Event loop exiting (receive channel closed). Terminating server!")
				c.cancelCtx()
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
				err = c.handleMessageBatch(ctx, namespace, 0, msgTyped)
				if err == nil {
					ack, _ := json.Marshal(&cardanoWSCommandPayload{
						Type:  "ack",
						Topic: topic,
					})
					err = wsconn.Send(ctx, ack)
				}
			case map[string]interface{}:
				if batchNumber, ok := msgTyped["batchNumber"].(float64); ok {
					if events, ok := msgTyped["events"].([]interface{}); ok {
						// FFTM delivery with a batch number to use in the ack
						err = c.handleMessageBatch(ctx, namespace, (int64)(batchNumber), events)
						// Errors processing messages are converted into nacks
						ackOrNack := &cardanoWSCommandPayload{
							Topic:       topic,
							BatchNumber: int64(batchNumber),
						}
						if err == nil {
							ackOrNack.Type = "ack"
						} else {
							log.L(ctx).Errorf("Rejecting batch due error: %s", err)
							ackOrNack.Type = "error"
							ackOrNack.Message = err.Error()
						}
						b, _ := json.Marshal(&ackOrNack)
						err = wsconn.Send(ctx, b)
					}
				} else {
					l.Errorf("Message unexpected: %+v", msgTyped)
				}
			default:
				l.Errorf("Message unexpected: %+v", msgTyped)
				continue
			}

			if err != nil {
				l.Errorf("Event loop exiting (%s). Terminating server!", err)
				c.cancelCtx()
				return
			}
		}
	}
}

func (c *Cardano) handleMessageBatch(ctx context.Context, namespace string, batchID int64, messages []interface{}) error {
	events := make(common.EventsToDispatch)
	updates := make([]*core.OperationUpdate, 0)
	count := len(messages)
	for i, msgI := range messages {
		msgMap, ok := msgI.(map[string]interface{})
		if !ok {
			log.L(ctx).Errorf("Message cannot be parsed as JSON: %+v", msgI)
			return nil
		}
		msgJSON := fftypes.JSONObject(msgMap)

		switch msgJSON.GetString("type") {
		case "ContractEvent":
			signature := msgJSON.GetString("signature")

			logger := log.L(ctx)
			logger.Infof("[%d:%d/%d]: '%s'", batchID, i+1, count, signature)
			logger.Tracef("Message: %+v", msgJSON)
			c.processContractEvent(ctx, namespace, events, msgJSON)
		case "Receipt":
			var receipt common.BlockchainReceiptNotification
			msgBytes, _ := json.Marshal(msgMap)
			_ = json.Unmarshal(msgBytes, &receipt)

			err := common.AddReceiptToBatch(ctx, namespace, c, &receipt, &updates)
			if err != nil {
				log.L(ctx).Errorf("Failed to process receipt: %+v", msgMap)
			}
		default:
			log.L(ctx).Errorf("Unexpected message in batch: %+v", msgMap)
		}

	}

	if len(updates) > 0 {
		err := c.callbacks.BulkOperationUpdates(ctx, namespace, updates)
		if err != nil {
			return err
		}
	}
	// Dispatch all the events from this patch that were successfully parsed and routed to namespaces
	// (could be zero - that's ok)
	return c.callbacks.DispatchBlockchainEvents(ctx, events)
}

func (c *Cardano) processContractEvent(ctx context.Context, namespace string, events common.EventsToDispatch, msgJSON fftypes.JSONObject) {
	listenerID := msgJSON.GetString("listenerId")
	event := c.parseBlockchainEvent(ctx, msgJSON)
	if event != nil {
		c.callbacks.PrepareBlockchainEvent(ctx, events, namespace, &blockchain.EventForListener{
			Event:      event,
			ListenerID: listenerID,
		})
	}
}

func (c *Cardano) parseBlockchainEvent(ctx context.Context, msgJSON fftypes.JSONObject) *blockchain.Event {
	sBlockNumber := msgJSON.GetString("blockNumber")
	sTransactionHash := msgJSON.GetString("transactionHash")
	blockNumber := msgJSON.GetInt64("blockNumber")
	txIndex := msgJSON.GetInt64("transactionIndex")
	logIndex := msgJSON.GetInt64("logIndex")
	dataJSON := msgJSON.GetObject("data")
	signature := msgJSON.GetString("signature")
	name := strings.SplitN(signature, "(", 2)[0]
	timestampStr := msgJSON.GetString("timestamp")
	timestamp, err := fftypes.ParseTimeString(timestampStr)
	if err != nil {
		log.L(ctx).Errorf("Blockchain event is not valid - missing timestamp: %+v", msgJSON)
		return nil // move on
	}

	if sBlockNumber == "" || sTransactionHash == "" {
		log.L(ctx).Errorf("Blockchain event is not valid - missing data: %+v", msgJSON)
		return nil // move on
	}

	delete(msgJSON, "data")
	return &blockchain.Event{
		BlockchainTXID: sTransactionHash,
		Source:         c.Name(),
		Name:           name,
		ProtocolID:     fmt.Sprintf("%.12d/%.6d/%.6d", blockNumber, txIndex, logIndex),
		Output:         dataJSON,
		Info:           msgJSON,
		Timestamp:      timestamp,
		Location:       c.buildEventLocationString(msgJSON),
		Signature:      signature,
	}
}

func (c *Cardano) buildEventLocationString(msgJSON fftypes.JSONObject) string {
	return fmt.Sprintf("address=%s", msgJSON.GetString("address"))
}

func formatCardanoAddress(ctx context.Context, key string) (string, error) {
	// TODO: could check for valid bech32, instead of just a conventional HRP
	if addressVerify.MatchString(key) {
		return key, nil
	}
	return "", i18n.NewError(ctx, coremsgs.MsgInvalidCardanoAddress)
}
