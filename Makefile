all:
	go build -gcflags "all=-N -l" -tags 'server_vql extras release yara' -o output/cvelociraptor ./bin/

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

debug_frontend:
	dlv debug --build-flags="-tags 'server_vql extras'" ./bin/ -- --elastic_config ./testdata/elastic/config.yaml --config ./testdata/config/server.config.yaml frontend -v --debug

debug_add_user:
	dlv debug --build-flags="-tags 'server_vql extras'" ./bin/ -- --elastic_config ./testdata/elastic/config.yaml --config ./testdata/config/server.config.yaml  query "SELECT user_create(user='mic', roles='administrator', password='mic') FROM scope()"

reset_elastic:
	./output/cvelociraptor  --elastic_config testdata/elastic/config.yaml  --config ./testdata/config/server.config.yaml  -v elastic reset

create_client:
	./output/cvelociraptor  --elastic_config testdata/elastic/config.yaml  --config ./testdata/config/server.config.yaml  -v query "SELECT client_create(client_id='C.719a8cba1ad9cead', hostname='mydevbox.example.com', os='linux', labels=['dev', 'mike']) FROM scope()"

linux_m1:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -gcflags "all=-N -l" -tags 'server_vql extras release yara' -o output/velociraptor ./bin/
