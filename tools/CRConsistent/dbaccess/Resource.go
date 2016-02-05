package dbaccess

import (
	"errors"
	_ "github.com/Sirupsen/logrus"
	r "github.com/dancannon/gorethink"
	"github.com/shipyard/shipyard/containerresourcing"
)

func (dbaccess *DBAccess) ResourceList() ([]containerresourcing.ContainerResource, error) {
	res, err := r.Table(table_resource).Run(dbaccess.idesession)
	defer func(){
		if res!=nil{
			res.Close()
		}
	}()
	
	if err != nil {
		return nil, errors.New("Run() Error: " + err.Error())
	}

	if res.IsNil() {
		return nil, nil
	}

	var array []containerresourcing.ContainerResource

	if res.All(&array); err != nil {

		return nil, err
	}
	return array, nil
}
