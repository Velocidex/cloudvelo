# Docker setup for testing

This docker compose file will start all the components locally

```
docker-compose --profile dev up
```

## Inspecting the local s3 bucket

Make a bucket for use by Velociraptor

```
aws s3 --endpoint http://localhost:4566/ mb s3://velociraptor
```

You can use the AWS cli to inspect the local buckets created by localstack.

```
aws s3 --endpoint http://localhost:4566/ ls
```
