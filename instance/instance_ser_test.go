package instance

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

const defJSON = `
{
    "id": 1,
    "model":"test",
    "name": "test",
    "rootTask": {
      "id": 1,
      "type": 1,
      "activityType": "",
      "name": "root",
      "tasks": [
        {
          "id": 2,
          "ref": "test-log",
          "name": "a",
          "input": [
            {
              "name": "message",
              "value": "test message",
              "type": "string"
            }
          ]
        },
        {
          "id": 3,
          "ref": "test-log",
          "name": "b",
          "input": [
            {
              "name": "counterName",
              "value": "test_counter",
              "type": "string"
            }
          ]
        }
      ],
      "links": [
        { "id": 1,  "name": "","from": 2, "to": 3 }
      ]
    }
  }
`


//func TestFullSerialization(t *testing.T) {
//
//	defRep := &definition.DefinitionRep{}
//	err := json.Unmarshal([]byte(defJSON), defRep)
//	assert.Nil(t, err)
//
//	log.RootLogger().Infof("Def Rep: %v", defRep)
//
//	def, _ := definition.NewDefinition(defRep)
//	assert.NotNil(t, def)
//
//	instance, err := NewIndependentInstance("12345", "uri", def, log.RootLogger())
//	assert.Nil(t, err)
//	instance.Start(nil)
//
//	hasWork := true
//
//	for hasWork && instance.Status() < model.FlowStatusCompleted {
//		hasWork = instance.DoStep()
//
//		jsonRep, _ := json.Marshal(instance)
//		log.RootLogger().Debugf("Snapshot: %s\n", string(jsonRep))
//	}
//}

/*
func TestChangeSerialization(t *testing.T) {

	defRep := &definition.DefinitionRep{}
	err := json.Unmarshal([]byte(defJSON), defRep)
	assert.Nil(t, err)

	log.RootLogger().Infof("Def Rep: %v", defRep)

	def, _ := definition.NewDefinition(defRep)
	assert.NotNil(t, def)

	instance, err := NewIndependentInstance("12345", "uri", def, log.RootLogger())
	assert.Nil(t, err)
	instance.Start(nil)

	hasWork := true

	for hasWork && instance.Status() < model.FlowStatusCompleted {
		hasWork = instance.DoStep()

		json, _ := json.Marshal(instance.ChangeTracker)
		log.RootLogger().Debugf("Change: %s\n", string(json))
	}
}
*/
//func TestIncrementalSerialization(t *testing.T) {
//
//	defRep := &flowdef.DefinitionRep{}
//	json.Unmarshal([]byte(defJSON), defRep)
//
//	idGen, _ := util.NewGenerator()
//	id := idGen.NextAsString()
//
//	def, _ := flowdef.NewDefinition(defRep)
//
//	instance := NewFlowInstance(id, "uri2", def)
//
//	instance.Start(nil)
//
//	hasWork := true
//
//	for hasWork && instance.Status() < StatusCompleted {
//		hasWork = instance.DoStep()
//
//		json, _ := json.Marshal(instance.GetChanges())
//		log.Debugf("Changes: %s\n", string(json))
//	}
//}
