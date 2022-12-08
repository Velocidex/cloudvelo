# Docker setup for testing

This docker compose file will start all the components locally

```
docker-compose --profile dev up
```

## Inspecting the local s3 bucket

Make a bucket for use by Velociraptor

```
aws s3 --endpoint-url http://localhost:4566/ --no-verify-ssl mb s3://velociraptor
```

You can use the AWS cli to inspect the local buckets created by localstack.

```
aws s3 --endpoint-url http://localhost:4566/ --no-verify-ssl ls s3://velociraptor/
```


## Replace localstack with minio

Localstack is very limited and does not support large files. For some
testing we need minio.

```
MINIO_ROOT_USER=admin MINIO_ROOT_PASSWORD=password ./minio server /tmp/minio --console-address ":9001" --address ":4566"
```

```
./mc alias set myminio http://192.168.1.11:4566 admin password
./mc mb myminio/velociraptor
```
