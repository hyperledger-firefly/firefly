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

package client

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/gorilla/websocket"
	"github.com/hyperledger/firefly-common/pkg/fftypes"
	"github.com/hyperledger/firefly/pkg/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	urlGetNamespaces     = "/namespaces"
	urlUploadData        = "/data"
	urlGetMessages       = "/messages"
	urlBroadcastMessage  = "/messages/broadcast"
	urlPrivateMessage    = "/messages/private"
	urlRequestMessage    = "/messages/requestreply"
	urlGetData           = "/data"
	urlGetDataBlob       = "/data/%s/blob"
	urlGetEvents         = "/events"
	urlSubscriptions     = "/subscriptions"
	urlDatatypes         = "/datatypes"
	urlIdentities        = "/identities"
	urlIdentity          = "/identities/%s"
	urlVerifiers         = "/verifiers"
	urlTokenPools        = "/tokens/pools"
	urlTokenMint         = "/tokens/mint"
	urlTokenBurn         = "/tokens/burn"
	urlTokenTransfers    = "/tokens/transfers"
	urlTokenApprovals    = "/tokens/approvals"
	urlTokenAccounts     = "/tokens/accounts"
	urlTokenBalances     = "/tokens/balances"
	urlContractInvoke    = "/contracts/invoke"
	urlContractQuery     = "/contracts/query"
	urlContractInterface = "/contracts/interfaces"
	urlContractGenerate  = "/contracts/interfaces/generate"
	urlContractListeners = "/contracts/listeners"
	urlContractAPI       = "/apis"
	urlBlockchainEvents  = "/blockchainevents"
	urlOperations        = "/operations"
	urlGetOrganizations  = "/network/organizations"
	urlOrganizationsSelf = "/network/organizations/self"
	urlNodesSelf         = "/network/nodes/self"
	urlNetworkAction     = "/network/action"
	urlGetOrgKeys        = "/identities/%s/verifiers"
	urlStatus            = "/status"
)

type Logger interface {
	Logf(format string, args ...interface{})
}

type FireFlyClient struct {
	logger    Logger
	Hostname  string
	Namespace string
	Client    *resty.Client
}

func NewFireFly(l Logger, hostname, namespace string) *FireFlyClient {
	client := NewResty(l)
	client.SetBaseURL(hostname + "/api/v1")
	return &FireFlyClient{
		logger:    l,
		Hostname:  hostname,
		Namespace: namespace,
		Client:    client,
	}
}

func NewResty(l Logger) *resty.Client {
	client := resty.New()
	client.OnBeforeRequest(func(c *resty.Client, req *resty.Request) error {
		var b []byte
		if _, isReader := req.Body.(io.Reader); !isReader {
			b, _ = json.Marshal(req.Body)
		}
		l.Logf("%s: ==> %s %s %s: %s", fftypes.Now(), req.Method, req.URL, req.QueryParam, string(b))
		return nil
	})
	client.OnAfterResponse(func(c *resty.Client, resp *resty.Response) error {
		if resp == nil {
			return nil
		}
		l.Logf("%s: <== %d", fftypes.Now(), resp.StatusCode())
		if resp.IsError() {
			l.Logf("<!! %s", resp.String())
			l.Logf("Headers: %+v", resp.Header())
		}
		return nil
	})
	return client
}

func (client *FireFlyClient) namespaced(url string) string {
	return "/namespaces/" + client.Namespace + url
}

func (client *FireFlyClient) WebSocket(t *testing.T, query string, authHeader http.Header) *websocket.Conn {
	u, _ := url.Parse(client.Hostname)
	scheme := "ws"
	if strings.Contains(client.Hostname, "https") {
		scheme = "wss"
	}
	wsURL := url.URL{
		Scheme:   scheme,
		Host:     u.Host,
		Path:     "/ws",
		RawQuery: query,
	}
	ws, resp, err := websocket.DefaultDialer.Dial(wsURL.String(), authHeader)
	require.NoError(t, err)
	if resp != nil {
		resp.Body.Close()
	}
	return ws
}

func (client *FireFlyClient) GetNamespaces() (*resty.Response, error) {
	return client.Client.R().
		SetResult(&[]core.Namespace{}).
		Get(urlGetNamespaces)
}

func (client *FireFlyClient) GetMessageEvents(t *testing.T, startTime time.Time, topic string, expectedStatus int) (events []*core.EnrichedEvent) {
	path := client.namespaced(urlGetEvents)
	resp, err := client.Client.R().
		SetQueryParam("created", fmt.Sprintf(">%d", startTime.UnixNano())).
		SetQueryParam("topic", topic).
		SetQueryParam("sort", "sequence").
		SetQueryParam("fetchreferences", "true").
		SetResult(&events).
		Get(path)
	require.NoError(t, err)
	require.Equal(t, expectedStatus, resp.StatusCode(), "GET %s [%d]: %s (count=%d)", path, resp.StatusCode(), resp.String(), len(events))
	return events
}

func (client *FireFlyClient) GetMessages(t *testing.T, startTime time.Time, msgType core.MessageType, topic string) (msgs []*core.Message) {
	path := client.namespaced(urlGetMessages)
	resp, err := client.Client.R().
		SetQueryParam("type", string(msgType)).
		SetQueryParam("created", fmt.Sprintf(">%d", startTime.UnixNano())).
		SetQueryParam("topics", topic).
		SetQueryParam("sort", "confirmed").
		SetResult(&msgs).
		Get(path)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode(), "GET %s [%d]: %s", path, resp.StatusCode(), resp.String())
	return msgs
}

