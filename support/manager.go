package support

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"sync"

	"github.com/project-flogo/core/support"
	"github.com/project-flogo/core/support/log"
	"github.com/project-flogo/flow/definition"
)

const (
	uriSchemeFile = "file://"
	uriSchemeHttp = "http://"
)

type FlowManager struct {
	//todo switch to cache
	rfMu         sync.Mutex // protects the flow maps
	remoteFlows  map[string]*definition.Definition
	flowProvider definition.Provider
}

func NewFlowManager(flowProvider definition.Provider) *FlowManager {
	manager := &FlowManager{}

	if flowProvider != nil {
		manager.flowProvider = flowProvider
	} else {
		//todo which logger should this use?
		manager.flowProvider = &BasicRemoteFlowProvider{logger: log.RootLogger()}
	}

	return manager
}

func (fm *FlowManager) GetFlow(uri string) (*definition.Definition, error) {

	fm.rfMu.Lock()
	defer fm.rfMu.Unlock()

	if fm.remoteFlows == nil {
		fm.remoteFlows = make(map[string]*definition.Definition)
	}

	flow, exists := fm.remoteFlows[uri]

	if !exists {

		defRep, err := fm.flowProvider.GetFlow(uri)
		if err != nil {
			return nil, err
		}

		flow, err = materializeFlow(defRep)
		if err != nil {
			return nil, err
		}

		fm.remoteFlows[uri] = flow
	}

	return flow, nil
}

type BasicRemoteFlowProvider struct {
	logger log.Logger
}

func (fp *BasicRemoteFlowProvider) GetFlow(flowURI string) (*definition.DefinitionRep, error) {

	logger := fp.logger

	var flowDefBytes []byte

	if strings.HasPrefix(flowURI, uriSchemeFile) {
		// File URI
		logger.Infof("Loading Local Flow: %s\n", flowURI)
		flowFilePath, _ := support.URLStringToFilePath(flowURI)

		readBytes, err := ioutil.ReadFile(flowFilePath)
		if err != nil {
			readErr := fmt.Errorf("error reading flow with uri '%s', %s", flowURI, err.Error())
			logger.Errorf(readErr.Error())
			return nil, readErr
		}
		if readBytes[0] == 0x1f && readBytes[2] == 0x8b {
			flowDefBytes, err = unzip(readBytes)
			if err != nil {
				decompressErr := fmt.Errorf("error uncompressing flow with uri '%s', %s", flowURI, err.Error())
				logger.Errorf(decompressErr.Error())
				return nil, decompressErr
			}
		} else {
			flowDefBytes = readBytes

		}

	} else if strings.HasPrefix(flowURI, uriSchemeHttp) {
		req, err := http.NewRequest("GET", flowURI, nil)
		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			getErr := fmt.Errorf("error getting flow with uri '%s', %s", flowURI, err.Error())
			logger.Errorf(getErr.Error())
			return nil, getErr
		}
		defer resp.Body.Close()

		logger.Infof("response Status:", resp.Status)

		if resp.StatusCode >= 300 {
			//not found
			getErr := fmt.Errorf("error getting flow with uri '%s', status code %d", flowURI, resp.StatusCode)
			logger.Errorf(getErr.Error())
			return nil, getErr
		}

		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			readErr := fmt.Errorf("error reading flow response body with uri '%s', %s", flowURI, err.Error())
			logger.Errorf(readErr.Error())
			return nil, readErr
		}

		val := resp.Header.Get("flow-compressed")
		if strings.ToLower(val) == "true" {
			decodedBytes, err := decodeAndUnzip(string(body))
			if err != nil {
				decodeErr := fmt.Errorf("error decoding compressed flow with uri '%s', %s", flowURI, err.Error())
				logger.Errorf(decodeErr.Error())
				return nil, decodeErr
			}
			flowDefBytes = decodedBytes
		} else {
			flowDefBytes = body
		}
	} else {
		return nil, fmt.Errorf("unsupport uri %s", flowURI)
	}

	var flow *definition.DefinitionRep
	err := json.Unmarshal(flowDefBytes, &flow)
	if err != nil {
		logger.Errorf(err.Error())
		return nil, fmt.Errorf("error marshalling flow with uri '%s', %s", flowURI, err.Error())
	}

	return flow, nil
}

func decodeAndUnzip(encoded string) ([]byte, error) {

	decoded, _ := base64.StdEncoding.DecodeString(encoded)
	return unzip(decoded)
}

func unzip(compressed []byte) ([]byte, error) {

	buf := bytes.NewBuffer(compressed)
	r, err := gzip.NewReader(buf)
	if err != nil {
		return nil, err
	}
	jsonAsBytes, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}

	return jsonAsBytes, nil
}

func materializeFlow(flowRep *definition.DefinitionRep) (*definition.Definition, error) {

	def, err := definition.NewDefinition(flowRep)
	if err != nil {
		return nil, fmt.Errorf("error unmarshalling flow: %s", err.Error())
	}

	//todo validate flow

	//factory := definition.GetExprFactory()

	//if factory == nil {
	//	factory = linker.NewDefaultLinkerFactory()
	//}

	//def.SetLinkExprManager(factory.NewLinkExprManager())
	//todo init activities

	return def, nil

}
