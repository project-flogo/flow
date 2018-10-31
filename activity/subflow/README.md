<!--
title: Subflow
weight: 4619
-->

# Subflow
This activity allows you to start a subflow.

## Installation
### Flogo Web
This activity comes out of the box with the Flogo Web UI
### Flogo CLI
```bash
flogo install github.com/project-flogo/flow/activity/subflow
```

## Metadata
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
_The Input/Output metadata is determined from the Input/Output metadata of the sub-flow that is being executed_

## Settings
| Setting     | Required | Description |
|:------------|:---------|:------------|
| flowURI     | true     | The URI of the flow to execute |         


## Examples
The below example executes "mysubflow" and set its input values to literals "foo" and "bar".
```json
{
  "id": "RunSubFlow",
  "activity": {
    "ref": "github.com/project-flogo/flow/activity/subflow",
    "settings" : {
      "flowURI" : "res://flow:mysubflow"
    },
    "input": {
      "flowIn1":"foo",
      "flowIn2":"bar" 
    }
  }
}
```