func (client *FireFlyClient) GetData(t *testing.T, startTime time.Time) (data core.DataArray) {
	path := client.namespaced(urlGetData)
	resp, err := client.Client.R().
		SetQueryParam("created", fmt.Sprintf(">%d", startTime.UnixNano())).
		SetResult(&data).
		Get(path)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode(), "GET %s [%d]: %s", path, resp.StatusCode(), resp.String())
	return data
}

func (client *FireFlyClient) GetDataByID(t *testing.T, id *fftypes.UUID) *core.Data {
	var data core.Data
	path := client.namespaced(urlGetData + "/" + id.String())
	resp, err := client.Client.R().
		SetResult(&data).
		Get(path)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode(), "GET %s [%d]: %s", path, resp.StatusCode(), resp.String())
	return &data
}

func (client *FireFlyClient) GetDataForMessage(t *testing.T, startTime time.Time, msgID *fftypes.UUID) (data core.DataArray) {
	path := client.namespaced(urlGetMessages)
	path += "/" + msgID.String() + "/data"
	resp, err := client.Client.R().
		SetQueryParam("created", fmt.Sprintf(">%d", startTime.UnixNano())).
		SetResult(&data).
		Get(path)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode(), "GET %s [%d]: %s", path, resp.StatusCode(), resp.String())
	return data
}

func (client *FireFlyClient) GetBlob(t *testing.T, data *core.Data, expectedStatus int) []byte {
	path := client.namespaced(fmt.Sprintf(urlGetDataBlob, data.ID))
	resp, err := client.Client.R().
		SetDoNotParseResponse(true).
		Get(path)
	require.NoError(t, err)
	require.Equal(t, expectedStatus, resp.StatusCode(), "GET %s [%d]: %s", path, resp.StatusCode(), resp.String())
	blob, err := io.ReadAll(resp.RawBody())
	require.NoError(t, err)
	return blob
}

func (client *FireFlyClient) GetOrgs(t *testing.T) (orgs []*core.Identity) {
	path := client.namespaced(urlGetOrganizations)
	resp, err := client.Client.R().
		SetQueryParam("sort", "created").
		SetResult(&orgs).
		Get(path)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode(), "GET %s [%d]: %s", path, resp.StatusCode(), resp.String())
	return orgs
}

func (client *FireFlyClient) RegisterSelfOrg(t *testing.T, confirm bool) {
	path := client.namespaced(urlOrganizationsSelf)
	input := make(map[string]interface{})
	resp, err := client.Client.R().
		SetQueryParam("confirm", strconv.FormatBool(confirm)).
		SetBody(input).
		Post(path)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode(), "POST %s [%d]: %s", path, resp.StatusCode(), resp.String())
}

func (client *FireFlyClient) RegisterSelfNode(t *testing.T, confirm bool) {
	path := client.namespaced(urlNodesSelf)
	input := make(map[string]interface{})
	resp, err := client.Client.R().
		SetQueryParam("confirm", strconv.FormatBool(confirm)).
		SetBody(input).
		Post(path)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode(), "POST %s [%d]: %s", path, resp.StatusCode(), resp.String())
}

func (client *FireFlyClient) GetIdentityBlockchainKeys(t *testing.T, identityID *fftypes.UUID, expectedStatus int) (verifiers []*core.Verifier) {
	path := client.namespaced(fmt.Sprintf(urlGetOrgKeys, identityID))
	resp, err := client.Client.R().
		SetQueryParam("type", fmt.Sprintf("!=%s", core.VerifierTypeFFDXPeerID)).
		SetResult(&verifiers).
		Get(path)
	require.NoError(t, err)
	require.Equal(t, expectedStatus, resp.StatusCode(), "GET %s [%d]: %s", path, resp.StatusCode(), resp.String())
	return verifiers
}

func (client *FireFlyClient) CreateSubscription(t *testing.T, input interface{}, expectedStatus int) *core.Subscription {
	path := client.namespaced(urlSubscriptions)
	var sub core.Subscription
	resp, err := client.Client.R().
		SetBody(input).
		SetResult(&sub).
		SetHeader("Content-Type", "application/json").
		Post(path)
	require.NoError(t, err)
	require.Equal(t, expectedStatus, resp.StatusCode(), "POST %s [%d]: %s", path, resp.StatusCode(), resp.String())
	return &sub
}

func (client *FireFlyClient) CleanupExistingSubscription(t *testing.T, namespace, name string) {
	var subs []*core.Subscription
	path := client.namespaced(urlSubscriptions)
	resp, err := client.Client.R().
		SetResult(&subs).
		Get(path)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode(), "GET %s [%d]: %s", path, resp.StatusCode(), resp.String())
	for _, s := range subs {
		if s.Namespace == namespace && s.Name == name {
			client.DeleteSubscription(t, s.ID)
		}
	}
}

