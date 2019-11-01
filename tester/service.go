package tester

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/julienschmidt/httprouter"
	"github.com/project-flogo/core/data/coerce"
	"github.com/project-flogo/core/support/log"
	"github.com/project-flogo/core/support/service"
	"github.com/project-flogo/flow/instance"
)

func init() {
	_ = service.RegisterFactory(&FlowTesterFactory{})
}

type FlowTesterFactory struct {
}

func (s *FlowTesterFactory) NewService(config *service.Config) (service.Service, error) {
	ft := &RestFlowTester{}
	err := ft.init(config.Settings)
	if err != nil {
		return nil, err
	}

	ft.reqProcessor = NewRequestProcessor()

	//todo switch this logger
	ft.logger = log.RootLogger()

	return ft, nil
}

// RestFlowTester is default REST implementation of the EngineTester
type RestFlowTester struct {
	reqProcessor *RequestProcessor
	server       *Server
	logger       log.Logger
}

func (ft *RestFlowTester) Name() string {
	return "FlowTester"
}

// Start implements engine.EngineTester.Start
func (ft *RestFlowTester) Start() error {
	return ft.server.Start()
}

// Stop implements engine.EngineTester.Stop
func (ft *RestFlowTester) Stop() error {
	return ft.server.Stop()
}

// Init implements engine.EngineTester.Init
func (ft *RestFlowTester) init(settings map[string]interface{}) error {

	router := httprouter.New()
	router.OPTIONS("/flow/start", handleOption)
	router.POST("/flow/start", ft.StartFlow)

	router.OPTIONS("/flow/restart", handleOption)
	router.POST("/flow/restart", ft.RestartFlow)

	router.OPTIONS("/flow/resume", handleOption)
	router.POST("/flow/resume", ft.ResumeFlow)

	router.OPTIONS("/status", handleOption)
	router.GET("/status", ft.Status)

	port := 8080
	var err error
	sPort, set := settings["port"]
	if set {
		port, err = coerce.ToInt(sPort)
		if err != nil {
			return err
		}
	}

	addr := ":" + strconv.Itoa(port)
	ft.server = NewServer(addr, router)

	return nil
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
func (ft *RestFlowTester) StartFlow(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {

	logger := ft.logger

	w.Header().Add("Access-Control-Allow-Origin", "*")

	req := &StartRequest{}
	err := json.NewDecoder(r.Body).Decode(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	results, err := ft.reqProcessor.StartFlow(req)

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
			err := encoder.Encode(idResponse)
			if err != nil {
				logger.Errorf("Unable to encode response: %v", err)
			}
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
func (ft *RestFlowTester) RestartFlow(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {

	logger := ft.logger

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

	results, err := ft.reqProcessor.RestartFlow(req)

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	idAttr, ok := results["id"]

	if ok {

		idResponse := &instance.IDResponse{ID: idAttr.(string)}

		logger.Debugf("Restarted Instance [ID:%s] for %s", idResponse.ID, req.InitialState.FlowURI())

		encoder := json.NewEncoder(w)
		err := encoder.Encode(idResponse)
		if err != nil {
			logger.Errorf("Unable to encode response: %v", err)
		}
	} else {
		w.WriteHeader(http.StatusOK)
	}
}

// ResumeFlow resumes a Flow Instance (POST "/flow/resume").
//
// To post a resume flow, try this at a shell:
// $ curl -H "Content-Type: application/json" -X POST -d '{...}' http://localhost:8080/flow/resume
func (ft *RestFlowTester) ResumeFlow(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {

	logger := ft.logger

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

	results, err := ft.reqProcessor.ResumeFlow(req)

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	idAttr, ok := results["id"]

	if ok {

		idResponse := &instance.IDResponse{ID: idAttr.(string)}

		logger.Debugf("Resumed Instance [ID:%s] for %s", idResponse.ID, req.State.FlowURI())

		encoder := json.NewEncoder(w)
		err := encoder.Encode(idResponse)
		if err != nil {
			logger.Errorf("Unable to encode response: %v", err)
		}
	} else {
		w.WriteHeader(http.StatusOK)
	}
}

// Status is a basic health check for the server to determine if it is up
func (ft *RestFlowTester) Status(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {

	w.Header().Add("Access-Control-Allow-Origin", "*")

	_, _ = w.Write([]byte(`{"status":"ok"}`))
	//w.WriteHeader(http.StatusOK)
}
