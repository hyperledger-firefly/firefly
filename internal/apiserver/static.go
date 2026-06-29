// Copyright © 2022 Kaleido, Inc.
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
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/common/pkg/i18n"
	"github.com/hyperledger-firefly/common/pkg/log"
	"github.com/hyperledger-firefly/firefly/internal/coremsgs"
)

type staticHandler struct {
	staticPath string
	indexPath  string
	urlPrefix  string
	os         osInterface
}

func newStaticHandler(staticPath, indexPath, urlPrefix string) *staticHandler {
	return &staticHandler{
		staticPath: staticPath,
		indexPath:  indexPath,
		urlPrefix:  urlPrefix,
		os:         &realOS{},
	}
}

func (h staticHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path, err := filepath.Rel(h.urlPrefix, r.URL.Path)
	if err != nil {
		// if we failed to get the path respond with a 404
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	// prepend the path with the path to the static directory
	path = filepath.Join(h.staticPath, strings.ReplaceAll(path, "..", "_"), "/")

	// check whether a file exists at the given path
	_, err = h.os.Stat(path)
	if os.IsNotExist(err) {
		// file does not exist, serve index.html
		http.ServeFile(w, r, filepath.Join(h.staticPath, h.indexPath))
		return
	} else if err != nil {
		// if we got an error (that wasn't that the file doesn't exist) stating the
		// file, return a 500 internal server error and stop
		log.L(r.Context()).Errorf("Failed to serve file: %s", err)
		w.WriteHeader(500)
		_ = json.NewEncoder(w).Encode(&fftypes.RESTError{
			Error: i18n.ExpandWithCode(r.Context(), i18n.MessageKey(coremsgs.MsgAPIServerStaticFail)),
		})
		return
	}

	// otherwise, use http.FileServer to serve the static dir
	http.StripPrefix(h.urlPrefix, http.FileServer(http.Dir(h.staticPath))).ServeHTTP(w, r)
}

type osInterface interface {
	Stat(name string) (os.FileInfo, error)
}

type realOS struct{}

func (r *realOS) Stat(name string) (os.FileInfo, error) {
	return os.Stat(name)
}
