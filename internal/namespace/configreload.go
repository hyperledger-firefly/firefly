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

package namespace

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hyperledger-firefly/common/pkg/config"
	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/common/pkg/log"
	"github.com/hyperledger-firefly/firefly/internal/coreconfig"
	"github.com/hyperledger-firefly/firefly/internal/orchestrator"
	"github.com/spf13/viper"
)

func (nm *namespaceManager) dumpRootConfig() (jsonTree fftypes.JSONObject) {
	viperTree := viper.AllSettings()
	b, _ := json.Marshal(viperTree)
	_ = json.Unmarshal(b, &jsonTree)
	return
}

func (nm *namespaceManager) startConfigListener() error {
	if config.GetBool(coreconfig.ConfigAutoReload) {
		return config.WatchConfig(nm.ctx, nm.configFileChanged, nil)
	}
	return nil
}

func (nm *namespaceManager) configFileChanged() {
	log.L(nm.ctx).Infof("Detected configuration file reload")

	// Because of the things we do to make defaults work with arrays, we have to reset
	// the config when it changes and re-read it.
	// We are passed this by our parent, as the config initialization of defaults and sections
	// might include others than under the namespaces tree (API Server etc. etc.)
	err := nm.reloadConfig()
	if err != nil {
		log.L(nm.ctx).Errorf("Failed to re-read configuration after config reload notification: %s", err)
		nm.cancelCtx() // stop the world
		return
	}

	nm.configReloaded(nm.ctx)
}

func (nm *namespaceManager) configReloaded(ctx context.Context) {
	// Always make sure log level is up to date
	log.SetLevel(config.GetString(config.LogLevel))

	// Get Viper to dump the whole new config, with everything resolved across env vars
	// and the config file etc.
	// We use this to detect if anything has changed.
	rawConfig := nm.dumpRootConfig()

	// Build the new set of plugins from the config (including those that are unchanged)
	allPluginsInNewConf, err := nm.loadPlugins(ctx, rawConfig)
	if err != nil {
		log.L(ctx).Errorf("Failed to initialize plugins after config reload: %s", err)
		return
	}

	// Analyze the new list to see which plugins need to be updated,
	// so we load the namespaces against the correct list of plugins
	availablePlugins, updatedPlugins, pluginsToStop := nm.analyzePluginChanges(ctx, allPluginsInNewConf)

	// Build the new set of namespaces (including those that are unchanged)
	allNewNamespaces, err := nm.loadNamespaces(ctx, rawConfig, availablePlugins)
	if err != nil {
		log.L(ctx).Errorf("Failed to load namespaces after config reload: %s", err)
		return
	}

	// From this point we need to block any API calls resolving namespaces,
	// until the reload is complete
	nm.nsMux.Lock()
	defer nm.nsMux.Unlock()

	// Stop all defunct namespaces
	availableNS, updatedNamespaces := nm.stopDefunctNamespaces(ctx, availablePlugins, allNewNamespaces)

	// Stop all defunct plugins - now the namespaces using them are all stopped
	nm.stopDefunctPlugins(ctx, pluginsToStop)

	// If there are any namespaces that are completely gone at this point we need to purge
	// them from the system (all the handlers/callback registrations), before we update the
	// namespace list and lose track of the old orchestrator
	for oldNSName, oldNS := range nm.namespaces {
		if _, stillExists := availableNS[oldNSName]; !stillExists && oldNS.orchestrator != nil {
			orchestrator.Purge(ctx, &oldNS.Namespace, oldNS.plugins, oldNS.config.Multiparty.Node.Name)
		}
	}

	// Update the new lists
	nm.plugins = availablePlugins
	nm.namespaces = availableNS

	// Only initialize updated plugins
	if err = nm.initPlugins(updatedPlugins); err != nil {
		log.L(ctx).Errorf("Failed to initialize plugins after config reload: %s", err)
		nm.cancelCtx() // stop the world
		return
	}

	// Now we can start all the new things
	if err = nm.startNamespacesAndPlugins(updatedNamespaces, updatedPlugins); err != nil {
		log.L(ctx).Errorf("Failed to initialize namespaces after config reload: %s", err)
		nm.cancelCtx() // stop the world
		return
	}

}

