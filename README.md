## How to build

This version of Velociraptor depends on the open source codebase as
used on GitHub. The GitHub repo is included as a git submodule. We use
the same GUI so we need to have the React App built. Therefore we cant
use a simple go get to install the dependency.

1. First make sure the git submodule is cloned properly

```
git submodule update --init
```

2. Next build the Velociraptor GUI

```
cd ./velociraptor/gui/velociraptor
npm ci
npm run build
cd -
```

3. Now generate the go files (NOTE: You need to have ~/go/bin in your
   path so we can install fluxb0x there to be able to generate the Go
   files.

```
cd ./velociraptor/
make linux
cd -
```

4. Finally we can build the Cloud version by running make in the top level.

```
make
```

You will find the binary in `./output/cvelociraptor`

## Notes

In the codebase and below we use the term Elastic to refer to the
opensource backend database which originally was managed by Elastic
Inc. Recently, the original Elastic database was split into an
opensource project (https://opensearch.org/) and a non-open source
database offered by Elastic Inc. Further, the Elastic maintained Go
client libraries refuse to connect to the open source database.

As such, we need to decide which flavor of Elastic to support moving
forward. As an open source project we prefer to support open source
dependencies, so this project only supports the opensearch backend.

Any references to Elastic in the codebase or documentation actually
refer to opensearch and that is the only database that is supported at
this time.

## Start the GUI

```
./output/cvelociraptor --config testdata/config/server.config.yaml gui
```

## Initialize the elastic indexes

The following will delete all indexes and recreate them with the correct mappings.

```
./output/cvelociraptor  --config testdata/config/server.config.yaml elastic reset
```

## To create a new user

We can use VQL to create a new user so you may log into the GUI

```
$ ./output/cvelociraptor  --config testdata/config/server.config.yaml  -v query "SELECT user_create(user='mic', roles='administrator', password='hunter1') FROM scope()"
```

## Start the frontend (receiving data from clients)

```
./output/cvelociraptor frontend  --config testdata/config/server.config.yaml -v
```

## Start the client

```
./output/cvelociraptor -v client  --config testdata/config/client.config.yaml
```
