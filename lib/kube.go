package lib

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
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
			CertAuth     string `yaml:"certificate-authority"`
			CertAuthData string `yaml:"certificate-authority-data"`
			Server       string
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
			ClientCertificateData string `yaml:"client-certificate-data"`
			ClientKeyData         string `yaml:"client-key-data"`
			Token                 string
		}
	}
}

// Look at the predefined files in the Application's filesystem for
// creds and certs for how to talk to Kubernetes
func init() {
	err := LoadKubeConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}
}

func LoadKubeConfig() error {
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
		log := ""
		buf, err := ioutil.ReadFile(kubeconfig)
		if err != nil || len(buf) == 0 {
			return fmt.Errorf("Error reading %q: %s", kubeconfig, err)
		}

		kconfig := KubeConfig{}
		err = yaml.Unmarshal(buf, &kconfig)
		if err != nil {
			return fmt.Errorf("Error parsing %q: %s", kubeconfig, err)
		}

		ctxName := kconfig.CurrentContext
		for _, ctx := range kconfig.Contexts {
			log += fmt.Sprintf("\nChecking context: %s", ctx.Name)
			if ctx.Name != ctxName {
				continue
			}
			log += fmt.Sprintf("\nFound context %q", ctxName)

			Namespace = ctx.Context.Namespace
			for _, cluster := range kconfig.Clusters {
				if cluster.Name != ctx.Context.Cluster {
					continue
				}
				log += fmt.Sprintf("\nFound cluster %q", cluster.Name)

				Server = cluster.Cluster.Server

				cert := []byte{}
				if cluster.Cluster.CertAuth != "" {
					log += fmt.Sprintf("\nUsing CertAuth")
					cert, err = ioutil.ReadFile(cluster.Cluster.CertAuth)
					if err != nil {
						return fmt.Errorf("Error reading Cert (%s): %s",
							cluster.Cluster.CertAuth, err)
					}
					CertPool = x509.NewCertPool()
					CertPool.AppendCertsFromPEM(cert)

				} else if cluster.Cluster.CertAuthData != "" {
					log += fmt.Sprintf("\nUsing CertAuthData")
					CertPool = x509.NewCertPool()
					data, err := base64.StdEncoding.DecodeString(
						cluster.Cluster.CertAuthData)
					if err != nil {
						return fmt.Errorf("Error base64 decoding Cert Auth "+
							" Data: %s", err)
					}
					CertPool.AppendCertsFromPEM(data)
				}
				break
			}

			for _, user := range kconfig.Users {
				if user.Name == ctx.Context.User {
					log += fmt.Sprintf("\nFound user %q", user.Name)
					Token = user.User.AuthProvider.Config.IDToken
					if Token == "" {
						Token = user.User.Token
						break
					}
				}
			}

			log += fmt.Sprintf("\nServer: %q Token: %q", Server, Token)
			if Server != "" && Token != "" {
				return nil
			}
		}
		return fmt.Errorf("Can't find current context (%) in Kube config(%s)%s",
			ctxName, kubeconfig, log)
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
					return nil
				}
			} else {
				return fmt.Errorf("Error reading '%s/ca.crt': %s", kubeDir, err)
			}
		}
		return fmt.Errorf("Error reading '%s/namespace': %s", kubeDir, err)
	}

	return fmt.Errorf("KUBECONFIG isn't set and no serviceaccount dir")
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
		Continue           string
		ResourceVersion    string
		RemainingItemCount *int64 `json:"remainingItemCount,omitempty"`
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

type KubeStatus struct {
	Kind       string `json:"kind,omitempty"`
	APIVersion string `json:"apiVersion,omitempty"`
	Metadata   struct {
		SelfLink           string `json:"selfLink,omitempty"`
		ResourceVersion    string `json:"resourceVersion,omitempty"`
		Continue           string `json:"continue,omitempty"`
		RemainingItemCount *int64 `json:"remainingItemCount,omitempty"`
	} `json:"metadata,omitempty"`
	Status  string `json:"status,omitempty"`
	Message string `json:"message,omitempty"`
	Reason  string `json:"reason,omitempty"`
	Details struct {
		Name   string `json:"name,omitempty"`
		Group  string `json:"group,omitempty"`
		Kind   string `json:"kind,omitempty"`
		Causes []struct {
			Reason  string `json:"reason,omitempty"`
			Message string `json:"message,omitempty"`
			Field   string `json:"field,omitempty"`
		} `json:"causes,omitempty"`
	} `json:"details,omitempty"`
	Code int `json:"code,omitempty"`
}
