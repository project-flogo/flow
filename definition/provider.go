package definition

// ExtensionProvider is the interface that describes an object
// that can provide flow definitions from a URI
type Provider interface {

	// GetFlow retrieves the flow definition for the specified uri
	GetFlow(flowURI string) (*DefinitionRep, error)

	//// AddCompressedFlow adds the flow for a specified id
	//AddCompressedFlow(id string, flow string) error
	//// AddUnCompressedFlow adds the flow for a specified id
	//AddUncompressedFlow(id string, flow []byte) error
	//// AddFlowURI adds the flow for a specified uri
	//AddFlowURI(id string, uri string) error
}

//// RemoteFlowProvider is an implementation of FlowProvider service
//// that can access flows via URI
//type RemoteFlowProvider struct {
//	//todo: switch to LRU cache
//	mutex     *sync.Mutex
//	flowCache map[string]*definition.Definition
//	flowMgr   *support.FlowManagerOld
//}
//
//// NewRemoteFlowProvider creates a RemoteFlowProvider
//func NewRemoteFlowProvider() *RemoteFlowProvider {
//	var service RemoteFlowProvider
//	service.flowCache = make(map[string]*definition.Definition)
//	service.mutex = &sync.Mutex{}
//	service.flowMgr = support.NewFlowManager()
//	return &service
//}
//
//func (pps *RemoteFlowProvider) Name() string {
//	return service.ServiceFlowProvider
//}
//
//// Start implements util.Managed.Start()
//func (pps *RemoteFlowProvider) Start() error {
//	// no-op
//	return nil
//}
//
//// Stop implements util.Managed.Stop()
//func (pps *RemoteFlowProvider) Stop() error {
//	// no-op
//	return nil
//}
//
//// GetFlow implements flow.ExtensionProvider.GetFlow
//func (pps *RemoteFlowProvider) GetFlow(id string) (*definition.Definition, error) {
//
//	// todo turn pps.flowCache to real cache
//	if flow, ok := pps.flowCache[id]; ok {
//		logger.Debugf("Accessing cached Flow: %s\n")
//		return flow, nil
//	}
//
//	logger.Debugf("Getting Flow: %s\n", id)
//
//	flowRep, err := pps.flowMgr.GetFlow(id)
//	if err != nil {
//		return nil, err
//	}
//
//	def, err := definition.NewDefinition(flowRep)
//	if err != nil {
//		errorMsg := fmt.Sprintf("Error unmarshalling flow '%s': %s", id, err.Error())
//		logger.Errorf(errorMsg)
//		return nil, fmt.Errorf(errorMsg)
//	}
//
//	//todo hack until we fully move over to new action implementation
//	factory := definition.GetLinkExprManagerFactory()
//
//	if factory == nil {
//		factory = &fggos.GosLinkExprManagerFactory{}
//	}
//
//	def.SetLinkExprManager(factory.NewLinkExprManager(def))
//
//	//synchronize
//	pps.mutex.Lock()
//	pps.flowCache[id] = def
//	pps.mutex.Unlock()
//
//	return def, nil
//
//}
//
//func (pps *RemoteFlowProvider) AddCompressedFlow(id string, flow string) error {
//	return pps.flowMgr.AddCompressed(id, flow)
//}
//
//func (pps *RemoteFlowProvider) AddUncompressedFlow(id string, flow []byte) error {
//	return pps.flowMgr.AddUncompressed(id, flow)
//}
//
//func (pps *RemoteFlowProvider) AddFlowURI(id string, uri string) error {
//	return pps.flowMgr.AddURI(id, uri)
//}
//
//func DefaultConfig() *util.ServiceConfig {
//	return &util.ServiceConfig{Name: service.ServiceFlowProvider, Enabled: true}
//}
