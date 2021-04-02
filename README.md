# k8sapi

To build on Ubuntu, this should work:
```
$ apt install -y make git golang-go
$ go get gopkg.in/yaml.v2
$ make
```

To use with Code Engine:
```
ibmcloud ce project current
# grab the KUBECONFIG env var, quote the value if there are spaces
KUBECONFIG="..." ./k8sapi
```

or

```
KUBECONFIG="`ic ce project current  | sed -n s/.*KUBECONFIG=//p`" ./k8sapi
```
