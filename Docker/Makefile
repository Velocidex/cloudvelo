up:
	docker-compose --profile all up -d

clean:
	docker kill localstack opensearch velociraptor-client velociraptor-frontend velociraptor-gui velociraptor-foreman || true
	docker container rm velociraptor-foreman velociraptor-frontend velociraptor-gui localstack opensearch velociraptor-client
	docker volume rm docker_localstack-data docker_opensearch-data
