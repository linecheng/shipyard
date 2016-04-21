package manager

import (
	"errors"
	"fmt"
	"time"

	log "github.com/Sirupsen/logrus"
	r "github.com/dancannon/gorethink"
	resource "github.com/shipyard/shipyard/containerresourcing"
)

const (
	db_webide_backend = "webide_backend"
	table_resource    = "Resources"
)

func (m DefaultManager) initWebIdeBackendDb() {
	log.Info("checking webdie backend db")
	
	//check db
	exist,err := dbExist(m.session,db_webide_backend)
	if err!=nil{
		log.Fatalf("error check database :%s",db_webide_backend)
		return
	}
	if !exist{
		log.Infof("%s not exist, now create it.",db_webide_backend)
		_,err = r.DBCreate(db_webide_backend).RunWrite(m.session)
		if err!=nil{
			log.Fatalf("error creating database : %s ,Error : %s",db_webide_backend,err)
			return
		}
	}
	
	//check tables
	var db = r.DB(db_webide_backend)
	existMap,err := tableExist(m.session,db_webide_backend,table_resource)
	if err!=nil{
		log.Fatalf("error check table exist : %s",err)
		return
	}
	for tblName,val :=range existMap{
		if !val{
			log.Infof("create table %s",tblName)
			if _, err := db.TableCreate(tblName).RunWrite(m.session); err != nil {
				log.Fatalf("error creating table: %s , Error : %s", tblName, err)
				return
			}
		}
	}
	
	// _, err := db.Table(table_resource).Run(m.idesession)
	// if err != nil {
	// 	if _, err := db.TableCreate(table_resource).Run(m.idesession); err != nil {
	// 		log.Fatalf("error creating table: %s", err)
	// 		return
	// 	}
	// }

	log.Info("webdie backend db init success")
}

func (m DefaultManager) SaveResource(res *resource.ContainerResource) error {
	_, err := r.DB(db_webide_backend).Table(table_resource).Insert(res).RunWrite(m.session)

	return err
}
func (m DefaultManager) UpdateResource(resourceid string, res *resource.ContainerResource) error {
    res.LastUpdateTime=time.Now()
	_, err := r.DB(db_webide_backend).Table(table_resource).Filter(map[string]string{"ResourceID": resourceid}).Update(res).RunWrite(m.session)
	if err != nil {
		return errors.New("resourceid = " + resourceid + err.Error())
	}
	return nil
}

func (m DefaultManager) ResourceList() (*[]resource.ContainerResource, error) {
	res, err := r.DB(db_webide_backend).Table(table_resource).Run(m.session)
	defer func() {
		if res != nil {
			res.Close()
		}
	}()

	if err != nil {
		log.Infoln("get  ResourceList  from db Error :", err)
		return nil, err
	}

	var array []resource.ContainerResource
	if res.IsNil() {
		return nil, nil
	}

	if res.All(&array); err != nil {
		log.Infoln("res.All Error ", err)
		return nil, err
	}

	return &array, nil
}

func (m DefaultManager) DeleteResource(resourceId string) error {
	var res *r.Cursor
	var err error
	res, err = r.DB(db_webide_backend).Table(table_resource).Filter(map[string]string{"ResourceID": resourceId}).Run(m.session)
	defer func() {
		if res != nil {
			res.Close()
		}
	}()

	if err != nil {
		return err
	}

	if res.IsNil() {
		return nil
	}

	resp, err := r.DB(db_webide_backend).Table(table_resource).Filter(map[string]string{"ResourceID": resourceId}).Delete().RunWrite(m.session)
	if err != nil {
		return err
	}

	if resp.Deleted >= 1 {
		return nil
	} else {
		return errors.New("no to delete")
	}
}

func (m DefaultManager) GetResource(resourceid string) (*resource.ContainerResource, error) {
	res, err := r.DB(db_webide_backend).Table(table_resource).Filter(map[string]string{"ResourceID": resourceid}).Run(m.session)
	defer func() {
		if res != nil {
			res.Close()
		}
	}()

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
	var  (
		res *r.Cursor
		err error
	)
	defer func() {
		if res != nil {
			res.Close()//确保最终退出时的资源释放
		}
	}()

	for {
		if time.Now().Sub(begin) > timeout {
			done["error"] = fmt.Sprintf("资源 %s  查询超时，结束查询", resourceID)
			log.Infof(done["error"])
			c_done <- done
			return
		}

		res, err = r.DB(db_webide_backend).Table(table_resource).Filter(map[string]string{"ResourceID": resourceID}).Run(m.session)
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
		res.Close()// 在循环中及时释放对该资源的持有

		time.Sleep(5 * time.Second)
	}
}
