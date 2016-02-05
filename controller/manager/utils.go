package manager

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"strings"
	"time"
	"errors"

	"github.com/shipyard/shipyard"
	r "github.com/dancannon/gorethink"
)

func getTLSConfig(caCert, sslCert, sslKey []byte) (*tls.Config, error) {
	// TLS config
	var tlsConfig tls.Config
	tlsConfig.InsecureSkipVerify = true
	certPool := x509.NewCertPool()

	certPool.AppendCertsFromPEM(caCert)
	tlsConfig.RootCAs = certPool
	cert, err := tls.X509KeyPair(sslCert, sslKey)
	if err != nil {
		return &tlsConfig, err
	}
	tlsConfig.Certificates = []tls.Certificate{cert}

	return &tlsConfig, nil
}

func generateId(n int) string {
	hash := sha256.New()
	hash.Write([]byte(time.Now().String()))
	md := hash.Sum(nil)
	mdStr := hex.EncodeToString(md)
	return mdStr[:n]
}

func parseClusterNodes(driverStatus [][]string) ([]*shipyard.Node, error) {
	nodes := []*shipyard.Node{}
	var node *shipyard.Node
	nodeComplete := false
	name := ""
	addr := ""
	containers := ""
	containersTotalAndStart := ""
	reservedCPUs := ""
	reservedMemory := ""
	reservedCPUsOnlyStart := ""
	reservedMemoryOnlyStart := ""
	labels := []string{}
	for _, l := range driverStatus {
		if len(l) != 2 {
			continue
		}
		label := l[0]
		data := l[1]

		// cluster info label i.e. "Filters" or "Strategy"
		if strings.Index(label, "\u0008") > -1 {
			continue
		}

		if strings.Index(label, " └") == -1 {
			name = label
			addr = data
		}

		// node info like "Containers"
		switch label {
		case " └ Containers":
			containers = data
		case " └ Containers Total And Start":
			containersTotalAndStart = data
		case " └ Reserved CPUs":
			reservedCPUs = data
		case " └ Reserved Memory":
			reservedMemory = data
		case " └ Labels":
			lbls := strings.Split(data, ",")
			labels = lbls
			nodeComplete = true
		case " └ Reserved  CPUs Only Start":
			reservedCPUsOnlyStart = data
		case " └ Reserved Memory Only Start":
			reservedMemoryOnlyStart = data
		default:
			continue
		}

		if nodeComplete {
			node = &shipyard.Node{
				Name:                    name,
				Addr:                    addr,
				Containers:              containers,
				ContainersTotalAndStart: containersTotalAndStart,
				ReservedCPUs:            reservedCPUs,
				ReservedMemory:          reservedMemory,
				ReservedCPUsOnlyStart:   reservedCPUsOnlyStart,
				ReservedMemoryOnlyStart: reservedMemoryOnlyStart,
				Labels:                  labels,
			}
			nodes = append(nodes, node)

			// reset info
			name = ""
			addr = ""
			containers = ""
			reservedCPUs = ""
			reservedMemory = ""
			labels = []string{}
			nodeComplete = false
		}
	}

	return nodes, nil
}


func dbExist(session *r.Session, dbName string) (bool, error) {
	var (
		dbList []string
		res    *r.Cursor
		err    error
	)

	defer func() {
		if res != nil {
			res.Close()
		}
	}()

	res, err = r.DBList().Run(session)
	if err = res.All(&dbList); err != nil {
		return false, err
	}

	for _, item := range dbList {
		if item == dbName {
			return true, nil
		}
	}
	return false, nil
}

func tableExist(session *r.Session, dbName string, tableNames ...string) (map[string]bool, error) {

	var (
		tblList []string
		err     error
		cursor  *r.Cursor
		res = make(map[string]bool, len(tableNames))
	)

	defer func() {
		if cursor != nil {
			cursor.Close()
		}
	}()

	exist, err := dbExist(session, dbName)
	if exist == false {
		return res,  errors.New("db " + dbName + " not exist.")
	}

	cursor, err = r.DB(dbName).TableList().Run(session)
	if err != nil {
		return res, err
	}

	if err = cursor.All(&tblList); err != nil {
		return res, err
	}

	for _, name := range tableNames {
		var exi = false
		for _, dbItem := range tblList {
			if name == dbItem {
				exi = true
				break
			}
		}
		
		res[name]=exi
	}

	return res, nil
}
