all: k8sapi

export GO111MODULE=off

k8sapi: apprest.go kube.go
	go build -o k8sapi apprest.go kube.go

test: k8sapi
	sh -c "KUBECONFIG=\"`ibmcloud ce project current  | sed -n s/.*KUBECONFIG=//p`\" ./k8sapi"

clean:
	-rm -f k8sapi
	-ic ce app delete -n echo2 -f