func (client *FireFlyClient) DeleteSubscription(t *testing.T, id *fftypes.UUID) {
	path := client.namespaced(fmt.Sprintf("%s/%s", urlSubscriptions, id))
	resp, err := client.Client.R().Delete(path)
	require.NoError(t, err)
	require.Equal(t, 204, resp.StatusCode(), "DELETE %s [%d]: %s", path, resp.StatusCode(), resp.String())
}

func (client *FireFlyClient) BroadcastMessage(t *testing.T, topic, idempotencyKey string, data *core.DataRefOrValue, confirm bool) (*resty.Response, error) {
	return client.BroadcastMessageAsIdentity(t, "", topic, idempotencyKey, data, confirm)
}

func (client *FireFlyClient) BroadcastMessageAsIdentity(t *testing.T, did, topic, idempotencyKey string, data *core.DataRefOrValue, confirm bool) (*resty.Response, error) {
	var msg core.Message
	res, err := client.Client.R().
		SetBody(core.MessageInOut{
			Message: core.Message{
				Header: core.MessageHeader{
					Topics: fftypes.FFStringArray{topic},
					SignerRef: core.SignerRef{
						Author: did,
					},
				},
				IdempotencyKey: core.IdempotencyKey(idempotencyKey),
			},
			InlineData: core.InlineData{data},
		}).
		SetQueryParam("confirm", strconv.FormatBool(confirm)).
		SetResult(&msg).
		Post(client.namespaced(urlBroadcastMessage))
	client.logger.Logf("Sent broadcast msg: %s", msg.Header.ID)
	return res, err
}

func (client *FireFlyClient) ClaimCustomIdentity(t *testing.T, key, name, desc string, profile fftypes.JSONObject, parent *fftypes.UUID, confirm bool) *core.Identity {
	var identity core.Identity
	res, err := client.Client.R().
		SetBody(core.IdentityCreateDTO{
			Name:   name,
			Type:   core.IdentityTypeCustom,
			Parent: parent.String(),
			Key:    key,
			IdentityProfile: core.IdentityProfile{
				Description: desc,
				Profile:     profile,
			},
		}).
		SetQueryParam("confirm", strconv.FormatBool(confirm)).
		SetResult(&identity).
		Post(client.namespaced(urlIdentities))
	assert.NoError(t, err)
	assert.True(t, res.IsSuccess())
	client.logger.Logf("Identity creation initiated with key %s: %s (%s)", key, identity.ID, identity.DID)
	return &identity
}

func (client *FireFlyClient) GetIdentity(t *testing.T, id *fftypes.UUID) *core.Identity {
	var identity core.Identity
	res, err := client.Client.R().
		SetResult(&identity).
		Get(client.namespaced(fmt.Sprintf(urlIdentity, id)))
	assert.NoError(t, err)
	assert.True(t, res.IsSuccess())
	return &identity
}

func (client *FireFlyClient) GetOrganization(t *testing.T, idOrName string) *core.Identity {
	var identity core.Identity
	res, err := client.Client.R().
		SetResult(&identity).
		Get(client.namespaced(fmt.Sprintf("%s/%s", urlGetOrganizations, idOrName)))
	assert.NoError(t, err)
	if res.StatusCode() == http.StatusNotFound {
		return nil
	}
	assert.True(t, res.IsSuccess())
	return &identity
}

func (client *FireFlyClient) GetVerifiers(t *testing.T) []*core.Verifier {
	var verifiers []*core.Verifier
	res, err := client.Client.R().
		SetResult(&verifiers).
		Get(client.namespaced(urlVerifiers))
	assert.NoError(t, err)
	assert.True(t, res.IsSuccess())
	return verifiers
}

func (client *FireFlyClient) CreateBlob(t *testing.T, dt *core.DatatypeRef) *core.Data {
	r, _ := rand.Int(rand.Reader, big.NewInt(1024*1024))
	blob := make([]byte, r.Int64()+1024*1024)
	for i := 0; i < len(blob); i++ {
		blob[i] = byte('a' + i%26)
	}
	var blobHash fftypes.Bytes32 = sha256.Sum256(blob)
	client.logger.Logf("Blob size=%d hash=%s", len(blob), &blobHash)
	var data core.Data
	formData := map[string]string{}
	if dt == nil {
		// If there's no datatype, tell FireFly to automatically add a data payload
		formData["autometa"] = "true"
		formData["metadata"] = `{"mymeta": "data"}`
	} else {
		// Otherwise use a tagging only approach, where we allow a nil value, specify that this should
		// not be validated, but still set a datatype for classification of the data.
		formData["validator"] = "none"
		formData["datatype.name"] = dt.Name
		formData["datatype.version"] = dt.Version
	}
	path := client.namespaced(urlUploadData)
	resp, err := client.Client.R().
		SetFormData(formData).
		SetFileReader("file", "myfile.txt", bytes.NewReader(blob)).
		SetResult(&data).
		Post(path)
	require.NoError(t, err)
	require.Equal(t, 201, resp.StatusCode(), "POST %s [%d]: %s", path, resp.StatusCode(), resp.String())
	client.logger.Logf("Data created: %s", data.ID)
	if dt == nil {
		assert.Equal(t, "data", data.Value.JSONObject().GetString("mymeta"))
		assert.Equal(t, "myfile.txt", data.Value.JSONObject().GetString("filename"))
		assert.Equal(t, "myfile.txt", data.Blob.Name)
	} else {
		assert.Equal(t, core.ValidatorTypeNone, data.Validator)
		assert.Equal(t, *dt, *data.Datatype)
	}
	assert.Equal(t, int64(len(blob)), data.Blob.Size)
	assert.Equal(t, blobHash, *data.Blob.Hash)
	return &data
}

