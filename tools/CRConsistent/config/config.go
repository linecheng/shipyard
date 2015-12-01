package config

import (
	log "github.com/Sirupsen/logrus"
	"github.com/spf13/viper"
)

func InitConfig() {
	viper.SetConfigFile("config.json")
	viper.SetConfigType("json")
	//	viper.BindEnv("rethinkdb.addr", "RETHINKDB_ADDR")
	//	viper.BindEnv("swarm.url", "SWARM_URL")
	var err = viper.ReadInConfig()
	if err != nil {
		log.Error("notify load config.js error  ,will using default values : " + err.Error())
		return
	}
}
