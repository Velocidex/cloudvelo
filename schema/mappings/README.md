# Elastic schema

Elastic is unable to efficiently modify existing documents so we need
to store multiple documents in the same index and then just combine
them on reading.

## The client record

This is split into multiple records:

### Ping

Stored in document id `<client_id>_ping`. Contained ping time and
client id. Each ping message from the client will overwrite it.

### Labels

Stored in document id `<client_id>_labels`. Currently read - modiy -
write because it is not updated that frequently.

### Assigned hunts
