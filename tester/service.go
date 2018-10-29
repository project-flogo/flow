package tester

import (
	"encoding/json"
	"net/http"

	"github.com/julienschmidt/httprouter"
	"github.com/project-flogo/core/support"
	"github.com/project-flogo/core/support/log"
	"github.com/project-flogo/flow/instance"
	"github.com/project-flogo/flow/service"
)

// RestEngineTester is default REST implementation of the EngineTester
type RestEngineTester struct {
	reqProcessor *RequestProcessor
	server       *Server
	enabled      bool
	logger       log.Logger
}

// NewRestEngineTester creates a new REST EngineTester
func NewRestEngineTester(config *support.ServiceConfig) *RestEngineTester {
	et := &RestEngineTester{enabled: config.Enabled}
	et.init(config.Settings)
	et.reqProcessor = NewRequestProcessor()

	//todo what logger should this use?
	et.logger = log.RootLogger()

	return et
}

func (et *RestEngineTester) Name() string {
	return service.ServiceEngineTester
}

func (et *RestEngineTester) Enabled() bool {
	return et.enabled
}

// Start implements engine.EngineTester.Start
func (et *RestEngineTester) Start() error {
	return et.server.Start()
}

// Stop implements engine.EngineTester.Stop
func (et *RestEngineTester) Stop() error {
	return et.server.Stop()
}

// Init implements engine.EngineTester.Init
func (et *RestEngineTester) init(settings map[string]string) {

	router := httprouter.New()
	router.OPTIONS("/flow/start", handleOption)
	router.POST("/flow/start", et.StartFlow)

	router.OPTIONS("/flow/restart", handleOption)
	router.POST("/flow/restart", et.RestartFlow)

	router.OPTIONS("/flow/resume", handleOption)
	router.POST("/flow/resume", et.ResumeFlow)

	router.OPTIONS("/status", handleOption)
	router.GET("/status", et.Status)

	addr := ":" + settings["port"]
	et.server = NewServer(addr, router)
}

func handleOption(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	w.Header().Add("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "*")
	w.Header().Add("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Add("Access-Control-Allow-Headers", "Origin")
	w.Header().Add("Access-Control-Allow-Headers", "X-Requested-With")
	w.Header().Add("Access-Control-Allow-Headers", "Accept")
	w.Header().Add("Access-Control-Allow-Headers", "Accept-Language")
	w.Header().Set("Content-Type", "application/json")
}

// StartFlow starts a new Flow Instance (POST "/flow/start").
//
// To post a start flow, try this at a shell:
// $ curl -H "Content-Type: application/json" -X POST -d '{"flowUri":"base"}' http://localhost:8080/flow/start
func (et *RestEngineTester) StartFlow(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {

	logger := et.logger

	w.Header().Add("Access-Control-Allow-Origin", "*")

	req := &StartRequest{}
	err := json.NewDecoder(r.Body).Decode(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	results, err := et.reqProcessor.StartFlow(req)

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	idAttr, ok := results["id"]

	if ok {

		idResponse := &instance.IDResponse{ID: idAttr.(string)}
		//idResponse, ok := data.Value.(*instance.IDResponse)

		if ok {
			logger.Debugf("Started Instance [ID:%s] for %s", idResponse.ID, req.FlowURI)

			encoder := json.NewEncoder(w)
			encoder.Encode(idResponse)
		} else {
			logger.Error("Id not returned")
			w.WriteHeader(http.StatusOK)
		}
	} else {
		w.WriteHeader(http.StatusOK)
	}
}

// RestartFlow restarts a Flow Instance (POST "/flow/restart").
//
// To post a restart flow, try this at a shell:
// $ curl -H "Content-Type: application/json" -X POST -d '{...}' http://localhost:8080/flow/restart
func (et *RestEngineTester) RestartFlow(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {

	logger := et.logger

	w.Header().Add("Access-Control-Allow-Origin", "*")

	//defer func() {
	//	if r := recover(); r != nil {
	//		logger.Error("Unable to restart flow, make sure definition registered")
	//	}
	//}()

	req := &RestartRequest{}
	err := json.NewDecoder(r.Body).Decode(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	results, err := et.reqProcessor.RestartFlow(req)

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	idAttr, ok := results["id"]

	if ok {

		idResponse := &instance.IDResponse{ID: idAttr.(string)}

		logger.Debugf("Restarted Instance [ID:%s] for %s", idResponse.ID, req.InitialState.FlowURI())

		encoder := json.NewEncoder(w)
		encoder.Encode(idResponse)
	} else {
		w.WriteHeader(http.StatusOK)
	}
}

// ResumeFlow resumes a Flow Instance (POST "/flow/resume").
//
// To post a resume flow, try this at a shell:
// $ curl -H "Content-Type: application/json" -X POST -d '{...}' http://localhost:8080/flow/resume
func (et *RestEngineTester) ResumeFlow(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {

	logger := et.logger

	w.Header().Add("Access-Control-Allow-Origin", "*")

	defer func() {
		if r := recover(); r != nil {
			logger.Error("Unable to resume flow, make sure definition registered")
		}
	}()

	req := &ResumeRequest{}
	err := json.NewDecoder(r.Body).Decode(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	results, err := et.reqProcessor.ResumeFlow(req)

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	idAttr, ok := results["id"]

	if ok {

		idResponse := &instance.IDResponse{ID: idAttr.(string)}

		logger.Debugf("Resumed Instance [ID:%s] for %s", idResponse.ID, req.State.FlowURI())

		encoder := json.NewEncoder(w)
		encoder.Encode(idResponse)
	} else {
		w.WriteHeader(http.StatusOK)
	}
}

// Status is a basic health check for the server to determine if it is up
func (et *RestEngineTester) Status(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {

	w.Header().Add("Access-Control-Allow-Origin", "*")

	w.Write([]byte(`{"status":"ok"}`))
	//w.WriteHeader(http.StatusOK)
}

func DefaultConfig() *support.ServiceConfig {
	return &support.ServiceConfig{Name: service.ServiceEngineTester, Enabled: true, Settings: map[string]string{"port": "8080"}}
}
