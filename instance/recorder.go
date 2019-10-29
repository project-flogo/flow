package instance

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/project-flogo/core/support"
	"github.com/project-flogo/core/support/log"
	"github.com/project-flogo/flow/service"
)

// StateRecorder is the interface that describes a service that can record
// snapshots and steps of a Flow Instance
type StateRecorder interface {
	// RecordSnapshot records a Snapshot of the FlowInstance
	RecordSnapshot(instance *IndependentInstance)

	// RecordStep records the changes for the current Step of the Flow Instance
	RecordStep(instance *IndependentInstance)
}

// RemoteStateRecorder is an implementation of StateRecorder service
// that can access flows via URI
type RemoteStateRecorder struct {
	host    string
	enabled bool
	logger  log.Logger
}

// NewRemoteStateRecorder creates a new RemoteStateRecorder
func NewRemoteStateRecorder(config *support.ServiceConfig) *RemoteStateRecorder {

	recorder := &RemoteStateRecorder{enabled: config.Enabled}
	recorder.init(config.Settings)

	//todo switch this logger
	recorder.logger = log.RootLogger()

	return recorder
}

func (sr *RemoteStateRecorder) Name() string {
	return service.ServiceStateRecorder
}

func (sr *RemoteStateRecorder) Enabled() bool {
	return sr.enabled
}

// Start implements util.Managed.Start()
func (sr *RemoteStateRecorder) Start() error {
	// no-op
	return nil
}

// Stop implements util.Managed.Stop()
func (sr *RemoteStateRecorder) Stop() error {
	// no-op
	return nil
}

// Init implements services.StateRecorderService.Init()
func (sr *RemoteStateRecorder) init(settings map[string]string) {

	host, set := settings["host"]
	port, set := settings["port"]

	if !set {
		panic("RemoteStateRecorder: required setting 'host' not set")
	}

	if strings.Index(host, "http") != 0 {
		sr.host = "http://" + host + ":" + port
	} else {
		sr.host = host + ":" + port
	}

	sr.logger.Debugf("RemoteStateRecorder: StateRecorder Server = %s", sr.host)
}

// RecordSnapshot implements instance.StateRecorder.RecordSnapshot
func (sr *RemoteStateRecorder) RecordSnapshot(instance *IndependentInstance) {

	storeReq := &RecordSnapshotReq{
		ID:           instance.StepID(),
		FlowID:       instance.ID(),
		Status:       int(instance.Status()),
		SnapshotData: instance,
	}

	uri := sr.host + "/instances/snapshot"

	sr.logger.Debugf("POST Snapshot: %s\n", uri)

	jsonReq, _ := json.Marshal(storeReq)

	sr.logger.Debug("JSON: ", string(jsonReq))

	req, err := http.NewRequest("POST", uri, bytes.NewBuffer(jsonReq))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	sr.logger.Debug("response Status:", resp.Status)

	if resp.StatusCode >= 300 {
		//error
	}
}

// RecordStep implements instance.StateRecorder.RecordStep
func (sr *RemoteStateRecorder) RecordStep(instance *IndependentInstance) {

	storeReq := &RecordStepReq{
		ID:       instance.StepID(),
		FlowID:   instance.ID(),
		Status:   int(instance.Status()),
		StepData: instance.changeTracker,
		FlowURI:  instance.flowURI,
	}

	uri := sr.host + "/instances/steps"

	sr.logger.Debugf("POST Step: %s\n", uri)

	jsonReq, _ := json.Marshal(storeReq)

	sr.logger.Debug("JSON: ", string(jsonReq))

	req, err := http.NewRequest("POST", uri, bytes.NewBuffer(jsonReq))

	//todo fix this
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	sr.logger.Debug("response Status:", resp.Status)

	if resp.StatusCode >= 300 {
		//error
	}
}

// RecordSnapshotReq serializable representation of the RecordSnapshot request
type RecordSnapshotReq struct {
	ID     int    `json:"id"`
	FlowID string `json:"flowID"`
	State  int    `json:"state"`
	Status int    `json:"status"`

	SnapshotData *IndependentInstance `json:"snapshotData"`
}

// RecordStepReq serializable representation of the RecordStep request
type RecordStepReq struct {
	ID     int    `json:"id"`
	FlowID string `json:"flowID"`

	//todo should move to the "stepData"
	State  int `json:"state"`
	Status int `json:"status"`
	//todo we should have initial "init" to associate flowURI with flowID, instead of at every step
	FlowURI string `json:"flowURI"`

	StepData ChangeTracker `json:"stepData"`
}

func DefaultConfig() *support.ServiceConfig {
	return &support.ServiceConfig{Name: service.ServiceStateRecorder, Enabled: true, Settings: map[string]string{"host": ""}}
}
