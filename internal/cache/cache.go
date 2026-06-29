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

package cache

import (
	"context"
	"strings"
	"time"

	"github.com/hyperledger-firefly/common/pkg/cache"
	"github.com/hyperledger-firefly/common/pkg/config"
	"github.com/hyperledger-firefly/common/pkg/i18n"

	"github.com/hyperledger-firefly/firefly/internal/coreconfig"
	"github.com/hyperledger-firefly/firefly/internal/coremsgs"
)

type CConfig struct {
	ctx               context.Context
	namespace         string
	maxLimitConfigKey config.RootKey
	ttlConfigKey      config.RootKey
}

func NewCacheConfig(ctx context.Context, maxLimitConfigKey config.RootKey, ttlConfigKey config.RootKey, namespace string) *CConfig {
	if namespace == "" {
		namespace = "global"
	}
	cc := &CConfig{
		ctx:               ctx,
		namespace:         namespace,
		maxLimitConfigKey: maxLimitConfigKey,
		ttlConfigKey:      ttlConfigKey,
	}
	return cc
}

func (cc *CConfig) UniqueName() (string, error) {
	category, err := cc.Category()
	if err != nil {
		return "", err
	}
	return category, nil
}

func (cc *CConfig) Category() (string, error) {
	if cc.maxLimitConfigKey == "" {
		return "", i18n.NewError(cc.ctx, coremsgs.MsgCacheMissSizeLimitKeyInternal)
	}
	if cc.ttlConfigKey == "" {
		return "", i18n.NewError(cc.ctx, coremsgs.MsgCacheMissTTLKeyInternal)
	}

	categoryDerivedFromMaxLimitConfigKey, _ := parseConfigKeyString(string(cc.maxLimitConfigKey))
	categoryDerivedFromTTLConfigKey, _ := parseConfigKeyString(string(cc.ttlConfigKey))
	if categoryDerivedFromMaxLimitConfigKey != categoryDerivedFromTTLConfigKey {
		return "", i18n.NewError(cc.ctx, coremsgs.MsgCacheConfigKeyMismatchInternal, cc.maxLimitConfigKey, cc.ttlConfigKey, categoryDerivedFromMaxLimitConfigKey, categoryDerivedFromTTLConfigKey)

	}
	return categoryDerivedFromMaxLimitConfigKey, nil
}

func parseConfigKeyString(configKey string) (string, string) {
	keyParts := strings.Split(configKey, ".")
	categoryString := strings.Join(keyParts[:len(keyParts)-1], ".")
	configName := keyParts[len(keyParts)-1]
	return categoryString, configName
}

func (cc *CConfig) MaxSize() (int64, error) {
	_, sizeConfigName := parseConfigKeyString(string(cc.maxLimitConfigKey))
	switch sizeConfigName {
	case "limit":
		return config.GetInt64(cc.maxLimitConfigKey), nil
	case "size":
		return config.GetByteSize(cc.maxLimitConfigKey), nil
	default:
		return 0, i18n.NewError(cc.ctx, coremsgs.MsgCacheUnexpectedSizeKeyNameInternal, sizeConfigName)
	}
}

func (cc *CConfig) TTL() time.Duration {
	return config.GetDuration(cc.ttlConfigKey)
}

type Manager interface {
	GetCache(cc *CConfig) (CInterface, error)
	ResetCachesForNamespace(ns string)
	ListCacheNames(namespace string) []string
}

type CInterface cache.CInterface

type cacheManager struct {
	ffcache cache.Manager
}

func (cm *cacheManager) ResetCachesForNamespace(ns string) {
	cm.ffcache.ResetCaches(ns)
}
func (cm *cacheManager) ListCacheNames(namespace string) []string {
	return cm.ffcache.ListCacheNames(namespace)
}

func (cm *cacheManager) GetCache(cc *CConfig) (CInterface, error) {
	cacheName, err := cc.UniqueName()
	if err != nil {
		return nil, err
	}
	maxSize, err := cc.MaxSize()
	if err != nil {
		return nil, err
	}

	return cm.ffcache.GetCache(
		cc.ctx,
		cc.namespace,
		cacheName,
		maxSize,
		cc.TTL(),
		cm.ffcache.IsEnabled(),
	)
}
func NewCacheManager(ctx context.Context) Manager {
	cm := &cacheManager{
		ffcache: cache.NewCacheManager(ctx, config.GetBool(coreconfig.CacheEnabled)),
	}
	return cm
}

// should only be used for testing purpose
func NewUmanagedCache(ctx context.Context, sizeLimit int64, ttl time.Duration) CInterface {
	return cache.NewUmanagedCache(ctx, sizeLimit, ttl)
}