func (client *FireFlyClient) BroadcastBlobMessage(t *testing.T, topic string) (*core.Data, *resty.Response, error) {
	data := client.CreateBlob(t, nil)
	res, err := client.Client.R().
		SetBody(core.MessageInOut{
			Message: core.Message{
				Header: core.MessageHeader{
					Topics: fftypes.FFStringArray{topic},
				},
			},
			InlineData: core.InlineData{
				{DataRef: core.DataRef{ID: data.ID}},
			},
		}).
		Post(client.namespaced(urlBroadcastMessage))
	return data, res, err
}

func (client *FireFlyClient) PrivateBlobMessageDatatypeTagged(t *testing.T, topic string, members []core.MemberInput, startTime time.Time) (*core.Data, *resty.Response, error) {
	data := client.CreateBlob(t, &core.DatatypeRef{Name: "myblob"})
	res, err := client.Client.R().
		SetBody(core.MessageInOut{
			Message: core.Message{
				Header: core.MessageHeader{
					Topics: fftypes.FFStringArray{topic},
				},
			},
			InlineData: core.InlineData{
				{DataRef: core.DataRef{ID: data.ID}},
			},
			Group: &core.InputGroup{
				Members: members,
				Name:    fmt.Sprintf("test_%d", startTime.UnixNano()),
			},
		}).
		Post(client.namespaced(urlPrivateMessage))
	return data, res, err
}

func (client *FireFlyClient) PrivateMessage(topic, idempotencyKey string, data *core.DataRefOrValue, members []core.MemberInput, tag string, txType core.TransactionType, confirm bool, startTime time.Time) (*resty.Response, error) {
	return client.PrivateMessageWithKey("", topic, idempotencyKey, data, members, tag, txType, confirm, startTime)
}

func (client *FireFlyClient) PrivateMessageWithKey(key, topic, idempotencyKey string, data *core.DataRefOrValue, members []core.MemberInput, tag string, txType core.TransactionType, confirm bool, startTime time.Time) (*resty.Response, error) {
	msg := core.MessageInOut{
		Message: core.Message{
			Header: core.MessageHeader{
				Tag:    tag,
				TxType: txType,
				Topics: fftypes.FFStringArray{topic},
				SignerRef: core.SignerRef{
					Key: key,
				},
			},
			IdempotencyKey: core.IdempotencyKey(idempotencyKey),
		},
		InlineData: core.InlineData{data},
		Group: &core.InputGroup{
			Members: members,
			Name:    fmt.Sprintf("test_%d", startTime.UnixNano()),
		},
	}
	res, err := client.Client.R().
		SetBody(msg).
		SetQueryParam("confirm", strconv.FormatBool(confirm)).
		SetResult(&msg.Message).
		Post(client.namespaced(urlPrivateMessage))
	client.logger.Logf("Sent private message %s to %+v", msg.Header.ID, msg.Group.Members)
	return res, err
}

func (client *FireFlyClient) RequestReply(t *testing.T, data *core.DataRefOrValue, orgNames []string, tag string, txType core.TransactionType, startTime time.Time) *core.MessageInOut {
	members := make([]core.MemberInput, len(orgNames))
	for i, oName := range orgNames {
		// We let FireFly resolve the friendly name of the org to the identity
		members[i] = core.MemberInput{
			Identity: oName,
		}
	}
	msg := core.MessageInOut{
		Message: core.Message{
			Header: core.MessageHeader{
				Tag:    tag,
				TxType: txType,
			},
		},
		InlineData: core.InlineData{data},
		Group: &core.InputGroup{
			Members: members,
			Name:    fmt.Sprintf("test_%d", startTime.UnixNano()),
		},
	}
	path := client.namespaced(urlRequestMessage)
	var replyMsg core.MessageInOut
	resp, err := client.Client.R().
		SetBody(msg).
		SetResult(&replyMsg).
		Post(path)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode(), "POST %s [%d]: %s", path, resp.StatusCode(), resp.String())
	return &replyMsg
}

func (client *FireFlyClient) CreateDatatype(t *testing.T, datatype *core.Datatype, confirm bool) *core.Datatype {
	var dtReturn core.Datatype
	path := client.namespaced(urlDatatypes)
	resp, err := client.Client.R().
		SetBody(datatype).
		SetQueryParam("confirm", strconv.FormatBool(confirm)).
		SetResult(&dtReturn).
		Post(path)
	require.NoError(t, err)
	expected := 202
	if confirm {
		expected = 200
	}
	require.Equal(t, expected, resp.StatusCode(), "POST %s [%d]: %s", path, resp.StatusCode(), resp.String())
	return &dtReturn
}

func (client *FireFlyClient) CreateTokenPool(t *testing.T, pool *core.TokenPool, publish, confirm bool) *core.TokenPool {
	var poolOut core.TokenPool
	path := client.namespaced(urlTokenPools)
	resp, err := client.Client.R().
		SetBody(pool).
		SetQueryParam("confirm", strconv.FormatBool(confirm)).
		SetQueryParam("publish", strconv.FormatBool(publish)).
		SetResult(&poolOut).
		Post(path)
	require.NoError(t, err)
	expected := 202
	if confirm {
		expected = 200
	}
	require.Equal(t, expected, resp.StatusCode(), "POST %s [%d]: %s", path, resp.StatusCode(), resp.String())
	return &poolOut
}

