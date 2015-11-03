package manager

import (
	"errors"
	"fmt"
	_ "fmt"
	log "github.com/Sirupsen/logrus"
	r "github.com/dancannon/gorethink"
	resource "github.com/shipyard/shipyard/containerresourcing"
	"time"
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
	_, err := r.Db(db_webide_backend).Table(table_resource).Filter(map[string]string{"ResourceID": resourceid}).Update(res).RunWrite(m.session)
	if err != nil {
		return errors.New("resourceid = " + resourceid + err.Error())
	}
	return nil
}
func (m DefaultManager) GetResource(resourceid string) (*resource.ContainerResource, error) {
	res, err := r.Db(db_webide_backend).Table(table_resource).Filter(map[string]string{"ResourceID": resourceid}).Run(m.session)
	if err != nil {
		return nil, err
	}
	var cr resource.ContainerResource

	if res.IsNil() {
		return nil, nil
	}

	if err = res.One(&cr); err != nil {
		return nil, err
	}

	return &cr, nil
}

func (m DefaultManager) WaitUntilResourceAvaiable(resourceID string, timeout time.Duration, c_done chan map[string]string) {
	var done = map[string]string{
		"done":  "false",
		"error": "",
	}
	var begin = time.Now()
	log.Infof("开始等待资源%s由Moving转为可用状态,%ds秒后超时", resourceID, timeout/time.Second)

	for {
		if time.Now().Sub(begin) > timeout {
			done["error"] = fmt.Sprintf("资源 %s  查询超时，结束查询", resourceID)
			log.Infof(done["error"])
			c_done <- done
			return
		}

		var res, err = r.Db(db_webide_backend).Table(table_resource).Filter(map[string]string{"ResourceID": resourceID}).Run(m.session)
		if err != nil {
			done["error"] = err.Error()
			c_done <- done
			return
		}

		var data map[string]interface{}
		res.One(&data)
		log.Infof("%d s:  资源%v  状态为->%v", time.Now().Sub(begin)/time.Second, resourceID, data["Status"])

		if data["Status"] == "avaiable" {
			log.Infof("资源%v 已可用，查询结束 ", resourceID)
			done["done"] = "true"
			done["containerID"] = fmt.Sprint(data["ContainerID"])
			c_done <- done
			return
		}

		time.Sleep(5 * time.Second)
	}
}
