up:
	mkdir -p .localstack/data
	cp .localstack/template/* .localstack/data
	docker-compose build
	docker-compose --profile all up -d

base:
	mkdir -p .localstack/data
	cp .localstack/template/* .localstack/data
	docker-compose build
	docker-compose --profile base up -d

down:
	docker kill localstack opensearch velociraptor-client velociraptor-frontend velociraptor-gui velociraptor-foreman || true

restart: down
	docker-compose build
	docker-compose --profile all up -d

clean:
	rm -f .localstack/data/*
	docker kill localstack opensearch velociraptor-client velociraptor-frontend velociraptor-gui velociraptor-foreman || true
	docker container rm velociraptor-foreman velociraptor-frontend velociraptor-gui localstack opensearch velociraptor-client || true
	docker volume rm docker_opensearch-data

bucket:
	aws s3 --endpoint-url http://localhost:4566/ --no-verify-ssl mb s3://velociraptor
