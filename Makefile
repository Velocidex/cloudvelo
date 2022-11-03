SERVER_CONFIG=./Docker/config/server.config.yaml
CLIENT_CONFIG=./Docker/config/client.config.yaml
OVERRIDE_FILE=./Docker/config/local_override.json
BINARY=./output/cvelociraptor
CONFIG_ARGS= --config $(SERVER_CONFIG) --override_file $(OVERRIDE_FILE)
CLIENT_CONFIG_ARGS= --config $(CLIENT_CONFIG) --override_file $(OVERRIDE_FILE)
DLV=dlv debug --build-flags="-tags 'server_vql extras'" ./bin/ --
WRITEBACK_DIR=./pool_writebacks/
POOL_NUMBER=200

all:
	go run make.go -v Auto

debug_client:
	$(DLV) client -v $(CLIENT_CONFIG_ARGS)

.PHONY: client
client:
	$(BINARY) client -v $(CLIENT_CONFIG_ARGS)

pool_client:
	$(BINARY) pool_client -v $(CLIENT_CONFIG_ARGS) --writeback_dir $(WRITEBACK_DIR) --number $(POOL_NUMBER)

debug_pool_client:
	$(DLV) pool_client -v $(CLIENT_CONFIG_ARGS) --writeback_dir $(WRITEBACK_DIR) --number $(POOL_NUMBER)

gui:
	$(BINARY) $(CONFIG_ARGS) gui -v --debug

debug_gui:
	$(DLV) $(CONFIG_ARGS) gui -v --debug

frontend:
	$(BINARY) $(CONFIG_ARGS) frontend -v --debug

.PHONY: foreman
foreman:
	$(BINARY) $(CONFIG_ARGS) foreman -v --debug

debug_foreman:
	$(DLV) $(CONFIG_ARGS) foreman -v --debug

debug_frontend:
	$(DLV) $(CONFIG_ARGS) frontend -v --debug

reset_elastic:
	$(BINARY) $(CONFIG_ARGS) elastic reset --recreate $(INDEX)

windows:
	go run make.go -v Windows

linux_m1:
	go run make.go -v LinuxM1

linux_musl:
	go run make.go -v LinuxMusl

docker:
	go run make.go -v DockerImage

assets:
	go run make.go -v Assets

test:
	go run make.go -v BareAssets
	go test -v ./foreman/
	go test -v ./ingestion/
	go test -v ./filestore/
	go test -v ./services/...
	go test -v ./vql/...
