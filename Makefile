VGO=go
BINARY_NAME=firefly
GOFILES := $(shell find cmd internal pkg -name '*.go' -print)
GOBIN := $(shell $(VGO) env GOPATH)/bin
LINT := $(GOBIN)/golangci-lint
MOCKERY := $(GOBIN)/mockery
DATE := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

# Expect that FireFly compiles with CGO disabled
CGO_ENABLED=0
GOGC=30

.DELETE_ON_ERROR:

all: build test go-mod-tidy
test: deps lint
		$(VGO) test ./internal/... ./pkg/... ./cmd/... ./doc-site ./ffconfig/... -cover -coverprofile=coverage.txt -covermode=atomic -timeout=45s ${TEST_ARGS}
coverage.html:
		$(VGO) tool cover -html=coverage.txt
coverage: test coverage.html
lint: ${LINT}
		GOGC=20 $(LINT) run -v --timeout 5m
${MOCKERY}:
		$(VGO) install github.com/vektra/mockery/v2@latest
${LINT}:
		$(VGO) install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.64.8
ffcommon:
		$(eval WSCLIENT_PATH := $(shell $(VGO) list -f '{{.Dir}}' github.com/hyperledger-firefly/common/pkg/wsclient))


define makemock
mocks: mocks-$(strip $(1))-$(strip $(2))
mocks-$(strip $(1))-$(strip $(2)): ${MOCKERY} ffcommon
	${MOCKERY} --case underscore --dir $(1) --name $(2) --outpkg $(3) --output mocks/$(strip $(3))
endef

$(eval $(call makemock, $$(WSCLIENT_PATH),          WSClient,             wsmocks))
$(eval $(call makemock, pkg/blockchain,             Plugin,               blockchainmocks))
$(eval $(call makemock, pkg/blockchain,             Callbacks,            blockchainmocks))
$(eval $(call makemock, pkg/core,                   OperationCallbacks,   coremocks))
$(eval $(call makemock, pkg/database,               Plugin,               databasemocks))
$(eval $(call makemock, pkg/database,               Callbacks,            databasemocks))
$(eval $(call makemock, pkg/sharedstorage,          Plugin,               sharedstoragemocks))
$(eval $(call makemock, pkg/sharedstorage,          Callbacks,            sharedstoragemocks))
$(eval $(call makemock, pkg/events,                 Plugin,               eventsmocks))
$(eval $(call makemock, pkg/events,                 Callbacks,            eventsmocks))
$(eval $(call makemock, pkg/identity,               Plugin,               identitymocks))
$(eval $(call makemock, pkg/identity,               Callbacks,            identitymocks))
$(eval $(call makemock, pkg/dataexchange,           Plugin,               dataexchangemocks))
$(eval $(call makemock, pkg/dataexchange,           DXEvent,              dataexchangemocks))
$(eval $(call makemock, pkg/dataexchange,           Callbacks,            dataexchangemocks))
$(eval $(call makemock, pkg/tokens,                 Plugin,               tokenmocks))
$(eval $(call makemock, pkg/tokens,                 Callbacks,            tokenmocks))
$(eval $(call makemock, internal/txcommon,          Helper,               txcommonmocks))
$(eval $(call makemock, internal/txwriter,          Writer,               txwritermocks))
$(eval $(call makemock, internal/identity,          Manager,              identitymanagermocks))
$(eval $(call makemock, internal/syncasync,         Sender,               syncasyncmocks))
$(eval $(call makemock, internal/syncasync,         Bridge,               syncasyncmocks))
$(eval $(call makemock, internal/data,              Manager,              datamocks))
$(eval $(call makemock, internal/batch,             Manager,              batchmocks))
$(eval $(call makemock, internal/broadcast,         Manager,              broadcastmocks))
$(eval $(call makemock, internal/blockchain/common, FireflySubscriptions, blockchaincommonmocks))
$(eval $(call makemock, internal/privatemessaging,  Manager,              privatemessagingmocks))
$(eval $(call makemock, internal/shareddownload,    Manager,              shareddownloadmocks))
$(eval $(call makemock, internal/shareddownload,    Callbacks,            shareddownloadmocks))
$(eval $(call makemock, internal/definitions,       Handler,              definitionsmocks))
$(eval $(call makemock, internal/definitions,       Sender,               definitionsmocks))
$(eval $(call makemock, internal/events,            EventManager,         eventmocks))
$(eval $(call makemock, internal/events/system,     EventInterface,       systemeventmocks))
$(eval $(call makemock, internal/namespace,         Manager,              namespacemocks))
$(eval $(call makemock, internal/networkmap,        Manager,              networkmapmocks))
$(eval $(call makemock, internal/assets,            Manager,              assetmocks))
$(eval $(call makemock, internal/contracts,         Manager,              contractmocks))
$(eval $(call makemock, internal/spievents,         Manager,              spieventsmocks))
$(eval $(call makemock, internal/orchestrator,      Orchestrator,         orchestratormocks))
$(eval $(call makemock, internal/cache,             Manager,              cachemocks))
$(eval $(call makemock, internal/metrics,           Manager,              metricsmocks))
$(eval $(call makemock, internal/operations,        Manager,              operationmocks))
$(eval $(call makemock, internal/multiparty,        Manager,              multipartymocks))
$(eval $(call makemock, internal/apiserver,         FFISwaggerGen,        apiservermocks))
$(eval $(call makemock, internal/apiserver,         Server,               apiservermocks))
$(eval $(call makemock, internal/events/websockets, WebSocketsNamespaced, websocketsmocks))

firefly-nocgo: ${GOFILES}
		CGO_ENABLED=0 $(VGO) build -o ${BINARY_NAME}-nocgo -ldflags "-X main.buildDate=$(DATE) -X main.buildVersion=$(BUILD_VERSION) -X 'github.com/hyperledger-firefly/firefly/cmd.BuildVersionOverride=$(BUILD_VERSION)' -X 'github.com/hyperledger-firefly/firefly/cmd.BuildDate=$(DATE)' -X 'github.com/hyperledger-firefly/firefly/cmd.BuildCommit=$(GIT_REF)'" -tags=prod -tags=prod -v
firefly: ${GOFILES}
		$(VGO) build -o ${BINARY_NAME} -ldflags "-X main.buildDate=$(DATE) -X main.buildVersion=$(BUILD_VERSION) -X 'github.com/hyperledger-firefly/firefly/cmd.BuildVersionOverride=$(BUILD_VERSION)' -X 'github.com/hyperledger-firefly/firefly/cmd.BuildDate=$(DATE)' -X 'github.com/hyperledger-firefly/firefly/cmd.BuildCommit=$(GIT_REF)'" -tags=prod -tags=prod -v
go-mod-tidy: .ALWAYS
		$(VGO) mod tidy
build: firefly-nocgo firefly
e2e: build
		./test/e2e/run.sh
.ALWAYS: ;
e2e-rebuild: .ALWAYS
		DOWNLOAD_CLI=false BUILD_FIREFLY=false CREATE_STACK=false ./test/e2e/run.sh
clean:
		$(VGO) clean
		rm -f *.so ${BINARY_NAME}
deps:
		$(VGO) get
reference:
		$(VGO) test ./internal/apiserver ./internal/reference ./doc-site -timeout=10s -tags reference
manifest:
		./manifestgen.sh
docker:
		./docker_build.sh --load $(DOCKER_ARGS)
docker-multiarch:
		./docker_build.sh --platform linux/amd64,linux/arm64 $(DOCKER_ARGS) 
docs: .ALWAYS
		cd doc-site && mkdocs build