func (client *FireFlyClient) PublishTokenPool(t *testing.T, poolID *fftypes.UUID, networkName string, confirm bool) {
	path := client.namespaced(urlTokenPools + "/" + poolID.String() + "/publish")
	resp, err := client.Client.R().
		SetBody(&core.DefinitionPublish{
			NetworkName: networkName,
		}).
		SetQueryParam("confirm", strconv.FormatBool(confirm)).
		Post(path)
	require.NoError(t, err)
	expected := 202
	if confirm {
		expected = 200
	}
	require.Equal(t, expected, resp.StatusCode(), "POST %s [%d]: %s", path, resp.StatusCode(), resp.String())
}

func (client *FireFlyClient) GetTokenPools(t *testing.T, startTime time.Time) (pools []*core.TokenPool) {
	path := client.namespaced(urlTokenPools)
	resp, err := client.Client.R().
		SetQueryParam("created", fmt.Sprintf(">%d", startTime.UnixNano())).
		SetResult(&pools).
		Get(path)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode(), "GET %s [%d]: %s", path, resp.StatusCode(), resp.String())
	return pools
}

func (client *FireFlyClient) GetTokenPool(t *testing.T, poolID *fftypes.UUID) (pool *core.TokenPool) {
	path := client.namespaced(urlTokenPools + "/" + poolID.String())
	resp, err := client.Client.R().
		SetResult(&pool).
		Get(path)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode(), "GET %s [%d]: %s", path, resp.StatusCode(), resp.String())
	return pool
}

func (client *FireFlyClient) DeleteTokenPool(t *testing.T, poolID *fftypes.UUID, expectedStatus int) {
	path := client.namespaced(urlTokenPools + "/" + poolID.String())
	resp, err := client.Client.R().Delete(path)
	require.NoError(t, err)
	require.Equal(t, expectedStatus, resp.StatusCode(), "DELETE %s [%d]: %s", path, resp.StatusCode(), resp.String())
}

func (client *FireFlyClient) MintTokens(t *testing.T, mint *core.TokenTransferInput, confirm bool, expectedStatus ...int) *core.TokenTransfer {
	var transferOut core.TokenTransfer
	path := client.namespaced(urlTokenMint)
	resp, err := client.Client.R().
		SetBody(mint).
		SetQueryParam("confirm", strconv.FormatBool(confirm)).
		SetResult(&transferOut).
		Post(path)
	require.NoError(t, err)
	if len(expectedStatus) == 0 {
		if confirm {
			expectedStatus = []int{200}
		} else {
			expectedStatus = []int{202}
		}
	}
	require.Equal(t, expectedStatus[0], resp.StatusCode(), "POST %s [%d]: %s", path, resp.StatusCode(), resp.String())
	return &transferOut
}

func (client *FireFlyClient) BurnTokens(t *testing.T, burn *core.TokenTransferInput, confirm bool, expectedStatus ...int) *core.TokenTransfer {
	var transferOut core.TokenTransfer
	path := client.namespaced(urlTokenBurn)
	resp, err := client.Client.R().
		SetBody(burn).
		SetQueryParam("confirm", strconv.FormatBool(confirm)).
		SetResult(&transferOut).
		Post(path)
	require.NoError(t, err)
	if len(expectedStatus) == 0 {
		if confirm {
			expectedStatus = []int{200}
		} else {
			expectedStatus = []int{202}
		}
	}
	require.Equal(t, expectedStatus[0], resp.StatusCode(), "POST %s [%d]: %s", path, resp.StatusCode(), resp.String())
	return &transferOut
}

func (client *FireFlyClient) TransferTokens(t *testing.T, transfer *core.TokenTransferInput, confirm bool, expectedStatus ...int) *core.TokenTransfer {
	var transferOut core.TokenTransfer
	path := client.namespaced(urlTokenTransfers)
	resp, err := client.Client.R().
		SetBody(transfer).
		SetQueryParam("confirm", strconv.FormatBool(confirm)).
		SetResult(&transferOut).
		Post(path)
	require.NoError(t, err)
	if len(expectedStatus) == 0 {
		if confirm {
			expectedStatus = []int{200}
		} else {
			expectedStatus = []int{202}
		}
	}
	require.Equal(t, expectedStatus[0], resp.StatusCode(), "POST %s [%d]: %s", path, resp.StatusCode(), resp.String())
	return &transferOut
}

func (client *FireFlyClient) GetTokenTransfers(t *testing.T, poolID *fftypes.UUID) (transfers []*core.TokenTransfer) {
	path := client.namespaced(urlTokenTransfers)
	resp, err := client.Client.R().
		SetQueryParam("pool", poolID.String()).
		SetResult(&transfers).
		Get(path)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode(), "GET %s [%d]: %s", path, resp.StatusCode(), resp.String())
	return transfers
}

