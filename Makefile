SERVER_CONFIG=./Docker/config/server.config.yaml
CLIENT_CONFIG=./Docker/config/client.config.yaml
OVERRIDE_FILE=./Docker/config/local_override.json
BINARY=./output/cvelociraptor
CONFIG_ARGS= --config $(SERVER_CONFIG) --override_file $(OVERRIDE_FILE)
CLIENT_CONFIG_ARGS= --config $(CLIENT_CONFIG) --override_file $(OVERRIDE_FILE)
DLV=dlv debug --init ./scripts/dlv.init --build-flags="-tags 'server_vql extras'" ./bin/ -- --debug --debug_filter result_set
WRITEBACK_DIR=/tmp/pool_writebacks/
POOL_NUMBER=20

all:
	go run make.go -v Auto

debug_client:
	$(DLV) client -v $(CLIENT_CONFIG_ARGS) --debug --debug_port 6061

.PHONY: client
client:
	$(BINARY) client -v $(CLIENT_CONFIG_ARGS)

pool_client:
	$(BINARY) pool_client -v $(CLIENT_CONFIG_ARGS) --writeback_dir $(WRITEBACK_DIR) --number $(POOL_NUMBER)

debug_pool_client:
	$(DLV) pool_client -v $(CLIENT_CONFIG_ARGS) --writeback_dir $(WRITEBACK_DIR) --number $(POOL_NUMBER)

gui:
	$(BINARY) $(CONFIG_ARGS) gui -v --debug

dump:
	$(BINARY) $(CONFIG_ARGS) elastic dump -v

dump_persisted:
	$(BINARY) $(CONFIG_ARGS) elastic dump --index="persisted" -v --org_id O123

debug_gui:
	$(DLV) $(CONFIG_ARGS) gui -v

frontend:
	$(BINARY) $(CONFIG_ARGS) frontend -v --debug

frontend-Mock:
	$(BINARY) $(CONFIG_ARGS) frontend -v --debug --mock

.PHONY: foreman
foreman:
	$(BINARY) $(CONFIG_ARGS) foreman -v --debug

debug_foreman:
	$(DLV) $(CONFIG_ARGS) foreman -v --debug

debug_frontend:
	$(DLV) $(CONFIG_ARGS) frontend -v --debug

debug_frontend-Mock:
	$(DLV) $(CONFIG_ARGS) frontend -v --debug --mock

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
