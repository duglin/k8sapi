package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	k8sapi "github.com/duglin/k8sapi/lib"
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
	path := "/apis/serving.knative.dev/v1/namespaces/" + k8sapi.Namespace + "/services"
	code, body, err := k8sapi.KubeCall("POST", path, appStr)
	if code/100 != 2 {
		if err != nil {
			return err
		}
		return fmt.Errorf("%d: %s->%s", code, path, body)
	}

	// Now wait for the URL of the App to be created and return it
	path += "/" + name
	for {
		code, body, err = k8sapi.KubeCall("GET", path, "")
		if i := strings.Index(body, "\"url\":"); i > 0 {
			// Grab the URL + rest of line (w/o http)
			url := body[i+7:]
			// Stop at the "
			url = "https" + url[4:strings.Index(url, "\"")]
			fmt.Printf("URL: %s\n", url)
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	return nil
}

func DeleteApp(name string) error {
	path := "/apis/serving.knative.dev/v1/namespaces/" + k8sapi.Namespace +
		"/services/" + name
	code, body, err := k8sapi.KubeCall("DELETE", path, "")
	if code/100 != 2 {
		if err != nil {
			return err
		}
		return fmt.Errorf("%d: %s", code, body)
	}
	return nil
}

func GetAppStatus(name string) (bool, error) {
	path := "/apis/serving.knative.dev/v1/namespaces/" + k8sapi.Namespace +
		"/services/" + name
	code, body, err := k8sapi.KubeCall("GET", path, "")
	if code/100 != 2 {
		if err != nil {
			return false, err
		}
		return false, fmt.Errorf("%d: %s", code, body)
	}

	/* APP's STATUS  JSON:
	   "status": {
	       "address": {
	           "url": "http://echo2.79gf3v2htsc.svc.cluster.local"
	       },
	       "conditions": [
	           {
	               "lastTransitionTime": "2021-04-08T00:52:19Z",
	               "status": "True",
	               "type": "ConfigurationsReady"
	           },
	           {
	               "lastTransitionTime": "2021-04-08T00:52:26Z",
	               "status": "True",
	               "type": "Ready"
	           },
	           {
	               "lastTransitionTime": "2021-04-08T00:52:26Z",
	               "status": "True",
	               "type": "RoutesReady"
	           }
	       ],
	       "latestCreatedRevisionName": "echo2-00001",
	       "latestReadyRevisionName": "echo2-00001",
	       "observedGeneration": 1,
	       "traffic": [
	           {
	               "latestRevision": true,
	               "percent": 100,
	               "revisionName": "echo2-00001"
	           }
	       ],
	       "url": "http://echo2.79gf3v2htsc.us-south.codeengine.appdomain.cloud"
	   }
	*/

	raw := json.RawMessage{}
	res := map[string]json.RawMessage{}
	json.Unmarshal([]byte(body), &res)
	if raw = (res["status"]); raw == nil {
		return false, nil
	}
	json.Unmarshal(raw, &res)
	conditions := []struct {
		LastTransitionTime string
		Status             string
		Type               string
	}{}
	raw = res["conditions"]
	json.Unmarshal(raw, &conditions)
	for _, c := range conditions {
		if c.Type == "Ready" {
			return c.Status == "True", nil
		}
	}
	return false, nil
}

func WaitForApp(name string) error {
	for {
		ready, err := GetAppStatus(name)
		if err != nil {
			return err
		}
		if ready {
			return nil
		}
		time.Sleep(time.Second)
	}
}

func main() {
	if k8sapi.Namespace == "" {
		k8sapi.Namespace = "default"
	}

	err := CreateApp("echo2", "duglin/echo")
	if err != nil {
		fmt.Printf("%s\n", err)
		os.Exit(1)
	}
	err = WaitForApp("echo2")
	if err != nil {
		fmt.Printf("%s\n", err)
		os.Exit(1)
	}
	fmt.Printf("App is ready\n")
}