func (nm *namespaceManager) stopDefunctNamespaces(ctx context.Context, newPlugins map[string]*plugin, newNamespaces map[string]*namespace) (availableNamespaces, updatedNamespaces map[string]*namespace) {

	// build a set of all the namespaces we've either added new, or have changed
	updatedNamespaces = make(map[string]*namespace)
	availableNamespaces = make(map[string]*namespace)
	namespacesToStop := make(map[string]*namespace)
	newNamespaceNames := make([]string, 0)
	updatedNamespaceNames := make([]string, 0)
	for nsName, newNS := range newNamespaces {
		newNamespaceNames = append(newNamespaceNames, nsName)
		if existingNS := nm.namespaces[nsName]; existingNS != nil {
			var changes []string
			if !existingNS.configHash.Equals(newNS.configHash) {
				changes = append(changes, "namespace_config") // Encompasses the list of plugins
			}
			for _, pluginName := range newNS.pluginNames {
				existingPlugin := nm.plugins[pluginName]
				newPlugin := newPlugins[pluginName]
				if existingPlugin == nil || newPlugin == nil ||
					!existingPlugin.configHash.Equals(newPlugin.configHash) {
					changes = append(changes, fmt.Sprintf("plugin:%s", pluginName))
				}
			}
			if len(changes) == 0 {
				log.L(ctx).Debugf("Namespace '%s' unchanged after config reload", nsName)
				availableNamespaces[nsName] = existingNS
				continue
			}
			// We need to stop the existing namespace
			log.L(ctx).Infof("Namespace '%s' configuration changed: %v", nsName, changes)
			namespacesToStop[nsName] = existingNS
		}
		// This is either changed, or brand new - mark it in the map
		availableNamespaces[nsName] = newNS
		updatedNamespaces[nsName] = newNS
		updatedNamespaceNames = append(updatedNamespaceNames, nsName)
	}

	// Stop everything we need to stop
	oldNamespaceNames := make([]string, 0)
	stoppingNamespaceNames := make([]string, 0)
	for nsName, existingNS := range nm.namespaces {
		oldNamespaceNames = append(oldNamespaceNames, nsName)
		if namespacesToStop[nsName] != nil || newNamespaces[nsName] == nil {
			stoppingNamespaceNames = append(stoppingNamespaceNames, nsName)
			log.L(ctx).Debugf("Stopping namespace '%s' after config reload. Loaded at %s", nsName, existingNS.loadTime)
			nm.stopNamespace(ctx, existingNS)

			// Clear cache managers for the stopped namespace, now the orchestrator is stopped
			nm.cacheManager.ResetCachesForNamespace(nsName)
		}
	}
	log.L(nm.ctx).Infof("Namespace reload summary: old=%v new=%v updated=%v stopping=%v", oldNamespaceNames, newNamespaceNames, updatedNamespaceNames, stoppingNamespaceNames)

	return availableNamespaces, updatedNamespaces

}

func (nm *namespaceManager) analyzePluginChanges(ctx context.Context, newPlugins map[string]*plugin) (availablePlugins, updatedPlugins, pluginsToStop map[string]*plugin) {

	// build a set of all the plugins we've either added new, or have changed
	availablePlugins = make(map[string]*plugin)
	updatedPlugins = make(map[string]*plugin)
	pluginsToStop = make(map[string]*plugin)
	for pluginName, newPlugin := range newPlugins {
		if existingPlugin := nm.plugins[pluginName]; existingPlugin != nil {
			if existingPlugin.configHash.Equals(newPlugin.configHash) {
				log.L(ctx).Debugf("Plugin '%s' unchanged after config reload", pluginName)
				availablePlugins[pluginName] = existingPlugin
				continue
			}
			// We need to stop the existing plugin
			pluginsToStop[pluginName] = existingPlugin
		}
		// This is either changed, or brand new - mark it in the map
		updatedPlugins[pluginName] = newPlugin
		availablePlugins[pluginName] = newPlugin
	}

	// Look for everything that's deleted
	for pluginName, existingPlugin := range nm.plugins {
		if newPlugins[pluginName] == nil {
			pluginsToStop[pluginName] = existingPlugin
		}
	}

	return
}

func (nm *namespaceManager) stopDefunctPlugins(ctx context.Context, pluginsToStop map[string]*plugin) {
	for pluginName, plugin := range pluginsToStop {
		log.L(ctx).Debugf("Stopping plugin '%s' after config reload. Loaded at %s", pluginName, plugin.loadTime)
		plugin.cancelCtx()
	}
}
