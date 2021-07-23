package lib

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/user"

	"gopkg.in/yaml.v2"
)

var Server = ""
var Namespace = ""
var Token = ""
var CertPool = (*x509.CertPool)(nil)

type KubeConfig struct {
	Clusters []struct {
		Name    string
		Cluster struct {
			CertAuth string `yaml:"certificate-authority"`
			Server   string
		}
	}
	Contexts []struct {
		Name    string
		Context struct {
			Cluster   string
			Namespace string
			User      string
		}
	}
	CurrentContext string `yaml:"current-context"`
	Kind           string
	// Preferences
	Users []struct {
		Name string
		User struct {
			AuthProvider struct {
				Name   string
				Config struct {
					ClientID     string `yaml:"client-id"`
					ClientSecret string `yaml:"client-secret"`
					IDToken      string `yaml:"id-token"`
					RefreshToken string `yaml:"refresh-token"`
				}
			} `yaml:"auth-provider"`
		}
	}
}

// Look at the predefined files in the Application's filesystem for
// creds and certs for how to talk to Kubernetes
func init() {
	var buf []byte

	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		user, _ := user.Current()
		if user.HomeDir != "" {
			kubeconfig = user.HomeDir + "/.kube/config"
			if _, err := os.Stat(kubeconfig); err != nil {
				kubeconfig = ""
			}
		}
	}

	// Use KUBECONFIG env var if set
	if kubeconfig != "" {
		buf, err := ioutil.ReadFile(kubeconfig)
		if err == nil && len(buf) > 0 {
			kconfig := KubeConfig{}
			err = yaml.Unmarshal(buf, &kconfig)
			if err == nil {
				ctxName := kconfig.CurrentContext
				for _, ctx := range kconfig.Contexts {
					if ctx.Name == ctxName {
						Namespace = ctx.Context.Namespace
						for _, cluster := range kconfig.Clusters {
							if cluster.Name == ctx.Context.Cluster {
								Server = cluster.Cluster.Server

								cert := []byte{}
								if cluster.Cluster.CertAuth != "" {
									if cert, err = ioutil.ReadFile(cluster.Cluster.CertAuth); err == nil {
										CertPool = x509.NewCertPool()
										CertPool.AppendCertsFromPEM(cert)

									}
								}
								break
							}
						}
						for _, user := range kconfig.Users {
							if user.Name == ctx.Context.User {
								Token = user.User.AuthProvider.Config.IDToken
							}
						}
						if Namespace != "" && Server != "" && Token != "" {
							return
						}
					}
				}
				fmt.Fprintf(os.Stderr, "Can't find current kube context\n")
			}
		} else {
			fmt.Fprintf(os.Stderr, "Error reading KUEBCONFIG(%s): %s\n",
				kubeconfig, err)
		}
		os.Exit(1)
	}

	// Assume we must be in a container so just use the config files that
	// Kube has given us
	kubeDir := "/var/run/secrets/kubernetes.io/serviceaccount"
	if _, err := os.Stat(kubeDir); err == nil {
		if buf, err = ioutil.ReadFile(kubeDir + "/namespace"); err == nil {
			Namespace = string(buf)

			cert := []byte{}
			if cert, err = ioutil.ReadFile(kubeDir + "/ca.crt"); err == nil {
				CertPool = x509.NewCertPool()
				CertPool.AppendCertsFromPEM(cert)

				if buf, err = ioutil.ReadFile(kubeDir + "/token"); err == nil {
					Token = string(buf)
					Server = "https://kubernetes.default.svc:443"
					return
				}
			}
		}
		fmt.Fprintf(os.Stderr, "Error reading Kube files: %s\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "KUBECONFIG isn't set or no serviceaccount dir\n")
	// os.Exit(1)
}

// Call Kubernetes at the specified path with the method and body (if present)
func KubeCall(method string, path string, body string) (int, string, error) {
	var reader io.Reader

	if body != "" {
		reader = bytes.NewReader([]byte(body))
	}

	// Now do the Kubernetes call
	req, err := http.NewRequest(method, Server+path, reader)
	if err != nil {
		return 0, "", err
	}

	if method == "PATCH" {
		req.Header.Add("Content-Type", "application/merge-patch+json")
	} else {
		req.Header.Add("Content-Type", "application/json")
	}
	req.Header.Add("Authorization", "Bearer "+Token)

	// fmt.Printf("URL: %s\n", Server+path)
	// fmt.Printf("Token: %s\n", Token)

	client := &http.Client{}
	if CertPool != nil {
		// Only going to be used if we're in a container
		client.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{RootCAs: CertPool}}
	}

	res, err := client.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer res.Body.Close()
	buf, err := ioutil.ReadAll(res.Body)

	return res.StatusCode, string(buf), err
}

func KubeStream(method string, path string, body string) (int, io.Reader, error) {
	var reader io.Reader

	if body != "" {
		reader = bytes.NewReader([]byte(body))
	}

	// Now do the Kubernetes call
	req, err := http.NewRequest(method, Server+path, reader)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", "Bearer "+Token)

	// fmt.Printf("URL: %s\n", Server+path)
	// fmt.Printf("Token: %s\n", Token)

	client := &http.Client{}
	if CertPool != nil {
		// Only going to be used if we're in a container
		client.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{RootCAs: CertPool}}
	}

	res, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}

	return res.StatusCode, res.Body, nil
}

type KubeObject struct {
	APIVersion string
	Kind       string
	Metadata   struct {
		Name                string            `json:"name,omitempty"`
		Namespace           string            `json:"namespace,omitempty"`
		CreationTimestamp   string            `json:"creationTimestamp,omitempty"`
		DeletionTimestamp   string            `json:"deletionTimestamp,omitempty"`
		Annotations         map[string]string `json:"annotations,omitempty"`
		Labels              map[string]string `json:"labels,omitempty"`
		ResourceVersion     string            `json:"resourceVersion,omitempty"`
		UID                 string            `json:"uid,omitempty"`
		GenerateName        string            `json:"generateName,omitempty"`
		DeletionGracePeriod int64             `json:"deletionGracePeriod,omitempty"`
		Generation          int64             `json:"geneation,omitempty"`
		Finalizers          []string          `json:"finalizers,omitempty"`
		// SelfLink            string   `json:"selfLink,omitempty"`
		OwnerReference []struct {
			APIVersion         string `json:"apiVersion,omitempty"`
			Kind               string `json:"kind,omitempty"`
			Name               string `json:"name,omitempty"`
			UID                string `json:"uid,omitempty"`
			BlockOwnerDeletion bool   `json:"blockOwnerDeletion,omitempty"`
			Controller         bool   `json:"controller,omitempty"`
		} `json:"ownerReference,omitempty"`
	}
	Spec   json.RawMessage
	Status json.RawMessage
}

type KubeEvent struct {
	Object struct {
		KubeObject

		Code    int
		Message string
		Reason  string
		// Status  string
	}
	Type string // ADDED, DELETED, MODIFIED, ERROR
}

type KubeList struct {
	APIVersion string
	Kube       string
	Metadata   struct {
		Continue        string
		ResourceVersion string
	}
	Items []*KubeObject
}

type KubeListHeader struct {
	APIVersion string
	Kube       string
	Metadata   struct {
		Continue        string
		ResourceVersion string
	}
}
