package api

import (
	"fmt"
	"io/ioutil"
	"net/http"

	log "github.com/Sirupsen/logrus"
	"github.com/codegangsta/negroni"
	"github.com/gorilla/context"
	"github.com/gorilla/mux"
	"github.com/mailgun/oxy/forward"
	"github.com/shipyard/shipyard/auth"
	"github.com/shipyard/shipyard/controller/manager"
	"github.com/shipyard/shipyard/controller/middleware/access"
	"github.com/shipyard/shipyard/controller/middleware/audit"
	mAuth "github.com/shipyard/shipyard/controller/middleware/auth"
	"github.com/shipyard/shipyard/tlsutils"
	"golang.org/x/net/websocket"
)

type (
	Api struct {
		listenAddr         string
		manager            manager.Manager
		authWhitelistCIDRs []string
		enableCors         bool
		serverVersion      string
		allowInsecure      bool
		tlsCACertPath      string
		tlsCertPath        string
		tlsKeyPath         string
		dUrl               string
		fwd                *forward.Forwarder
		registryAddr       string
		waitMovingTimeout  int
	}

	ApiConfig struct {
		ListenAddr         string
		Manager            manager.Manager
		AuthWhiteListCIDRs []string
		EnableCORS         bool
		AllowInsecure      bool
		TLSCACertPath      string
		TLSCertPath        string
		TLSKeyPath         string

		RegistryAddr      string
		WaitMovingTimeout int //以秒为单位
	}

	Credentials struct {
		Username string `json:"username,omitempty"`
		Password string `json:"password,omitempty"`
	}
)

func writeCorsHeaders(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Access-Control-Allow-Origin", "*")
	w.Header().Add("Access-Control-Allow-Headers", "Origin, X-Requested-With, Content-Type, Accept")
	w.Header().Add("Access-Control-Allow-Methods", "GET, POST, DELETE, PUT, OPTIONS")
}

func NewApi(config ApiConfig) (*Api, error) {
	return &Api{
		listenAddr:         config.ListenAddr,
		manager:            config.Manager,
		authWhitelistCIDRs: config.AuthWhiteListCIDRs,
		enableCors:         config.EnableCORS,
		allowInsecure:      config.AllowInsecure,
		tlsCertPath:        config.TLSCertPath,
		tlsKeyPath:         config.TLSKeyPath,
		tlsCACertPath:      config.TLSCACertPath,
		registryAddr:       config.RegistryAddr,
		waitMovingTimeout:  config.WaitMovingTimeout,
	}, nil
}