func (client *FireFlyClient) TokenApproval(t *testing.T, approval *core.TokenApprovalInput, confirm bool, expectedStatus ...int) *core.TokenApproval {
	var approvalOut core.TokenApproval
	path := client.namespaced(urlTokenApprovals)
	resp, err := client.Client.R().
		SetBody(approval).
		SetQueryParam("confirm", strconv.FormatBool(confirm)).
		SetResult(&approvalOut).
		Post(path)
	require.NoError(t, err)
	if len(expectedStatus) == 0 {
		if confirm {
			expectedStatus = []int{200}
		} else {
			expectedStatus = []int{202}
		}
	}
	require.Equal(t, expectedStatus[0], resp.StatusCode(), "POST %s [%d]: %s", path, resp.StatusCode(), resp.String())
	return &approvalOut
}

func (client *FireFlyClient) GetTokenApprovals(t *testing.T, poolID *fftypes.UUID) (approvals []*core.TokenApproval) {
	path := client.namespaced(urlTokenApprovals)
	resp, err := client.Client.R().
		SetQueryParam("pool", poolID.String()).
		SetQueryParam("active", "true").
		SetResult(&approvals).
		Get(path)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode(), "GET %s [%d]: %s", path, resp.StatusCode(), resp.String())
	return approvals
}

func (client *FireFlyClient) GetTokenAccounts(t *testing.T, poolID *fftypes.UUID) (accounts []*core.TokenAccount) {
	path := client.namespaced(urlTokenAccounts)
	resp, err := client.Client.R().
		SetResult(&accounts).
		Get(path)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode(), "GET %s [%d]: %s", path, resp.StatusCode(), resp.String())
	return accounts
}

func (client *FireFlyClient) GetTokenAccountPools(t *testing.T, identity string) (pools []*core.TokenAccountPool) {
	path := client.namespaced(urlTokenAccounts + "/" + identity + "/pools")
	resp, err := client.Client.R().
		SetQueryParam("sort", "-updated").
		SetResult(&pools).
		Get(path)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode(), "GET %s [%d]: %s", path, resp.StatusCode(), resp.String())
	return pools
}

func (client *FireFlyClient) GetTokenBalance(t *testing.T, poolID *fftypes.UUID, tokenIndex, key string) (account *core.TokenBalance) {
	var accounts []*core.TokenBalance
	path := client.namespaced(urlTokenBalances)
	resp, err := client.Client.R().
		SetQueryParam("pool", poolID.String()).
		SetQueryParam("tokenIndex", tokenIndex).
		SetQueryParam("key", key).
		SetResult(&accounts).
		Get(path)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode(), "GET %s [%d]: %s", path, resp.StatusCode(), resp.String())
	require.Equal(t, 1, len(accounts))
	return accounts[0]
}

func (client *FireFlyClient) CreateContractListener(t *testing.T, event *fftypes.FFIEvent, location *fftypes.JSONObject) *core.ContractListener {
	body := core.ContractListenerInput{
		ContractListener: core.ContractListener{
			Location: fftypes.JSONAnyPtr(location.String()),
			Event: &core.FFISerializedEvent{
				FFIEventDefinition: event.FFIEventDefinition,
			},
			Topic: "firefly-e2e",
		},
	}
	var sub core.ContractListener
	path := client.namespaced(urlContractListeners)
	resp, err := client.Client.R().
		SetBody(&body).
		SetResult(&sub).
		Post(path)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode(), "POST %s [%d]: %s", path, resp.StatusCode(), resp.String())
	return &sub
}

func (client *FireFlyClient) CreateFFIContractListener(t *testing.T, ffiReference *fftypes.FFIReference, eventPath string, location *fftypes.JSONObject) *core.ContractListener {
	body := core.ContractListenerInput{
		ContractListener: core.ContractListener{
			Location:  fftypes.JSONAnyPtr(location.String()),
			Interface: ffiReference,
			Topic:     "firefly-e2e",
		},
		EventPath: eventPath,
	}
	var listener core.ContractListener
	path := client.namespaced(urlContractListeners)
	resp, err := client.Client.R().
		SetBody(&body).
		SetResult(&listener).
		Post(path)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode(), "POST %s [%d]: %s", path, resp.StatusCode(), resp.String())
	return &listener
}

func (client *FireFlyClient) GetContractListeners(t *testing.T, startTime time.Time) (subs []*core.ContractListener) {
	path := client.namespaced(urlContractListeners)
	resp, err := client.Client.R().
		SetQueryParam("created", fmt.Sprintf(">%d", startTime.UnixNano())).
		SetResult(&subs).
		Get(path)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode(), "GET %s [%d]: %s", path, resp.StatusCode(), resp.String())
	return subs
}

func (client *FireFlyClient) GetContractEvents(t *testing.T, startTime time.Time, subscriptionID *fftypes.UUID) (events []*core.BlockchainEvent) {
	path := client.namespaced(urlBlockchainEvents)
	resp, err := client.Client.R().
		SetQueryParam("timestamp", fmt.Sprintf(">%d", startTime.UnixNano())).
		SetQueryParam("subscriptionId", subscriptionID.String()).
		SetResult(&events).
		Get(path)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode(), "GET %s [%d]: %s", path, resp.StatusCode(), resp.String())
	return events
}

func (client *FireFlyClient) DeleteContractListener(t *testing.T, id *fftypes.UUID) {
	path := client.namespaced(urlContractListeners + "/" + id.String())
	resp, err := client.Client.R().Delete(path)
	require.NoError(t, err)
	require.Equal(t, 204, resp.StatusCode(), "DELETE %s [%d]: %s", path, resp.StatusCode(), resp.String())
}

