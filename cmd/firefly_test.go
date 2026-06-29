// Copyright © 2021 Kaleido, Inc.
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

package cmd

import (
	"context"
	"fmt"
	"os"
	"syscall"
	"testing"

	"github.com/hyperledger-firefly/firefly/mocks/apiservermocks"
	"github.com/hyperledger-firefly/firefly/mocks/namespacemocks"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

const configDir = "../test/data/config"

func TestGetEngine(t *testing.T) {
	assert.NotNil(t, getRootManager())
}

func TestExecMissingConfig(t *testing.T) {
	_utManager = &namespacemocks.Manager{}
	defer func() { _utManager = nil }()
	viper.Reset()
	err := Execute()
	assert.Regexp(t, "Not Found", err)
}

func TestShowConfig(t *testing.T) {
	_utManager = &namespacemocks.Manager{}
	defer func() { _utManager = nil }()
	viper.Reset()
	rootCmd.SetArgs([]string{"showconf"})
	defer rootCmd.SetArgs([]string{})
	err := rootCmd.Execute()
	assert.NoError(t, err)
}

func TestExecEngineInitFail(t *testing.T) {
	o := &namespacemocks.Manager{}
	o.On("Init", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(fmt.Errorf("splutter"))
	_utManager = o
	defer func() { _utManager = nil }()
	os.Chdir(configDir)
	err := Execute()
	assert.Regexp(t, "splutter", err)
}

func TestExecEngineStartFail(t *testing.T) {
	o := &namespacemocks.Manager{}
	o.On("Init", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	o.On("Start").Return(fmt.Errorf("bang"))
	_utManager = o
	defer func() { _utManager = nil }()
	os.Chdir(configDir)
	err := Execute()
	assert.Regexp(t, "bang", err)
}

func TestExecOkExitSIGINT(t *testing.T) {
	o := &namespacemocks.Manager{}
	o.On("Init", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	o.On("Start").Return(nil)
	o.On("WaitStop").Return()
	_utManager = o
	defer func() { _utManager = nil }()

	os.Chdir(configDir)
	go func() {
		sigs <- syscall.SIGINT
	}()
	err := Execute()
	assert.NoError(t, err)
}

func TestExecOkCancel(t *testing.T) {
	o := &namespacemocks.Manager{}
	init := o.On("Init", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	init.RunFn = func(a mock.Arguments) {
		cancelCtx := a[1].(context.CancelFunc)
		cancelCtx()
	}
	o.On("Start").Return(nil)
	o.On("WaitStop")
	_utManager = o
	defer func() { _utManager = nil }()

	os.Chdir(configDir)
	err := run()
	assert.NoError(t, err)
}

func TestExecOkRestartThenExit(t *testing.T) {
	o := &namespacemocks.Manager{}
	initCount := 0
	init := o.On("Init", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	init.RunFn = func(a mock.Arguments) {
		resetChan := a[2].(chan bool)
		initCount++
		if initCount == 2 {
			init.ReturnArguments = mock.Arguments{fmt.Errorf("second run")}
		} else {
			resetChan <- true
		}
	}
	o.On("Start").Return(nil)
	o.On("WaitStop")
	_utManager = o
	defer func() { _utManager = nil }()

	os.Chdir(configDir)
	err := run()
	assert.EqualError(t, err, "second run")
}

func TestExecOkRestartConfigProblem(t *testing.T) {
	o := &namespacemocks.Manager{}
	tmpDir, err := os.MkdirTemp(os.TempDir(), "ut")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpDir)
	var orContext context.Context
	init := o.On("Init", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	init.RunFn = func(a mock.Arguments) {
		orContext = a[0].(context.Context)
		resetChan := a[2].(chan bool)
		resetChan <- true
	}
	o.On("Start").Return(nil)
	o.On("WaitStop").Run(func(args mock.Arguments) {
		<-orContext.Done()
		os.Chdir(tmpDir) // this will mean we fail to read the config
	})
	_utManager = o
	defer func() { _utManager = nil }()

	os.Chdir(configDir)
	err = run()
	assert.Regexp(t, "Config File.*Not Found", err)
}

func TestAPIServerError(t *testing.T) {
	o := &namespacemocks.Manager{}
	o.On("Init", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	o.On("Start").Return(nil)
	as := &apiservermocks.Server{}
	as.On("Serve", mock.Anything, o).Return(fmt.Errorf("pop"))

	errChan := make(chan error)
	resetChan := make(chan bool)
	go startFirefly(context.Background(), func() {}, o, as, errChan, resetChan, make(chan struct{}))
	err := <-errChan
	assert.EqualError(t, err, "pop")
}
