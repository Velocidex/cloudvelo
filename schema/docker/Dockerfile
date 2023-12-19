# syntax=docker/dockerfile:1
FROM alpine:3
RUN apk add --no-cache bash curl
COPY ./scripts/*.sh /
RUN chmod +x /create_indexes.sh
RUN chmod +x /create_scripts.sh
RUN chmod +x /create_policies.sh
RUN chmod +x /run.sh
COPY ./templates/*.json /index-templates/
COPY ./scripts/*.json /stored-scripts/
COPY ./policies/*.json /index-policies/
CMD ["/run.sh", "/stored-scripts",  "/index-templates", "/index-policies", "http://opensearch:9200"]