func (client *FireFlyClient) InvokeContractMethod(t *testing.T, req *core.ContractCallRequest, expectedStatus ...int) (*core.Operation, error) {
	res := &core.Operation{}
	path := client.namespaced(urlContractInvoke)
	var errResult fftypes.RESTError
	resp, err := client.Client.R().
		SetBody(req).
		SetResult(res).
		SetError(&errResult).
		Post(path)
	require.NoError(t, err)
	if len(expectedStatus) == 0 {
		expectedStatus = []int{202}
	}
	require.Equal(t, expectedStatus[0], resp.StatusCode(), "POST %s [%d]: %s", path, resp.StatusCode(), resp.String())
	if err == nil && errResult.Error != "" {
		return nil, errors.New(errResult.Error)
	}
	return res, err
}

func (client *FireFlyClient) QueryContractMethod(t *testing.T, req *core.ContractCallRequest) (interface{}, error) {
	var res interface{}
	path := client.namespaced(urlContractQuery)
	resp, err := client.Client.R().
		SetBody(req).
		SetResult(&res).
		Post(path)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode(), "POST %s [%d]: %s", path, resp.StatusCode(), resp.String())
	return res, err
}

func (client *FireFlyClient) GenerateFFIFromABI(t *testing.T, req *fftypes.FFIGenerationRequest) *fftypes.FFI {
	var res fftypes.FFI
	path := client.namespaced(urlContractGenerate)
	resp, err := client.Client.R().
		SetBody(req).
		SetResult(&res).
		Post(path)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode(), "POST %s [%d]: %s", path, resp.StatusCode(), resp.String())
	return &res
}

func (client *FireFlyClient) CreateFFI(t *testing.T, ffi *fftypes.FFI, publish bool) (*fftypes.FFI, error) {
	var res fftypes.FFI
	path := client.namespaced(urlContractInterface)
	resp, err := client.Client.R().
		SetBody(ffi).
		SetResult(&res).
		SetQueryParam("confirm", "true").
		SetQueryParam("publish", strconv.FormatBool(publish)).
		Post(path)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode(), "POST %s [%d]: %s", path, resp.StatusCode(), resp.String())
	return &res, err
}

func (client *FireFlyClient) GetFFI(t *testing.T, name, version string) (result *fftypes.FFI) {
	path := client.namespaced(urlContractInterface + "/" + name + "/" + version)
	resp, err := client.Client.R().
		SetResult(&result).
		Get(path)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode(), "GET %s [%d]: %s", path, resp.StatusCode(), resp.String())
	return result
}

func (client *FireFlyClient) PublishFFI(t *testing.T, name, version, networkName string, confirm bool) {
	path := client.namespaced(urlContractInterface + "/" + name + "/" + version + "/publish")
	resp, err := client.Client.R().
		SetBody(&core.DefinitionPublish{
			NetworkName: networkName,
		}).
		SetQueryParam("confirm", strconv.FormatBool(confirm)).
		Post(path)
	require.NoError(t, err)
	expected := 202
	if confirm {
		expected = 200
	}
	require.Equal(t, expected, resp.StatusCode(), "POST %s [%d]: %s", path, resp.StatusCode(), resp.String())
}

func (client *FireFlyClient) DeleteFFI(t *testing.T, id *fftypes.UUID, expectedStatus int) {
	path := client.namespaced(urlContractInterface + "/" + id.String())
	resp, err := client.Client.R().Delete(path)
	require.NoError(t, err)
	require.Equal(t, expectedStatus, resp.StatusCode(), "DELETE %s [%d]: %s", path, resp.StatusCode(), resp.String())
}

func (client *FireFlyClient) CreateContractAPI(t *testing.T, name string, ffiReference *fftypes.FFIReference, location *fftypes.JSONAny, publish bool) (*core.ContractAPI, error) {
	apiReqBody := &core.ContractAPI{
		Name:      name,
		Interface: ffiReference,
		Location:  location,
	}

	var res core.ContractAPI
	path := client.namespaced(urlContractAPI)
	resp, err := client.Client.R().
		SetBody(apiReqBody).
		SetResult(&res).
		SetQueryParam("confirm", "true").
		SetQueryParam("publish", strconv.FormatBool(publish)).
		Post(path)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode(), "POST %s [%d]: %s", path, resp.StatusCode(), resp.String())
	return &res, err
}

func (client *FireFlyClient) GetContractAPI(t *testing.T, name string) (result *core.ContractAPI) {
	path := client.namespaced(urlContractAPI + "/" + name)
	resp, err := client.Client.R().
		SetResult(&result).
		Get(path)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode(), "GET %s [%d]: %s", path, resp.StatusCode(), resp.String())
	return result
}

func (client *FireFlyClient) PublishContractAPI(t *testing.T, name, networkName string, confirm bool) {
	path := client.namespaced(urlContractAPI + "/" + name + "/publish")
	resp, err := client.Client.R().
		SetBody(&core.DefinitionPublish{
			NetworkName: networkName,
		}).
		SetQueryParam("confirm", strconv.FormatBool(confirm)).
		Post(path)
	require.NoError(t, err)
	expected := 202
	if confirm {
		expected = 200
	}
	require.Equal(t, expected, resp.StatusCode(), "POST %s [%d]: %s", path, resp.StatusCode(), resp.String())
}

