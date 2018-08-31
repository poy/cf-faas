# CF-FaaS
CloudFoundry Functions as a Service

CF-FaaS is a [Cloud Foundry][cloud-foundry] application that enables
developers to build via functions. It offers many advantages to building a
vanilla applications:

1. Autoscaling out of the box
1. Configurable caching for GET results (via [google's groupcache][groupcache])
1. Minimize application instance count
1. Keeps `cf push` experience
1. Decouple endpoints
1. Extendable. New events can be added to suit the needs of the system.

### Install

The easiest way to install CF-FaaS is via a provided install bash script. It
assumes the user is logged into the CF cli and that the desired space is
already targeted.

```
$ scripts/install.sh -h
Usage: scripts/install.sh [-a:m:b:p:h]
 -a application name (REQUIRED) - The given name (and route) for CF-FaaS.
 -m manifest path (REQUIRED)    - The path to the YAML file that configures the endpoints.
 -b bootstrap manifest path     - The path to the YAML file that configures the bootstrap endpoints.
 -r resolver URLS               - Comma separated list of key values (key:value). Each key-value pair is an event
                                  name and URL (e.g., eventname:some.url/v1/path,other-event:/v2/bootstrap/path).
 -h help                        - Shows this usage.

More information available at https://github.com/apoydence/cf-faas
```

### Configuration
CF-FaaS is a CF application, so all configuration is done through environment
variables (`cf set-env`). If you choose to use the install script, then most
of it is taken care of for you.

#### cf-faas
| Property | Required | Description |
|--------------------|----------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| MANIFEST | Required | The manifest (in YAML) that configures the functions. |
| BOOTSTRAP_MANIFEST | Optional | The manifest (in YAML) that adds function handlers for resolving. The manifest is of the format `HTTPManifest` (meaning it does not have different event types. Only `http`). These handlers are unavailable after resolving is complete. |
| RESOLVER_URLS | Optional | Resolver URLs are a key value pair (e.g., `key1:value1,key2:value2`) of event names to URLs. These are required when using non `http` event types. The URL should NOT include a scheme. Instead `http` will be added (e.g., `queue:/v1/resolve/queue,twitter:some.url/twitter`). |

#### worker
| Property | Required | Description |
|----------|----------|----------------------------------------------------------|
| DATA_DIR | Optional | The directory to store packages. Defaults to `/dev/shm`. |

### Manifest

CF-FaaS operates off of a provided manifest. The manifest configures all the
endpoints and corresponding functions.

```
---

functions:
- handler:
    app_name: faas-fibonacci # 1
    command: ./fibonacci # 2
  events:
    http: # 3
    - path: /v1/fibonacci # 4
      method: GET # 5
      no_auth: true # 6
      cache:
        duration: 5m # 7
        header: # 8
        - Authorization
```

Lets break down the previous example.

##### 1. Application Name (e.g., `faas-fibonacci)
The application name is used to find the bits that are required to execute the
given command. The app must be in the same space. The application must be
staged. The application does not need to have any instances. Pushing an app
for CF-FaaS should look like:
```
cf push <app-name> -b binary_buildpack -i 0
```

**NOTE** Only the binary buildpack is currently supported.

##### 2. Command (e.g., `./fibonacci`)
The bash command ran to service the request. If the command returns a non-0
exit code before POSTing the results, it will result in a 500 status code.

The command is executed within bash:
```
bash -c "<command>"
```

##### 3. Event name (e.g., `http`)
Event names are used to determine how to parse the YAML. CF-FaaS only
recognized `http` events. Any other event must be resolved. Resolver URLs are
provided (via an environment variable `RESOLVER_URLS`), to map event names to
RESTful endpoints. The endpoints are used to convert any event type into a
series of `http` events.

##### 4. Path (e.g., `/v1/fibonacci`)
The path is used to route requests to different functions. Each path must be
unique. [gorilla/mux][gorilla-mux] is used to route requests, therefore the
pattern matching and URL variable extraction is available to function.

The path
```
/v1/users/{user-name}
```

Has the `user-name` URL variable available. The value will be extracted and
made available to the receiving function.

##### 5. Method (e.g., `GET`)
The method is used for routing requests. If the method is `GET`, then caching
is available.

##### 6. No Auth (e.g., `true`)
Setting `no_auth` to true will all the corresponding path to not require an
authorization header to be set. If it is set to `false` or left out, it will
require one.

##### 7. Cache Duration (e.g., `5m`)
The cache duration is how long a request will be cached for before becoming
invalid. Caching is only available with `GET` requests.
[Groupcache][groupcache] is used to cache results.

##### 8. Cache Header (e.g., `Authorization`)
Any listed header is used to determine uniqueness with the request. If two
requests are identical (i.e., same path), but were to have different
`Authorization` header values, then they would have their cache values
available to eachother.

### Bootstrap Manifest
```
---

functions:
- handler:
    app_name: faas-plugin-postprinter
    command: ./plugin
  events:
  - path: /v1/plugins/queue
    method: POST
```

The bootstrap manifest is similar to the before mentioned manifest. It has one
key difference, instead of a map of event types to handlers, every event is
`http`. The bootstrap manifest is used for resolving event types.

### API
Each function has must follow a protocol:

1. Do a `GET` request to an address that is at the environment variable
   `CF_FAAS_RELAY_ADDR` with the header `X-CF-APP-INSTANCE` set to the
environment variable value of `X_CF_APP_INSTANCE`. This will fetch the request
from the server. The JSON format returned is

```go
type Request struct {
	Path         string            `json:"path"`
	URLVariables map[string]string `json:"url_variables"`
	Method       string            `json:"method"`
	Header       http.Header       `json:"headers"`
	Body         []byte            `json:"body"`
}
```

2. Once the work of the function is complete, do a `POST` request to the same
   `CF_FAAS_RELAY_ADDR` with the same `X-CF-APP-INSTANCE` header with a JSON
payload of the following format

```go
type Response struct {
	StatusCode int         `json:"status_code"`
	Header     http.Header `json:"header"`
	Body       []byte      `json:"body"`
}
```

This will complete the transaction and the user will receive the given result.

### Resolver API
The resolver API is used to resolve event types into `http` events. The
resolver endpoint will be hit with a `POST` request with the a
`ConvertRequest`.

```go
type ConvertRequest struct {
	Functions []ConvertFunction `json:"functions"`
}

type GenericData map[string]interface{}

type ConvertFunction struct {
	Handler ConvertHandler           `json:"handler"`
	Events  map[string][]GenericData `json:"events"`
}

type ConvertHandler struct {
	Command string `json:"command"`
	AppName string `json:"app_name,omitempty"`
}
```

The resuling body must be a `ConvertResponse`.

```go
type ConvertResponse struct {
	Functions []ConvertHTTPFunction `json:"functions"`
}

type ConvertHTTPFunction struct {
	Handler ConvertHandler     `json:"handler"`
	Events  []ConvertHTTPEvent `json:"events"`
}

type ConvertHTTPEvent struct {
	Path   string `yaml:"path"`
	Method string `yaml:"method"`
	Cache  struct {
		Duration time.Duration `yaml:"duration"`
		Header   []string      `yaml:"header"`
	} `yaml:"cache"`
}
```

[cloud-foundry]: https://www.cloudfoundry.org
[groupcache]:    https://github.com/golang/groupcache
[gorilla-mux]:   https://github.com/gorilla/mux
