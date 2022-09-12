up:
	mkdir -p .localstack/data
	cp .localstack/template/* .localstack/data
	docker-compose --profile all up -d

restart:
	docker kill localstack opensearch velociraptor-client velociraptor-frontend velociraptor-gui velociraptor-foreman || true
	docker-compose build
	docker-compose --profile all up -d

clean:
	rm -f .localstack/data/*
	docker kill localstack opensearch velociraptor-client velociraptor-frontend velociraptor-gui velociraptor-foreman || true
	docker container rm velociraptor-foreman velociraptor-frontend velociraptor-gui localstack opensearch velociraptor-client
	docker volume rm docker_opensearch-data