func (client *FireFlyClient) DeleteContractAPI(t *testing.T, name string, expectedStatus int) {
	path := client.namespaced(urlContractAPI + "/" + name)
	resp, err := client.Client.R().Delete(path)
	require.NoError(t, err)
	require.Equal(t, expectedStatus, resp.StatusCode(), "DELETE %s [%d]: %s", path, resp.StatusCode(), resp.String())
}

func (client *FireFlyClient) InvokeContractAPIMethod(t *testing.T, apiName string, methodName string, input *fftypes.JSONAny) (interface{}, error) {
	apiReqBody := map[string]interface{}{
		"input": input,
	}
	var res interface{}
	path := client.namespaced(fmt.Sprintf("%s/%s/invoke/%s", urlContractAPI, apiName, methodName))
	resp, err := client.Client.R().
		SetBody(apiReqBody).
		SetResult(&res).
		SetQueryParam("confirm", "true").
		Post(path)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode(), "POST %s [%d]: %s", path, resp.StatusCode(), resp.String())
	return res, err
}

func (client *FireFlyClient) QueryContractAPIMethod(t *testing.T, apiName string, methodName string, input *fftypes.JSONAny) (interface{}, error) {
	apiReqBody := map[string]interface{}{
		"input": input,
	}
	var res interface{}
	path := client.namespaced(fmt.Sprintf("%s/%s/query/%s", urlContractAPI, apiName, methodName))
	resp, err := client.Client.R().
		SetBody(apiReqBody).
		SetResult(&res).
		Post(path)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode(), "POST %s [%d]: %s", path, resp.StatusCode(), resp.String())
	return res, err
}

func (client *FireFlyClient) CreateContractAPIListener(t *testing.T, apiName, eventName, topic string) (*core.ContractListener, error) {
	apiReqBody := map[string]interface{}{
		"topic": topic,
	}
	var listener core.ContractListener
	path := client.namespaced(fmt.Sprintf("%s/%s/listeners/%s", urlContractAPI, apiName, eventName))
	resp, err := client.Client.R().
		SetBody(apiReqBody).
		SetResult(&listener).
		Post(path)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode(), "POST %s [%d]: %s", path, resp.StatusCode(), resp.String())
	return &listener, err
}

func (client *FireFlyClient) GetEvent(t *testing.T, eventID string) *core.Event {
	var ev core.Event
	path := client.namespaced(fmt.Sprintf("%s/%s", urlGetEvents, eventID))
	resp, err := client.Client.R().
		SetResult(&ev).
		Get(path)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode(), "GET %s [%d]: %s", path, resp.StatusCode(), resp.String())
	return &ev
}

func (client *FireFlyClient) GetBlockchainEvents(t *testing.T, startTime time.Time) (events []*core.BlockchainEvent) {
	path := client.namespaced(urlBlockchainEvents)
	resp, err := client.Client.R().
		SetQueryParam("timestamp", fmt.Sprintf(">%d", startTime.UnixNano())).
		SetResult(&events).
		Get(path)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode(), "GET %s [%d]: %s", path, resp.StatusCode(), resp.String())
	return events
}

func (client *FireFlyClient) GetBlockchainEvent(t *testing.T, eventID string) *core.BlockchainEvent {
	var ev core.BlockchainEvent
	path := client.namespaced(fmt.Sprintf("%s/%s", urlBlockchainEvents, eventID))
	resp, err := client.Client.R().
		SetResult(&ev).
		Get(path)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode(), "GET %s [%d]: %s", path, resp.StatusCode(), resp.String())
	return &ev
}

func (client *FireFlyClient) GetOperations(t *testing.T, startTime time.Time) (operations []*core.Operation) {
	path := client.namespaced(urlOperations)
	resp, err := client.Client.R().
		SetQueryParam("created", fmt.Sprintf(">%d", startTime.UnixNano())).
		SetResult(&operations).
		Get(path)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode(), "GET %s [%d]: %s", path, resp.StatusCode(), resp.String())
	return operations
}

func (client *FireFlyClient) GetOperation(t *testing.T, operationID string) (operation *core.Operation) {
	path := client.namespaced(urlOperations + "/" + operationID)
	resp, err := client.Client.R().
		SetResult(&operation).
		Get(path)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode(), "GET %s [%d]: %s", path, resp.StatusCode(), resp.String())
	return operation
}

func (client *FireFlyClient) NetworkAction(t *testing.T, action core.NetworkActionType) {
	path := client.namespaced(urlNetworkAction)
	input := &core.NetworkAction{Type: action}
	resp, err := client.Client.R().
		SetBody(input).
		Post(path)
	require.NoError(t, err)
	require.Equal(t, 202, resp.StatusCode(), "POST %s [%d]: %s", path, resp.StatusCode(), resp.String())
}

func (client *FireFlyClient) GetStatus() (*core.NamespaceStatus, *resty.Response, error) {
	var status core.NamespaceStatus
	path := client.namespaced(urlStatus)
	resp, err := client.Client.R().
		SetResult(&status).
		Get(path)
	return &status, resp, err
}
