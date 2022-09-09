up:
	docker-compose --profile all up -d

clean:
	docker kill localstack opensearch velociraptor-client velociraptor-frontend velociraptor-gui
	docker container rm velociraptor-frontend velociraptor-gui localstack opensearch velociraptor-client
	docker volume rm docker_localstack-data docker_opensearch-data
