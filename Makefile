all:
	go build -tags 'server_vql extras release yara' -o output/cvelociraptor ./bin/

debug_client:
	dlv debug --build-flags="-tags 'server_vql extras'" ./bin/ -- client -v  --config ./testdata/config/client.config.yaml

.PHONY: client
client:
	./output/cvelociraptor client -v  --config ./testdata/config/client.config.yaml

pool_client:
	./output/cvelociraptor pool_client -v  --config ./testdata/config/server.config.yaml --writeback_dir ./testdata/pool_writebacks/ --number 200

debug_pool_client:
	dlv debug --build-flags="-tags 'server_vql extras'" ./bin/ -- pool_client -v  --config ./testdata/config/server.config.yaml --writeback_dir ./testdata/pool_writebacks/ --number 200

gui:
	./output/cvelociraptor --elastic_config ./testdata/elastic/config.yaml  --config ./testdata/config/server.config.yaml  gui -v --debug

debug_gui:
	dlv debug --build-flags="-tags 'server_vql extras'" ./bin/ -- --elastic_config ./testdata/elastic/config.yaml  --config ./testdata/config/server.config.yaml  gui -v --debug

frontend:
	./output/cvelociraptor --elastic_config ./testdata/elastic/config.yaml --config ./testdata/config/server.config.yaml frontend -v --debug

.PHONY: foreman
foreman:
	./output/cvelociraptor --elastic_config ./testdata/elastic/config.yaml --config ./testdata/config/server.config.yaml foreman -v --debug

debug_foreman:
	dlv debug --build-flags="-tags 'server_vql extras'" ./bin/ -- --elastic_config ./testdata/elastic/config.yaml --config ./testdata/config/server.config.yaml foreman -v --debug

debug_frontend:
	dlv debug --build-flags="-tags 'server_vql extras'" ./bin/ -- --elastic_config ./testdata/elastic/config.yaml --config ./testdata/config/server.config.yaml frontend -v --debug

add_user:
	./output/cvelociraptor --elastic_config ./testdata/elastic/config.yaml --config ./testdata/config/server.config.yaml  query "SELECT user_create(user='mic', roles='administrator', password='mic') FROM scope()"

reset_elastic:
	./output/cvelociraptor  --elastic_config testdata/elastic/config.yaml  --config ./testdata/config/server.config.yaml  -v elastic reset

linux_m1:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -gcflags "all=-N -l" -tags 'server_vql extras release yara' -o output/velociraptor ./bin/

linux_musl:
	go run make.go -v LinuxMusl

docker:
	go run make.go -v DockerImage
