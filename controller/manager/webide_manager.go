package manager

import (
	"errors"
	_ "fmt"
	log "github.com/Sirupsen/logrus"
	r "github.com/dancannon/gorethink"
	resource "github.com/shipyard/shipyard/containerresourcing"
)

const (
	db_webide_backend = "webide_backend"
	table_resource    = "Resources"
)

func (m DefaultManager) initWebIdeBackendDb() {
	log.Info("begin to init webdie backend db")
	r.DbCreate(db_webide_backend).Run(m.session)
	var db = r.Db(db_webide_backend)
	_, err := db.Table(table_resource).Run(m.session)
	if err != nil {
		if _, err := db.TableCreate(table_resource).Run(m.session); err != nil {
			log.Fatalf("error creating table: %s", err)
			return
		}
	}

	log.Info("webdie backend db init success")
}

func (m DefaultManager) SaveResource(res *resource.ContainerResource) error {
	_, err := r.Db(db_webide_backend).Table(table_resource).Insert(res).RunWrite(m.session)
	return err
}
func (m DefaultManager) UpdateResource(resourceid string, res *resource.ContainerResource) error {
	log.Infoln("开始更新资源：ResourceID=" + resourceid)
	_, err := r.Db(db_webide_backend).Table(table_resource).Filter(map[string]string{"ID": resourceid}).Update(res).RunWrite(m.session)
	if err != nil {
		return errors.New("resourceid = " + resourceid + err.Error())
	}
	return nil
}
func (m DefaultManager) GetResource(resourceid string) (*resource.ContainerResource, error) {
	res, err := r.Db(db_webide_backend).Table(table_resource).Filter(map[string]string{"ID": resourceid}).Run(m.session)
	if err != nil {
		return nil, err
	}
	var cr resource.ContainerResource
	if err = res.One(&cr); err != nil {
		return nil, err
	}

	return &cr, nil
}
