---
title: Subflow
weight: 4619
---

# Subflow
This activity allows you to start a subflow.

## Installation
### Flogo Web
This activity comes out of the box with the Flogo Web UI
### Flogo CLI
```bash
flogo install github.com/TIBCOSoftware/flogo-contrib/activity/subflow
```

## Schema
```json
{
  "settings":[
    {
      "name": "flowURI",
      "type": "string",
      "required": true
    }
  ]
}
```
_The Input/Output schema is determined from the Input/Output metadata of the subflow that is being executed_

## Settings
| Setting     | Required | Description |
|:------------|:---------|:------------|
| flowURI     | True     | The URI of the flow to execute |         


## Examples
The below example executes "mysubflow" and set its input values to literals "foo" and "bar".
```json
{
  "id": "RunSubFlow",
  "activity": {
    "ref": "github.com/TIBCOSoftware/flogo-contrib/activity/subflow",
    "settings" : {
      "flowURI" : "res://flow:mysubflow"
    },
    "input": { 
  	  "mappings":[
        { "type": "literal", "value": "foo", "mapTo": "FlowIn1" },
        { "type": "literal", "value": "bar", "mapTo": "FlowIn2" }
      ]
    }
  }
}
```
