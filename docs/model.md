# Flow Model


The Flow Model is used to define a basic process flow.  The flow is essentially a DAG (Directed Acyclic Graph), which basically means it is a directed graph with no cycles.  You can think of it as a flow chart with no lines looping back.  A graph typically consist of nodes and edges.  In this model, Tasks are the equivalent of nodes and Links are directed edges.  Tasks are associated with Flogo Activities and this structure is used to orchestrate those activities.  This basically allows you to create an application using a simple json or you can use our UI to compose the flow.

The model also provides some additional task constructs to get around some of the limitations of a simple DAG.  For example, there are both Iterator and DoWhile tasks that can be used to create looping constructs.  These are used to loop over a specified activity.  If more complex logic is needed within that loop, one can also the [Subflow](activity/subflow/README.md) activity which can run another flow.

## JSON DSL
Sections:


* [Metadata](#metadata "Goto Metadata") - Flow Input/Output Metadata
* [Tasks](#tasks "Goto Tasks") - Flow Tasks
* [Links](#links "Goto Links") - Flow Links
* [ErrorHandler](#errorHandler "Goto ErrorHandler") - Flow Error Handler
    
[Full Example](#full-example "Full Example") 


## Metadata
The `metadata` section allows one to define all inputs and outputs of a flow.

```json
"metadata": {
  "input": [
    {
      "name": "in",
      "type": "string"
    }
  ],
  "output": [
    {
      "name": "out",
      "type": "string"
    }  
  ]
}
```



## Tasks
The `tasks` section allows one to define the tasks that are part of the flow. Tasks are associated with the Flogo activity you would like to execute.  In this case a log activity.

```json
"tasks": [
  {
    "id": "log_1",
    "name": "Log 1",
    "activity": {
      "ref": "#log",
      "input": {
        "message": "=$flow.in"
      }
    }
  },
  ...
]
```

#### Iterator Task
There are also special types of tasks that can be part of a flow.  An `iterator` lets you iterate for a specified count or over an array, map or object. 

Iterators take an `iterate` setting.  This can be set to a number or use a mapping that evaluates to an object.  It also takes an optional `delay` setting that can be used to set a delay in millseconds between iterations.

Count Example:  
 
```json
  {
    "id": "loop_log",
    "name": "Loop Log",
    "type": "iterator",
    "settings": {
      "iterate": 10,
      "delay": 5
    },
    "activity": {
      "ref": "#log",
      "input": {
        "message": "=$iteration[key]"
      }
    }
  }
```

Object Example:  
 
```json
  {
    "id": "loop_log",
    "name": "Loop Log",
    "type": "iterator",
    "settings": {
      "iterate": "=$flow.orders"
    },
    "activity": {
      "ref": "#log",
      "input": {
        "message": "=$iteration[value]"
      }
    }
  }
```

*Note:  `$iteration[index]` is also available to get the index of the current iteration.*

#### DoWhile Task
A `doWhile` task lets you loop over an activity using a conditional expression to determine how often to loop.

DoWhile tasks take a `condition` setting which is where the conditional expression is specified.  It also takes an optional `delay` setting that can be used to set a delay in millseconds between iterations.

```json
{
    "id": "RESTInvoke",
    "name": "RESTInvoke",
    "description": "Invokes a REST Service",
    "type": "doWhile",
    "settings": {
      "condition": "$iteration[index] < 5",
      "delay": 5
    },
    "activity": {
      "ref": "#rest",
      ...
    }
}
```

#### Accumulate
Both `iterate` and `doWhile` tasks support an `accumlate` setting.  If this setting is enabled, all the outputs of the invoked activity are accumulated in an array.

To Enable:

```json
{
    "id": "RESTInvoke",
    "name": "RESTInvoke",
    "type": "doWhile",
    "settings": {
      "condition": "=$iteration[index] < 5",
      "delay": 5,
      "accumulate": true
    },
    ...
}
```

Access Accumlated Values:

```json
  {
    "id": "log",
    "name": "Log",
    "activity": {
      "ref": "#log",
      "input": {
        "message": "=$activity[RESTInvoke][0]"
      }
    }
  }
```

*Note: This logs the output of the first iteration*

#### Retry

When an activity encounters an error, they may report that error is "Retriable".  In that case a `retryOnError ` setting can be set on the task that tells it to retry those errors.  You can indicate both how many retries and the iterval in milliseconds in which to retry.

```
{
    "id": "RESTInvoke",
    "name": "RESTInvoke",
    "type": "doWhile",
    "settings": {
      "retryOnError": {
        "count": 3,
        "interval": 1000
      }
    },
    ...
}

```

## Links
The `links` section allows one to define the links in the flow.  The links are used to define how one tasks connects to another.  In the following example we are indicating that task `log_2` comes after task `log_1`.  

```json
"links": [
  {
    "from": "log_1",
    "to": "log_2"
  }
]
```

#### Conditional/Expression Links
We can also have conditional links.  These are links that have a conditional expression and are only followed if that expression evaluates to true.

```json
"links": [
  {
    "from": "validateOrder",
    "to": "sendToManager",
    "type": "expression",
    "value": "$flow.amount > 1000"
  }
]
```

#### Otherwise Expression Links
When using conditional links, you might want to have a link that gets followed only if none of your conditional links evaluated to true. In this case you would use the "otherwise" link.

```json
"links": [
  {
    "from": "validateOrder",
    "to": "processOrder",
    "type": "exprOtherwise"
  }
]
```

#### Error Links
Error links can be used to explicitly handle errors from specific tasks.  When an error link is present, and there is an error, that link will be followed instead of escalating to the global error handler.

```json
"links": [
  {
    "from":"validateOrder",
    "to":"orderError",
    "type":"error"
  }
]
```

## ErrorHandler
The `errorHandler` section is used to define the global error handler for the flow.  If an error occurs that isn't explicitly handled by a task/error link, the error is escalted to the errorHandler.  The error handler is like a mini "flow", it has both a `tasks` and `links` section.
 
```json
"errorHandler": {
   "tasks": [
      { 
         "id":"SecondLog",
         "activity":{ 
             "ref":"github.com/TIBCOSoftware/flogo-contrib/activity/log",
             "input":{ 
                 "message": "log in error handler"
             },
             "output":{ },
             "mappings":{ }
         }
      }
   ]
}
```
*Note:  When an unhandled error occurs, if an errorHandler isn't defined in the flow, the flow automatically fails and returns an error.*


## Full Example
Sample flogo application configuration file. 

```json
{
  "name": "simpleApp",
  "type": "flogo:app",
  "version": "0.0.1",
  "appModel": "1.0.0",
  "description": "My flogo application description",
  "imports": [
    "github.com/project-flogo/flow",
    "github.com/project-flogo/contrib/trigger/rest",
    "github.com/project-flogo/contrib/activity/log"
  ],
  "triggers": [
    {
      "id": "my_rest_trigger",
      "ref": "#rest",
      "settings": {
        "port": "9233"
      },
      "handlers": [
        {
          "settings": {
            "method": "GET",
            "path": "/test"
          },
          "action": {
            "ref": "#flow",
            "settings": {
              "flowURI": "res://flow:myflow"
            },
            "input": {
              "orderType": "standard"
            },
            "output": {
              "data": "=$.value"
            }
          }
        }
      ]
    }
  ],
  "resources": [
    {
      "id": "flow:myflow",
      "data": {
        "name": "My Flow",
        "description": "Example description",
        "metadata": {
          "input": [
            { "name":"customerId", "type":"string" },
            { "name":"orderId", "type":"string" },
            { "name":"orderType", "type":"string" }
          ],
          "output":[
            { "name":"value", "type":"string" }
          ]
        },
        "tasks": [
          {
            "id": "FirstLog",
            "name": "FirstLog",
            "type": "iterator",
            "settings": {
              "iterate": 10
            },
            "activity": {
              "ref": "#log",
              "input": {
                "message": "=$iteration[key]"
              }
            }
          },
          {
            "id": "SecondLog",
            "name": "SecondLog",
            "activity" : {
              "ref": "#log",
              "input": {
                "message": "test message"
              }
            }
          }
        ],
        "links": [
          {
            "from": "FirstLog",
            "to": "SecondLog",
            "type": "expression",
            "value": "$flow.orderId > 1000"
          }
        ],
        "errorHandler": {
          "tasks": [
            {
              "id": "ErrorLog",
              "activity": {
                "ref": "#log",
                "input": {
                  "message": "log in error handler"
                }
              }
            }
          ]
        }
      }
    }
  ]
}
```
