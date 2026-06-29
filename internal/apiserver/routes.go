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

package apiserver

import (
	"context"

	"github.com/hyperledger-firefly/common/pkg/ffapi"
	"github.com/hyperledger-firefly/firefly/internal/coreconfig"
	"github.com/hyperledger-firefly/firefly/internal/coremsgs"
	"github.com/hyperledger-firefly/firefly/internal/namespace"
	"github.com/hyperledger-firefly/firefly/internal/orchestrator"
)

type coreRequest struct {
	mgr        namespace.Manager
	or         orchestrator.Orchestrator
	ctx        context.Context
	apiBaseURL string
}

type coreExtensions struct {
	EnabledIf             func(or orchestrator.Orchestrator) bool
	CoreJSONHandler       func(r *ffapi.APIRequest, cr *coreRequest) (output interface{}, err error)
	CoreFormUploadHandler func(r *ffapi.APIRequest, cr *coreRequest) (output interface{}, err error)
}

const (
	routeTagGlobal              = "Global"
	routeTagDefaultNamespace    = "Default Namespace"
	routeTagNonDefaultNamespace = "Non-Default Namespace"
)

var nsRoutes = []*ffapi.Route{}
var routes = append(
	globalRoutes([]*ffapi.Route{
		getNamespace,
		getNamespaces,
		getWebSockets,
	}),
	namespacedRoutes([]*ffapi.Route{
		deleteContractAPI,
		deleteContractInterface,
		deleteContractListener,
		deleteData,
		deleteSubscription,
		deleteTokenPool,
		getBatchByID,
		getBatches,
		getBlockchainEventByID,
		getBlockchainEvents,
		getChartHistogram,
		getContractAPIByName,
		getContractAPIInterface,
		getContractAPIs,
		getContractAPIListeners,
		getContractInterface,
		getContractInterfaceNameVersion,
		getContractInterfaces,
		getContractListenerByNameOrID,
		getContractListeners,
		getData,
		getDataBlob,
		getDataSubPaths,
		getDataValue,
		getDataByID,
		getDataMsgs,
		getDatatypeByName,
		getDatatypes,
		getEventByID,
		getEvents,
		getGroupByHash,
		getGroups,
		getIdentities,
		getIdentityByDID,
		getIdentityByID,
		getIdentityDID,
		getIdentityVerifiers,
		getMsgByID,
		getMsgData,
		getMsgEvents,
		getMsgs,
		getMsgTxn,
		getNetworkDIDDocByDID,
		getNetworkIdentities,
		getNetworkIdentityByDID,
		getNetworkNode,
		getNetworkNodes,
		getNetworkOrg,
		getNetworkOrgs,
		getNextPins,
		getOpByID,
		getOps,
		getPins,
		getStatus,
		getStatusMultiparty,
		getStatusBatchManager,
		getSubscriptionByID,
		getSubscriptions,
		getSubscriptionEventsFiltered,
		getTokenAccountPools,
		getTokenAccounts,
		getTokenApprovals,
		getTokenBalances,
		getTokenConnectors,
		getTokenPoolByNameOrID,
		getTokenPools,
		getTokenTransferByID,
		getTokenTransfers,
		getTxnBlockchainEvents,
		getTxnByID,
		getTxnOps,
		getTxns,
		getTxnStatus,
		getVerifierByID,
		getVerifiers,
		patchUpdateIdentity,
		postBatchCancel,
		postContractAPIInvoke,
		postContractAPIPublish,
		postContractAPIQuery,
		postContractAPIListeners,
		postContractListenerSignature,
		postContractInterfaceGenerate,
		postContractInterfacePublish,
		postContractDeploy,
		postContractInvoke,
		postContractQuery,
		postData,
		postDataBlobPublish,
		postDataValuePublish,
		postNetworkAction,
		postNewContractAPI,
		postNewContractInterface,
		postNewContractListener,
		postNewDatatype,
		postNewIdentity,
		postNewMessageBroadcast,
		postNewMessagePrivate,
		postNewMessageRequestReply,
		postNewSubscription,
		postNewOrganization,
		postNewOrganizationSelf,
		postNodesSelf,
		postOpRetry,
		postPinsRewind,
		postTokenApproval,
		postTokenBurn,
		postTokenMint,
		postTokenPool,
		postTokenPoolPublish,
		postTokenTransfer,
		putContractAPI,
		putSubscription,
		postVerifiersResolve,
	})...,
)

func globalRoutes(routes []*ffapi.Route) []*ffapi.Route {
	for _, route := range routes {
		route.Tag = routeTagGlobal
	}
	return routes
}

func namespacedRoutes(routes []*ffapi.Route) []*ffapi.Route {
	newRoutes := make([]*ffapi.Route, len(routes))
	for i, route := range routes {
		route.Tag = routeTagDefaultNamespace

		routeCopy1 := *route
		routeCopy1.Name += "Namespace"
		routeCopy1.Path = "namespaces/{ns}/" + route.Path
		routeCopy1.PathParams = append(routeCopy1.PathParams, &ffapi.PathParam{
			Name: "ns", ExampleFromConf: coreconfig.NamespacesDefault, Description: coremsgs.APIParamsNamespace,
		})
		routeCopy1.Tag = routeTagNonDefaultNamespace
		newRoutes[i] = &routeCopy1

		// Build a separate list of NS relative routes, to build a swagger limited to one namespace
		routeCopy2 := *route
		nsRoutes = append(nsRoutes, &routeCopy2)
	}
	return append(routes, newRoutes...)
}
