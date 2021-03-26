# k8sapi

To use with Code Engine:
```
ic ce project current
# grab the KUBECONFIG env var
KUBECONFIG=... ./k8sapi
```
or
```
sh -c "`ic ce project current  | awk '/export /{print $2}'` ./k8sapi"
```