func (a *Api) Run() error {
	globalMux := http.NewServeMux()
	controllerManager := a.manager
	client := a.manager.DockerClient()

	// forwarder for swarm
	var err error
	a.fwd, err = forward.New()
	if err != nil {
		return err
	}

	u := client.URL

	// setup redirect target to swarm
	scheme := "http://"

	// check if TLS is enabled and configure if so
	if client.TLSConfig != nil {
		log.Debug("configuring ssl for swarm redirect")
		scheme = "https://"
		// setup custom roundtripper with TLS transport
		r := forward.RoundTripper(
			&http.Transport{
				TLSClientConfig: client.TLSConfig,
			})
		f, err := forward.New(r)
		if err != nil {
			return err
		}

		a.fwd = f
	}

	a.dUrl = fmt.Sprintf("%s%s", scheme, u.Host)

	log.Debugf("configured docker proxy target: %s", a.dUrl)

	swarmRedirect := http.HandlerFunc(a.swarmRedirect)

	swarmHijack := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		a.swarmHijack(client.TLSConfig, a.dUrl, w, req)
	})

	apiRouter := mux.NewRouter()
	apiRouter.HandleFunc("/api/accounts", a.accounts).Methods("GET")
	apiRouter.HandleFunc("/api/accounts", a.saveAccount).Methods("POST")
	apiRouter.HandleFunc("/api/accounts/{username}", a.account).Methods("GET")
	apiRouter.HandleFunc("/api/accounts/{username}", a.deleteAccount).Methods("DELETE")
	apiRouter.HandleFunc("/api/roles", a.roles).Methods("GET")
	apiRouter.HandleFunc("/api/roles/{name}", a.role).Methods("GET")
	apiRouter.HandleFunc("/api/nodes", a.nodes).Methods("GET")
	apiRouter.HandleFunc("/api/nodes/{name}", a.node).Methods("GET")
	apiRouter.HandleFunc("/api/containers/{id}/scale", a.scaleContainer).Methods("POST")
	apiRouter.HandleFunc("/api/events", a.events).Methods("GET")
	apiRouter.HandleFunc("/api/events", a.purgeEvents).Methods("DELETE")
	apiRouter.HandleFunc("/api/registries", a.registries).Methods("GET")
	apiRouter.HandleFunc("/api/registries", a.addRegistry).Methods("POST")
	apiRouter.HandleFunc("/api/registries/{name}", a.registry).Methods("GET")
	apiRouter.HandleFunc("/api/registries/{name}", a.removeRegistry).Methods("DELETE")
	apiRouter.HandleFunc("/api/registries/{name}/repositories", a.repositories).Methods("GET")
	apiRouter.HandleFunc("/api/registries/{name}/repositories/{repo:.*}", a.repository).Methods("GET")
	apiRouter.HandleFunc("/api/registries/{name}/repositories/{repo:.*}", a.deleteRepository).Methods("DELETE")
	apiRouter.HandleFunc("/api/servicekeys", a.serviceKeys).Methods("GET")
	apiRouter.HandleFunc("/api/servicekeys", a.addServiceKey).Methods("POST")
	apiRouter.HandleFunc("/api/servicekeys", a.removeServiceKey).Methods("DELETE")
	apiRouter.HandleFunc("/api/webhookkeys", a.webhookKeys).Methods("GET")
	apiRouter.HandleFunc("/api/webhookkeys/{id}", a.webhookKey).Methods("GET")
	apiRouter.HandleFunc("/api/webhookkeys", a.addWebhookKey).Methods("POST")
	apiRouter.HandleFunc("/api/webhookkeys/{id}", a.deleteWebhookKey).Methods("DELETE")
	apiRouter.HandleFunc("/api/consolesession/{container}", a.createConsoleSession).Methods("GET")
	apiRouter.HandleFunc("/api/consolesession/{token}", a.consoleSession).Methods("GET")
	apiRouter.HandleFunc("/api/consolesession/{token}", a.removeConsoleSession).Methods("DELETE")

	// global handler
	globalMux.Handle("/", http.FileServer(http.Dir("static")))

	auditExcludes := []string{
		"^/containers/json",
		"^/images/json",
		"^/api/events",
	}
	apiAuditor := audit.NewAuditor(controllerManager, auditExcludes)

	// api router; protected by auth
	apiAuthRouter := negroni.New()
	apiAuthRequired := mAuth.NewAuthRequired(controllerManager, a.authWhitelistCIDRs)
	apiAccessRequired := access.NewAccessRequired(controllerManager)
	apiAuthRouter.Use(negroni.HandlerFunc(apiAuthRequired.HandlerFuncWithNext))
	apiAuthRouter.Use(negroni.HandlerFunc(apiAccessRequired.HandlerFuncWithNext))
	apiAuthRouter.Use(negroni.HandlerFunc(apiAuditor.HandlerFuncWithNext))
	apiAuthRouter.UseHandler(apiRouter)
	globalMux.Handle("/api/", apiAuthRouter)

	// account router ; protected by auth
	accountRouter := mux.NewRouter()
	accountRouter.HandleFunc("/account/changepassword", a.changePassword).Methods("POST")
	accountAuthRouter := negroni.New()
	accountAuthRequired := mAuth.NewAuthRequired(controllerManager, a.authWhitelistCIDRs)
	accountAuthRouter.Use(negroni.HandlerFunc(accountAuthRequired.HandlerFuncWithNext))
	accountAuthRouter.Use(negroni.HandlerFunc(apiAuditor.HandlerFuncWithNext))
	accountAuthRouter.UseHandler(accountRouter)
	globalMux.Handle("/account/", accountAuthRouter)

	// login handler; public
	loginRouter := mux.NewRouter()
	loginRouter.HandleFunc("/auth/login", a.login).Methods("POST")
	globalMux.Handle("/auth/", loginRouter)
	globalMux.Handle("/exec", websocket.Handler(a.execContainer))

	// hub handler; public
	hubRouter := mux.NewRouter()
	hubRouter.HandleFunc("/hub/webhook/{id}", a.hubWebhook).Methods("POST")
	globalMux.Handle("/hub/", hubRouter)

	var originRouter = mux.NewRouter()
	var prefix = "/origin"
	// these are pulled from the swarm api code to proxy and allow
	// usage with the standard Docker cli
	oriMap := map[string]map[string]http.HandlerFunc{
		"GET": {
			"/_ping":                          swarmRedirect,
			"/events":                         swarmRedirect,
			"/info":                           swarmRedirect,
			"/version":                        swarmRedirect,
			"/images/json":                    swarmRedirect,
			"/images/viz":                     swarmRedirect,
			"/images/search":                  swarmRedirect,
			"/images/get":                     swarmRedirect,
			"/images/{name:.*}/get":           swarmRedirect,
			"/images/{name:.*}/history":       swarmRedirect,
			"/images/{name:.*}/json":          swarmRedirect,
			"/containers/ps":                  swarmRedirect,
			"/containers/json":                swarmRedirect,
			"/containers/{name:.*}/export":    swarmRedirect,
			"/containers/{name:.*}/changes":   swarmRedirect,
			"/containers/{name:.*}/json":      swarmRedirect, //swarmRedirect,//连接到这个container，返回容器的详细信息
			"/containers/{name:.*}/top":       swarmRedirect,
			"/containers/{name:.*}/logs":      swarmRedirect,
			"/containers/{name:.*}/stats":     swarmRedirect,
			"/containers/{name:.*}/attach/ws": swarmHijack,
			"/exec/{execid:.*}/json":          swarmRedirect,
		},
		"POST": {
			"/auth":                         swarmRedirect,
			"/commit":                       swarmRedirect,
			"/build":                        swarmRedirect,
			"/images/create":                swarmRedirect,
			"/images/load":                  swarmRedirect,
			"/images/{name:.*}/push":        swarmRedirect,
			"/images/{name:.*}/tag":         swarmRedirect,
			"/containers/create":            swarmRedirect, //swarmRedirect,
			"/containers/{name:.*}/kill":    swarmRedirect,
			"/containers/{name:.*}/pause":   swarmRedirect,
			"/containers/{name:.*}/unpause": swarmRedirect,
			"/containers/{name:.*}/rename":  swarmRedirect,
			"/containers/{name:.*}/restart": swarmRedirect,
			"/containers/{name:.*}/start":   swarmRedirect,
			"/containers/{name:.*}/stop":    swarmRedirect,
			"/containers/{name:.*}/wait":    swarmRedirect,
			"/containers/{name:.*}/resize":  swarmRedirect,
			"/containers/{name:.*}/attach":  swarmHijack,
			"/containers/{name:.*}/copy":    swarmRedirect,
			"/containers/{name:.*}/exec":    swarmRedirect,
			"/exec/{execid:.*}/start":       swarmHijack,
			"/exec/{execid:.*}/resize":      swarmRedirect,
		},
		"DELETE": {
			"/containers/{name:.*}": swarmRedirect,
			"/images/{name:.*}":     swarmRedirect,
		},
		"OPTIONS": {
			"": swarmRedirect,
		},
	}

	for method, routes := range oriMap {
		for route, fct := range routes {
			localRoute := route
			localFct := fct
			wrap := func(w http.ResponseWriter, r *http.Request) {
				if a.enableCors {
					writeCorsHeaders(w, r)
				}
				localFct(w, r)
			}
			localMethod := method

			// add the new route
			originRouter.Path(prefix + "/v{version:[0-9.]+}" + localRoute).Methods(localMethod).HandlerFunc(wrap)
			originRouter.Path(prefix + localRoute).Methods(localMethod).HandlerFunc(wrap)
		}
	}
	originAuthRouter := negroni.New()
	originAuthRequired := mAuth.NewAuthRequired(controllerManager, a.authWhitelistCIDRs)
	originAccessRequired := access.NewAccessRequired(controllerManager)
	originAuthRouter.Use(negroni.HandlerFunc(originAuthRequired.HandlerFuncWithNext))
	originAuthRouter.Use(negroni.HandlerFunc(originAccessRequired.HandlerFuncWithNext))
	originAuthRouter.Use(negroni.HandlerFunc(apiAuditor.HandlerFuncWithNext))
	originAuthRouter.UseHandler(originRouter)
	globalMux.Handle(prefix+"/", originAuthRouter)

	// swarm
	resourcingRouter := mux.NewRouter()
	m := map[string]map[string]http.HandlerFunc{
		"GET": {
			"/_ping":                        swarmRedirect,
			"/events":                       swarmRedirect,
			"/info":                         swarmRedirect,
			"/version":                      swarmRedirect,
			"/images/json":                  swarmRedirect,
			"/images/viz":                   swarmRedirect,
			"/images/search":                swarmRedirect,
			"/images/get":                   swarmRedirect,
			"/images/{name:.*}/get":         swarmRedirect,
			"/images/{name:.*}/history":     swarmRedirect,
			"/images/{name:.*}/json":        swarmRedirect,
			"/containers/ps":                swarmRedirect,
			"/containers/json":              swarmRedirect,
			"/containers/{name:.*}/export":  a.redirectToContainer,
			"/containers/{name:.*}/changes": a.redirectToContainer,
			"/containers/{name:.*}/json":    a.inspectResource, //swarmRedirect,//连接到这个container，返回容器的详细信息
			"/containers/{name:.*}/top":     a.redirectToContainer,
			"/containers/{name:.*}/logs":    a.redirectToContainer,
			"/containers/{name:.*}/stats":   a.redirectToContainer,
			"/containers/{name:.*}/attach/ws": a.redirectToContainerHijack,
			"/exec/{execid:.*}/json": swarmRedirect,
		},
		"POST": {
			"/auth":                         swarmRedirect,
			"/commit":                       swarmRedirect,
			"/build":                        swarmRedirect,
			"/images/create":                swarmRedirect,
			"/images/load":                  swarmRedirect,
			"/images/{name:.*}/push":        swarmRedirect,
			"/images/{name:.*}/tag":         swarmRedirect,
			"/containers/create":            a.createResource, //swarmRedirect,
			"/containers/{name:.*}/kill":    a.redirectToContainer,
			"/containers/{name:.*}/pause":   a.redirectToContainer,
			"/containers/{name:.*}/unpause": a.redirectToContainer,
			"/containers/{name:.*}/rename":  a.redirectToContainer,
			"/containers/{name:.*}/restart": a.redirectToContainer,
			"/containers/{name:.*}/start":   a.startResource,
			"/containers/{name:.*}/stop":    a.redirectToContainer,
			"/containers/{name:.*}/wait":    a.redirectToContainer,
			"/containers/{name:.*}/resize":  a.redirectToContainer,
			 "/containers/{name:.*}/attach":  a.redirectToContainerHijack,
			"/containers/{name:.*}/copy": a.redirectToContainer,
			"/containers/{name:.*}/exec": a.redirectToContainer,
			"/exec/{execid:.*}/start":    swarmHijack,
			"/exec/{execid:.*}/resize":   swarmRedirect,
		},
		"DELETE": {
			"/containers/{name:.*}": a.deleteContainer,
			"/images/{name:.*}":     swarmRedirect,
		},
		"OPTIONS": {
			"": swarmRedirect,
		},
	}

	for method, routes := range m {
		for route, fct := range routes {
			localRoute := route
			localFct := fct
			wrap := func(w http.ResponseWriter, r *http.Request) {
				if a.enableCors {
					writeCorsHeaders(w, r)
				}
				localFct(w, r)
			}
			localMethod := method

			// add the new route
			resourcingRouter.Path("/v{version:[0-9.]+}" + localRoute).Methods(localMethod).HandlerFunc(wrap)
			resourcingRouter.Path(localRoute).Methods(localMethod).HandlerFunc(wrap)
		}
	}

	swarmAuthRouter := negroni.New()
	swarmAuthRequired := mAuth.NewAuthRequired(controllerManager, a.authWhitelistCIDRs)
	swarmAccessRequired := access.NewAccessRequired(controllerManager)
	swarmAuthRouter.Use(negroni.HandlerFunc(swarmAuthRequired.HandlerFuncWithNext))
	swarmAuthRouter.Use(negroni.HandlerFunc(swarmAccessRequired.HandlerFuncWithNext))
	swarmAuthRouter.Use(negroni.HandlerFunc(apiAuditor.HandlerFuncWithNext))
	swarmAuthRouter.UseHandler(resourcingRouter)
	globalMux.Handle("/containers/", swarmAuthRouter)
	globalMux.Handle("/_ping", swarmAuthRouter)
	globalMux.Handle("/commit", swarmAuthRouter)
	globalMux.Handle("/build", swarmAuthRouter)
	globalMux.Handle("/events", swarmAuthRouter)
	globalMux.Handle("/version", swarmAuthRouter)
	globalMux.Handle("/images/", swarmAuthRouter)
	globalMux.Handle("/exec/", swarmAuthRouter)
	globalMux.Handle("/v1.14/", swarmAuthRouter)
	globalMux.Handle("/v1.15/", swarmAuthRouter)
	globalMux.Handle("/v1.16/", swarmAuthRouter)
	globalMux.Handle("/v1.17/", swarmAuthRouter)
	globalMux.Handle("/v1.18/", swarmAuthRouter)
	globalMux.Handle("/v1.19/", swarmAuthRouter)
	globalMux.Handle("/v1.20/", swarmAuthRouter)

	var resourceRouter = mux.NewRouter()
	resourceRouter.HandleFunc("/resources/list", a.resources).Methods("GET")
	resourceRouter.HandleFunc("/resources/{id}", a.deleteResource).Methods("DELETE")
	resourceRouter.HandleFunc("/resources/{id}/move", a.moveResource).Methods("POST")
	resourceRouter.HandleFunc("/resources/{id}/movingprogress", a.movingProgress).Methods("GET")
	resourceAuthRouter := negroni.New()
	resourceAuthRequired := mAuth.NewAuthRequired(controllerManager, a.authWhitelistCIDRs)
	resourceAccessRequired := access.NewAccessRequired(controllerManager)
	resourceAuthRouter.Use(negroni.HandlerFunc(resourceAuthRequired.HandlerFuncWithNext))
	resourceAuthRouter.Use(negroni.HandlerFunc(resourceAccessRequired.HandlerFuncWithNext))
	resourceAuthRouter.Use(negroni.HandlerFunc(apiAuditor.HandlerFuncWithNext))
	resourceAuthRouter.UseHandler(resourceRouter)
	globalMux.Handle("/resources/", resourceAuthRouter)

	// check for admin user
	if _, err := controllerManager.Account("admin"); err == manager.ErrAccountDoesNotExist {
		// create roles
		acct := &auth.Account{
			Username:  "admin",
			Password:  "shipyard",
			FirstName: "Shipyard",
			LastName:  "Admin",
			Roles:     []string{"admin"},
		}
		if err := controllerManager.SaveAccount(acct); err != nil {
			log.Fatal(err)
		}
		log.Infof("created admin user: username: admin password: shipyard")
	}

	log.Infof("controller listening on %s", a.listenAddr)

	s := &http.Server{
		Addr:    a.listenAddr,
		Handler: context.ClearHandler(globalMux),
	}

	var runErr error

	if a.tlsCertPath != "" && a.tlsKeyPath != "" {
		log.Infof("using TLS for communication: cert=%s key=%s",
			a.tlsCertPath,
			a.tlsKeyPath,
		)

		// setup TLS config
		var caCert []byte
		if a.tlsCACertPath != "" {
			ca, err := ioutil.ReadFile(a.tlsCACertPath)
			if err != nil {
				return err
			}

			caCert = ca
		}

		serverCert, err := ioutil.ReadFile(a.tlsCertPath)
		if err != nil {
			return err
		}

		serverKey, err := ioutil.ReadFile(a.tlsKeyPath)
		if err != nil {
			return err
		}

		tlsConfig, err := tlsutils.GetServerTLSConfig(caCert, serverCert, serverKey, a.allowInsecure)
		if err != nil {
			return err
		}

		s.TLSConfig = tlsConfig

		runErr = s.ListenAndServeTLS(a.tlsCertPath, a.tlsKeyPath)
	} else {
		runErr = s.ListenAndServe()
	}

	return runErr
}
