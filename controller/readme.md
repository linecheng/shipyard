# Shipyard Controller
This is the core component for Shipyard.

# Setup
The only thing Shipyard needs to run is RethinkDB.

* Run RethinkDB: `docker run -it -d --name rethinkdb -P shipyard/rethinkdb`

* Run Shipyard: `docker run -it --name -P --link rethinkdb:rethinkdb shipyard/shipyard`

export GOPATH=$GOPATH:/home/cjt/code/go/src/github.com/shipyard/shipyard/Godeps/_workspace
./controller -D server --rethinkdb-addr 192.168.5.55:28015 -d tcp://192.168.5.55:4375 --registry 192.168.5.55:5000
./main -D server --rethinkdb-addr 192.168.5.55:28015 -d tcp://192.168.5.55:4375 --registry 192.168.5.55:5000
