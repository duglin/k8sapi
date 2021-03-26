package main

import (
	"fmt"
	"os"
)

// JSON for a Knative Service (CE App)
var appJSON = `
{ 
  "apiVersion": "serving.knative.dev/v1",
  "kind": "Service",
  "metadata": { "name": "%s" },
  "spec": {
    "template": {
      "metadata": {   
        "annotations": {
          "autoscaling.knative.dev/minScale": "1",
          "autoscaling.knative.dev/maxScale": "10"
        }
      },
      "spec": {
        "containerConcurrency": 100,
        "containers": [{
          "env": [
            { "name": "myvar", "value": "some-value" }      
          ],
          "image": "%s",
          "resources": {
             "requests": { "cpu": "100m", "memory": "256M" },
             "limits": { "cpu": "100m", "memory": "256M" }
          }
        }],
        "timeoutSeconds": 300
      }
    }
  }
}`

var minimalApp = `
{ 
  "apiVersion": "serving.knative.dev/v1",
  "kind": "Service",
  "metadata": { "name": "%s" },
  "spec": {
    "template": {
      "spec": {
        "containers": [{
          "image": "%s"
        }]
      }
    }
  }
}`

func CreateApp(name string, image string) error {
	appStr := fmt.Sprintf(minimalApp, name, image)
	path := "/apis/serving.knative.dev/v1/namespaces/" + Namespace + "/services"
	code, body, err := KubeCall("POST", path, appStr)
	if code/100 != 2 {
		if err != nil {
			return err
		}
		return fmt.Errorf("%d: %s", code, body)
	}
	return nil
}

func main() {
	err := CreateApp("echo2", "duglin/echo")
	if err != nil {
		fmt.Printf("%s\n", err)
		os.Exit(1)
	}
}
