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

package apiserver

import (
	"github.com/hyperledger-firefly/common/pkg/ffapi"
	"github.com/hyperledger-firefly/firefly/internal/coreconfig"
	"github.com/hyperledger-firefly/firefly/internal/coremsgs"
)

// The Service Provider Interface (SPI) allows external microservices (such as the FireFly Transaction Manager)
// to act as augmented components to the core.
var spiRoutes = append(globalRoutes([]*ffapi.Route{
	spiGetNamespaceByName,
	spiGetNamespaces,
	spiGetOpByID,
	spiPatchOpByID,
	spiPostReset,
}),
	namespacedSPIRoutes([]*ffapi.Route{
		spiGetOps,
	})...,
)

func namespacedSPIRoutes(routes []*ffapi.Route) []*ffapi.Route {
	newRoutes := make([]*ffapi.Route, len(routes))
	for i, route := range routes {
		route.Tag = routeTagDefaultNamespace

		routeCopy := *route
		routeCopy.Name += "Namespace"
		routeCopy.Path = "namespaces/{ns}/" + route.Path
		routeCopy.PathParams = append(routeCopy.PathParams, &ffapi.PathParam{
			Name: "ns", ExampleFromConf: coreconfig.NamespacesDefault, Description: coremsgs.APIParamsNamespace,
		})
		routeCopy.Tag = routeTagNonDefaultNamespace
		newRoutes[i] = &routeCopy
	}
	return append(routes, newRoutes...)
}
