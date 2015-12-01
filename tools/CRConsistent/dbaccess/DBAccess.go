package dbaccess

import (
	r "github.com/dancannon/gorethink"
	"github.com/spf13/viper"
)

var (
	databaseName   = "webide_backend"
	table_resource = "Resources"
)

type DBAccess struct {
	idesession *r.Session
}

func New() (*DBAccess, error) {
	var addr = viper.GetString("rethinkdb.addr")
	var authKey = ""

	session, err := r.Connect(r.ConnectOpts{
		Address:  addr,
		MaxIdle:  10,
		Database: databaseName,
		AuthKey:  authKey,
	})

	if err != nil {
		return nil, err
	}

	return &DBAccess{
		idesession: session,
	}, nil
}
