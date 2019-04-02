<p align="center">
  <img src ="https://raw.githubusercontent.com/TIBCOSoftware/flogo/master/images/projectflogo.png" />
</p>

<p align="center" >
  <b>Streams is a pipeline based, stream processing action for the Project Flogo Ecosystem</b>
</p>

<p align="center">
  <img src="https://travis-ci.org/project-flogo/flow.svg?branch=master"/>
  <img src="https://img.shields.io/badge/dependencies-up%20to%20date-green.svg"/>
  <img src="https://img.shields.io/badge/license-BSD%20style-blue.svg"/>
  <a href="https://gitter.im/project-flogo/Lobby?utm_source=share-link&utm_medium=link&utm_campaign=share-link"><img src="https://badges.gitter.im/Join%20Chat.svg"/></a>
</p>

# Flogo Flow

Edge devices have the potential for producing millions or even billions of events at rapid intervals, often times the events on their own are meaningless, hence the need to provide basic streaming operations against the slew of events.

A native streaming action as part of the Project Flogo Ecosystem accomplishes the following primary objectives:

- Enables apps to implement basic streaming constructs in a simple pipeline fashion
- Provides non-persistent state for streaming operations
  - Streams are persisted in memory until the end of the pipeline
- Serves as a pre-process pipeline for raw data to perform basic mathematical and logical operations. Ideal for feeding ML models

Some of the key highlights include:

😀 **Simple pipeline** construct enables a clean, easy way of dealing with streams of data<br/>
⏳ **Stream aggregation** across streams using time or event tumbling & sliding windows<br/>
🙌 **Join streams** from multiple event sources<br/>
🌪 **Filter** out the noise with stream filtering capabilities<br/>

## Getting Started

We’ve made building powerful streaming pipelines as easy as possible. Develop your pipelines using:

- A simple, clean JSON-based DSL
- Golang API

See the sample below of an aggregation pipeline (for brevity, the triggers and metadata of the resource has been omitted). Also don’t forget to check out the [examples](https://github.com/project-flogo/stream/tree/master/examples) in the repo.

```json
  "stages": [
    {
      "ref": "github.com/project-flogo/stream/activity/aggregate",
      "settings": {
        "function": "sum",
        "windowType": "timeTumbling",
        "windowSize": "5000"
      },
      "input": {
        "value": "=$.input"
      }
    },
    {
      "ref": "github.com/project-flogo/contrib/activity/log",
      "input": {
        "message": "=$.result"
      }
    }
  ]
```

## Try out the example

Firstly you should install the install the [Flogo CLI](https://github.com/project-flogo/core).
 
Next you should download our aggregation example [agg-flogo.json](https://github.com/project-flogo/stream/blob/master/examples/agg-flogo.json).

We'll create a our application using the example file, we'll call it myApp

```bash
$ flogo create -f agg-flogo.json myApp
```

Now, build it...

```bash
$ cd myApp/
$ flogo build
```

## Activities

Flogo Stream also provides some activities to assist in stream processing.

* [Aggregate](activity/aggregate/README.md) : This activity allows you to aggregate data and calculate an average or sliding average.
* [Filter](activity/filter/README.md) : This activity allows you to filter out data in a streaming pipeline.

## License 
Flogo source code in [this](https://github.com/project-flogo/strem) repository is under a BSD-style license, refer to [LICENSE](https://github.com/project-flogo/strem/blob/master/LICENSE)
